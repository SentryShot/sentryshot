use crate::{
    error::{GeneratePartError, GenerateTrafError, PartFinalizeError},
    types::VIDEO_TRACK_ID,
};
use bytes::Bytes;
use common::{
    time::{DurationH264, UnixH264},
    PartFinalized, VideoSample,
};
use mp4::ImmutableBox;
use std::sync::Arc;

fn generate_part(
    muxer_start_time: UnixH264,
    video_samples: Arc<Vec<VideoSample>>,
) -> Result<Bytes, GeneratePartError> {
    /*
       moof
       - mfhd
       - traf (video)
         - tfhd
         - tfdt
         - trun
       mdat
    */

    let mut moof = mp4::Boxes::new(mp4::Moof {}).with_child(
        // Mfhd.
        mp4::Boxes::new(mp4::Mfhd {
            full_box: mp4::FullBox::default(),
            sequence_number: 0,
        }),
    );

    let mfhd_offset = 24;
    let video_trun_size = (video_samples.len() * 16) + 20;
    let mdat_offset = mfhd_offset + video_trun_size + 44;

    let video_data_offset = i32::try_from(mdat_offset + 8)?;
    let traf = generate_traf(muxer_start_time, &video_samples, video_data_offset)?;

    moof.children.push(traf);

    let mdat = mp4::Boxes::new(MyMdat(video_samples));

    let mut buf = Vec::with_capacity(moof.size() + mdat.size());
    moof.marshal(&mut buf)?;
    mdat.marshal(&mut buf)?;

    Ok(Bytes::from(buf))
}

struct MyMdat(Arc<Vec<VideoSample>>);

impl mp4::ImmutableBox for MyMdat {
    fn box_type(&self) -> mp4::BoxType {
        mp4::TYPE_MDAT
    }

    fn size(&self) -> usize {
        self.0.iter().map(|sample| sample.avcc.len()).sum()
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), mp4::Mp4Error> {
        for sample in self.0.iter() {
            w.write_all(&sample.avcc)?;
        }
        Ok(())
    }
}

impl From<MyMdat> for Box<dyn ImmutableBox> {
    fn from(value: MyMdat) -> Self {
        Box::new(value)
    }
}

fn generate_traf(
    muxer_start_time: UnixH264,
    video_samples: &Vec<VideoSample>,
    data_offset: i32,
) -> Result<mp4::Boxes, GenerateTrafError> {
    use GenerateTrafError::*;
    /*
           traf
           - tfhd
           - tfdt
           - trun
    */

    let mut trun_entries = Vec::with_capacity(video_samples.len());
    for sample in video_samples {
        let flags = if sample.random_access_present {
            0
        } else {
            1 << 16 // sample_is_non_sync_sample
        };

        trun_entries.push(mp4::TrunEntry {
            sample_duration: u32::try_from(*sample.duration)
                .map_err(|e| TryFromInt("duration".to_owned(), e))?,
            sample_size: u32::try_from(sample.avcc.len())
                .map_err(|e| TryFromInt("sample_size".to_owned(), e))?,
            sample_flags: flags,
            sample_composition_time_offset_v0: 0,
            sample_composition_time_offset_v1: *sample.dts_offset,
        });
    }

    let first_sample = &video_samples[0];
    let dts = first_sample
        .dts()
        .ok_or(Dts(first_sample.pts, first_sample.dts_offset))?;
    let relative_dts = dts.checked_sub(muxer_start_time).ok_or(Sub)?;
    let base_media_decode_time_v1 = u64::try_from(*relative_dts)
        .map_err(|e| TryFromInt(format!("base_media_decode_time: {relative_dts:?}"), e))?;

    // Traf
    Ok(mp4::Boxes::new(mp4::Traf).with_children3(
        // Tfhd.
        mp4::Boxes::new(mp4::Tfhd {
            full_box: mp4::FullBox {
                version: 0,
                flags: [2, 0, 0],
            },
            track_id: VIDEO_TRACK_ID,
            ..mp4::Tfhd::default()
        }),
        // Tfdt.
        mp4::Boxes::new(mp4::Tfdt {
            full_box: mp4::FullBox {
                version: 1,
                flags: [0, 0, 0],
            },
            // sum of decode durations of all earlier samples
            base_media_decode_time_v0: 0,
            base_media_decode_time_v1,
        }),
        // Trun.
        mp4::Boxes::new(mp4::Trun {
            full_box: mp4::FullBox {
                version: 1,
                flags: mp4::u32_to_flags(
                    mp4::TRUN_DATA_OFFSET_PRESENT
                        | mp4::TRUN_SAMPLE_DURATION_PRESENT
                        | mp4::TRUN_SAMPLE_SIZE_PRESENT
                        | mp4::TRUN_SAMPLE_FLAGS_PRESENT
                        | mp4::TRUN_SAMPLE_COMPOSITION_TIME_OFFSET_PRESENT,
                ),
            },
            data_offset,
            first_sample_flags: 0,
            entries: trun_entries,
        }),
    ))
}

// fmp4 part.
#[derive(Clone)]
#[allow(clippy::module_name_repetitions)]
pub struct MuxerPart {
    pub id: u64,
    pub muxer_start_time: UnixH264,
    pub is_independent: bool,
    pub video_samples: Vec<VideoSample>,
}

impl std::fmt::Debug for MuxerPart {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "{} {} {}",
            self.id,
            self.is_independent,
            self.video_samples.len()
        )
    }
}

impl MuxerPart {
    pub fn new(id: u64, muxer_start_time: UnixH264) -> Self {
        Self {
            id,
            muxer_start_time,
            is_independent: false,
            video_samples: Vec::new(),
        }
    }

    pub fn duration(&self) -> Option<DurationH264> {
        let mut total = DurationH264::new(0);
        for e in &self.video_samples {
            total = total.checked_add(e.duration)?;
        }
        Some(total)
    }

    pub fn finalize(self) -> Result<PartFinalized, PartFinalizeError> {
        let rendered_duration = self.duration().ok_or(PartFinalizeError::Duration)?;
        let video_samples = Arc::new(self.video_samples);
        let rendered_content = if video_samples.is_empty() {
            None
        } else {
            Some(generate_part(self.muxer_start_time, video_samples.clone())?)
        };

        Ok(PartFinalized {
            id: self.id,
            is_independent: self.is_independent,
            video_samples: video_samples.clone(),
            rendered_duration,
            rendered_content,
        })
    }

    pub fn write_h264(&mut self, sample: VideoSample) {
        if sample.random_access_present {
            self.is_independent = true;
        }
        self.video_samples.push(sample);
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::time::{DtsOffset, UnixH264, SECOND};
    use pretty_assertions::assert_eq;
    use pretty_hex::pretty_hex;
    use sentryshot_padded_bytes::PaddedBytes;

    #[test]
    fn test_generate_part_minimal() {
        let got = generate_part(UnixH264::new(0), Arc::new(vec![VideoSample::default()])).unwrap();

        let want = vec![
            0, 0, 0, 0x68, b'm', b'o', b'o', b'f', //
            0, 0, 0, 0x10, b'm', b'f', b'h', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Sequence number.
            0, 0, 0, 0x50, b't', b'r', b'a', b'f', //
            0, 0, 0, 0x10, b't', b'f', b'h', b'd', //
            0, 2, 0, 0, // Track id.
            0, 0, 0, 1, // Sample size.
            0, 0, 0, 0x14, b't', b'f', b'd', b't', //
            1, 0, 0, 0, // Track id.
            0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
            0, 0, 0, 0x24, b't', b'r', b'u', b'n', // Video trun.
            1, 0, 0xf, 1, // FullBox.
            0, 0, 0, 1, // Sample count.
            0, 0, 0, 0x70, // Data offset.
            0, 0, 0, 0, // Entry sample duration.
            0, 0, 0, 0, // Entry sample size.
            0, 1, 0, 0, // Entry sample flags.
            0, 0, 0, 0, // Entry SampleCompositionTimeOffset
            0, 0, 0, 8, b'm', b'd', b'a', b't', //
        ];
        assert_eq!(want, got);
    }

    #[test]
    fn test_generate_part_video_sample() {
        let samples = vec![VideoSample {
            avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
            ..Default::default()
        }];

        let got = generate_part(UnixH264::new(0), Arc::new(samples)).unwrap();
        let want = vec![
            0, 0, 0, 0x68, b'm', b'o', b'o', b'f', //
            0, 0, 0, 0x10, b'm', b'f', b'h', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Sequence number.
            0, 0, 0, 0x50, b't', b'r', b'a', b'f', // Video traf.
            0, 0, 0, 0x10, b't', b'f', b'h', b'd', // Video tfhd.
            0, 2, 0, 0, // Track id.
            0, 0, 0, 1, // Sample size.
            0, 0, 0, 0x14, b't', b'f', b'd', b't', // Video tfdt.
            1, 0, 0, 0, // Track id.
            0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
            0, 0, 0, 0x24, b't', b'r', b'u', b'n', // Video trun.
            1, 0, 0xf, 1, // FullBox.
            0, 0, 0, 1, // Sample count.
            0, 0, 0, 0x70, // Data offset.
            0, 0, 0, 0, // Entry sample duration.
            0, 0, 0, 4, // Entry sample size.
            0, 1, 0, 0, // Entry sample flags.
            0, 0, 0, 0, // Entry SampleCompositionTimeOffset
            0, 0, 0, 0xc, b'm', b'd', b'a', b't', //
            b'a', b'b', b'c', b'd', // Video Sample
        ];
        assert_eq!(want, got);
    }

    #[test]
    fn test_generate_part_multiple_video_samples() {
        let samples = vec![
            VideoSample {
                avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
                random_access_present: true,
                ..Default::default()
            },
            VideoSample {
                avcc: Arc::new(PaddedBytes::new(b"efgh".to_vec())),
                ..Default::default()
            },
            VideoSample {
                avcc: Arc::new(PaddedBytes::new(b"ijkl".to_vec())),
                ..Default::default()
            },
        ];

        let got = generate_part(UnixH264::new(0), Arc::new(samples)).unwrap();

        let want = vec![
            0, 0, 0, 0x88, b'm', b'o', b'o', b'f', //
            0, 0, 0, 0x10, b'm', b'f', b'h', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Sequence number.
            0, 0, 0, 0x70, b't', b'r', b'a', b'f', // Video traf.
            0, 0, 0, 0x10, b't', b'f', b'h', b'd', // Video tfhd.
            0, 2, 0, 0, // Track id.
            0, 0, 0, 1, // Sample size.
            0, 0, 0, 0x14, b't', b'f', b'd', b't', // Video tfdt.
            1, 0, 0, 0, // Track id.
            0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
            0, 0, 0, 0x44, b't', b'r', b'u', b'n', // Video trun.
            1, 0, 0xf, 1, // FullBox.
            0, 0, 0, 3, // Sample count.
            0, 0, 0, 0x90, // Data offset.
            0, 0, 0, 0, // Entry1 sample duration.
            0, 0, 0, 4, // Entry1 sample size.
            0, 0, 0, 0, // Entry1 sample flags.
            0, 0, 0, 0, // Entry1 SampleCompositionTimeOffset
            0, 0, 0, 0, // Entry2 sample duration.
            0, 0, 0, 4, // Entry2 sample size.
            0, 1, 0, 0, // Entry2 sample flags.
            0, 0, 0, 0, // Entry2 SampleCompositionTimeOffset
            0, 0, 0, 0, // Entry3 sample duration.
            0, 0, 0, 4, // Entry3 sample size.
            0, 1, 0, 0, // Entry3 sample flags.
            0, 0, 0, 0, // Entry3 SampleCompositionTimeOffset
            0, 0, 0, 0x14, b'm', b'd', b'a', b't', //
            b'a', b'b', b'c', b'd', b'e', b'f', b'g', b'h', b'i', b'j', b'k',
            b'l', // Video Samples
        ];
        assert_eq!(want, got);
    }

    #[test]
    fn test_generate_part_minimal_real() {
        let start_time = UnixH264::new(1_000_000_000 * SECOND);
        let samples = vec![
            VideoSample {
                pts: start_time + UnixH264::new(54000),
                dts_offset: DtsOffset::new(54000 - 60000),
                avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
                random_access_present: true,
                duration: DurationH264::new(11999),
            },
            VideoSample {
                pts: start_time + UnixH264::new(63000),
                dts_offset: DtsOffset::new(63000 - 72000),
                avcc: Arc::new(PaddedBytes::new(b"efgh".to_vec())),
                random_access_present: false,
                duration: DurationH264::new(9000),
            },
        ];

        let got = generate_part(start_time, Arc::new(samples)).unwrap();

        let want = vec![
            0, 0, 0, 0x78, b'm', b'o', b'o', b'f', //
            0, 0, 0, 0x10, b'm', b'f', b'h', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Sequence number.
            0, 0, 0, 0x60, b't', b'r', b'a', b'f', // Video traf.
            0, 0, 0, 0x10, b't', b'f', b'h', b'd', // Video tfhd.
            0, 2, 0, 0, // Track id.
            0, 0, 0, 1, // Sample size.
            0, 0, 0, 0x14, b't', b'f', b'd', b't', // Video tfdt.
            1, 0, 0, 0, // Track id.
            0, 0, 0, 0, 0, 0, 0xea, 0x60, // BaseMediaDecodeTime.
            0, 0, 0, 0x34, b't', b'r', b'u', b'n', // Video trun.
            1, 0, 0xf, 1, // FullBox.
            0, 0, 0, 2, // Sample count.
            0, 0, 0, 0x80, // Data offset.
            0, 0, 0x2e, 0xdf, // Entry1 sample duration.
            0, 0, 0, 4, // Entry1 sample size.
            0, 0, 0, 0, // Entry1 sample flags.
            0xff, 0xff, 0xe8, 0x90, // 1 Entry SampleCompositionTimeOffset
            0, 0, 0x23, 0x28, // 2 Entry sample duration.
            0, 0, 0, 4, // 2 Entry sample size.
            0, 1, 0, 0, // 2 Entry sample flags.
            0xff, 0xff, 0xdc, 0xd8, // Entry2 SampleCompositionTimeOffset
            0, 0, 0, 0x10, b'm', b'd', b'a', b't', //
            b'a', b'b', b'c', b'd', b'e', b'f', b'g', b'h', // Samples
        ];
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }
}

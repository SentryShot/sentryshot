// SPDX-License-Identifier: GPL-2.0-or-later

use common::{
    TrackParameters, VideoSample,
    time::{DtsOffset, H264_TIMESCALE, UnixH264},
};
use mp4::{ImmutableBox, ImmutableBoxSync, TfdtBaseMediaDecodeTime, TrunEntries};
use thiserror::Error;

// 14496-12_2015 8.3.2.3
// track_ID is an integer that uniquely identifies this track
// over the entire life‐time of this presentation.
// Track IDs are never re‐used and cannot be zero.
pub const VIDEO_TRACK_ID: u32 = 1;

#[allow(clippy::module_name_repetitions)]
pub fn generate_init(params: &TrackParameters) -> Result<Vec<u8>, mp4::Mp4Error> {
    /*
       - ftyp
       - moov
         - mvhd
         - trak (video)
         - mvex
           - trex (video)
    */

    let ftyp = mp4::Boxes::new(
        // Ftyp.
        mp4::Ftyp {
            major_brand: *b"mp42",
            minor_version: 1,
            compatible_brands: vec![
                mp4::CompatibleBrandElem(*b"mp41"),
                mp4::CompatibleBrandElem(*b"mp42"),
                mp4::CompatibleBrandElem(*b"isom"),
                mp4::CompatibleBrandElem(*b"hlsf"),
            ],
        },
    );

    let trak = generate_trak(params);

    let moov = mp4::Boxes::new(mp4::Moov).with_children3(
        // Mvhd.
        mp4::Boxes::new(mp4::Mvhd {
            timescale: 1000,
            rate: 65536,
            volume: 256,
            matrix: [0x0001_0000, 0, 0, 0, 0x0001_0000, 0, 0, 0, 0x4000_0000],
            next_track_id: 2,
            ..mp4::Mvhd::default()
        }),
        // Trak.
        trak,
        // Mvex.
        mp4::Boxes::new(mp4::Mvex)
            // Trex.
            .with_child(mp4::Boxes::new(mp4::Trex {
                track_id: VIDEO_TRACK_ID,
                default_sample_description_index: 1,
                ..mp4::Trex::default()
            })),
    );

    let size = ftyp.size() + moov.size();
    let mut buf = Vec::with_capacity(size);

    ftyp.marshal(&mut buf)?;
    moov.marshal(&mut buf)?;

    Ok(buf)
}

#[allow(clippy::too_many_lines)]
fn generate_trak(params: &TrackParameters) -> mp4::Boxes {
    /*
       trak
       - tkhd
       - mdia
         - mdhd
         - hdlr
         - minf
           - vmhd
           - dinf
             - dref
               - url
           - stbl
             - stsd
               - avc1
                 - avcC
                 - btrt
             - stts
             - stsc
             - stsz
             - stco
    */

    let stbl = mp4::Boxes::new(mp4::Stbl).with_children5(
        // Stds.
        mp4::Boxes::new(mp4::Stsd {
            full_box: mp4::FullBox::default(),
            entry_count: 1,
        })
        .with_child(
            // Avc1.
            mp4::Boxes::new(mp4::Avc1 {
                sample_entry: mp4::SampleEntry {
                    reserved: [0, 0, 0, 0, 0, 0],
                    data_reference_index: 1,
                },
                width: params.width,
                height: params.height,
                horiz_resolution: 4_718_592,
                vert_resolution: 4_718_592,
                frame_count: 1,
                depth: 24,
                pre_defined3: -1,
                ..mp4::Avc1::default()
            })
            .with_children2(
                // AvcC.
                mp4::Boxes::new(MyAvcC(params.extra_data.clone())),
                // Btrt.
                mp4::Boxes::new(mp4::Btrt {
                    buffer_size_db: 0,
                    max_bitrate: 1_000_000,
                    avg_bitrate: 1_000_000,
                }),
            ),
        ),
        // Stts.
        mp4::Boxes::new(mp4::Stts::default()),
        // Stsc.
        mp4::Boxes::new(mp4::Stsc::default()),
        // Stsz.
        mp4::Boxes::new(mp4::Stsz::default()),
        // Stco.
        mp4::Boxes::new(mp4::Stco::default()),
    );

    let minf = mp4::Boxes::new(mp4::Minf).with_children3(
        // Vmhd.
        mp4::Boxes::new(mp4::Vmhd {
            full_box: mp4::FullBox {
                version: 0,
                flags: [0, 0, 1],
            },
            graphics_mode: 0,
            opcolor: [0, 0, 0],
        }),
        // Dinf.
        mp4::Boxes::new(mp4::Dinf).with_child(
            // Dref.
            mp4::Boxes::new(mp4::Dref {
                full_box: mp4::FullBox::default(),
                entry_count: 1,
            })
            .with_child(mp4::Boxes::new(
                // Url.
                mp4::Url {
                    full_box: mp4::FullBox {
                        version: 0,
                        flags: [0, 0, 1],
                    },
                    location: String::new(),
                },
            )),
        ),
        // Stbl.
        stbl,
    );

    // Trak.
    mp4::Boxes::new(mp4::Trak {}).with_children2(
        // Tkhd.
        mp4::Boxes::new(mp4::Tkhd {
            flags: [0, 0, 3],
            track_id: VIDEO_TRACK_ID,
            width: u32::from(params.width) * 65536,
            height: u32::from(params.height) * 65536,
            matrix: [0x0001_0000, 0, 0, 0, 0x0001_0000, 0, 0, 0, 0x4000_0000],
            ..mp4::Tkhd::default()
        }),
        // Mdia
        mp4::Boxes::new(mp4::Mdia {}).with_children3(
            // Mdhd.
            mp4::Boxes::new(mp4::Mdhd {
                timescale: H264_TIMESCALE,
                language: *b"und",
                ..mp4::Mdhd::default()
            }),
            // Hdlr
            mp4::Boxes::new(mp4::Hdlr {
                handler_type: *b"vide",
                name: "VideoHandler".to_owned(),
                ..mp4::Hdlr::default()
            }),
            // Minf.
            minf,
        ),
    )
}

struct MyAvcC(Vec<u8>);

impl ImmutableBox for MyAvcC {
    fn box_type(&self) -> mp4::BoxType {
        mp4::TYPE_AVCC
    }

    fn size(&self) -> usize {
        self.0.len()
    }
}

impl ImmutableBoxSync for MyAvcC {
    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), mp4::Mp4Error> {
        w.write_all(&self.0)?;
        Ok(())
    }
}

impl From<MyAvcC> for Box<dyn ImmutableBoxSync> {
    fn from(value: MyAvcC) -> Self {
        Box::new(value)
    }
}

#[derive(Debug, Error)]
pub enum GenerateMoofError {
    #[error("from int: {0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("generate traf: {0}")]
    GenerateTraf(#[from] GenerateTrafError),

    #[error("mp4: {0}")]
    Mp4(#[from] mp4::Mp4Error),
}

pub(crate) fn generate_moof_and_empty_mdat(
    muxer_start_time: UnixH264,
    frames: &[&VideoSample],
) -> Result<Vec<u8>, GenerateMoofError> {
    /*
       moof
       - mfhd
       - traf (video)
         - tfhd
         - tfdt
         - trun
       mdat (empty)
    */

    let mut moof = mp4::Boxes::new(mp4::Moof {}).with_child(
        // Mfhd.
        mp4::Boxes::new(mp4::Mfhd {
            full_box: mp4::FullBox::default(),
            sequence_number: 0,
        }),
    );

    let mfhd_offset = 24;
    let video_trun_size = (frames.len() * 16) + 20;
    let mdat_offset = mfhd_offset + video_trun_size + 44;

    let video_data_offset = i32::try_from(mdat_offset + 8)?;
    let traf = generate_traf(muxer_start_time, frames, video_data_offset)?;

    moof.children.push(traf);

    let mdat_size = frames.iter().map(|v| v.avcc.len()).sum();
    let mdat = mp4::Boxes::new(EmptyMdat(mdat_size));

    let mut buf = Vec::with_capacity(moof.size() + mdat.size());
    moof.marshal(&mut buf)?;
    mdat.marshal(&mut buf)?;

    Ok(buf)
}

struct EmptyMdat(usize);

impl ImmutableBox for EmptyMdat {
    fn box_type(&self) -> mp4::BoxType {
        mp4::TYPE_MDAT
    }

    fn size(&self) -> usize {
        self.0
    }
}

impl ImmutableBoxSync for EmptyMdat {
    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), mp4::Mp4Error> {
        Ok(())
    }
}

impl From<EmptyMdat> for Box<dyn ImmutableBoxSync> {
    fn from(value: EmptyMdat) -> Self {
        Box::new(value)
    }
}

#[derive(Debug, Error)]
pub enum GenerateTrafError {
    #[error("from int: {0} {1}")]
    TryFromInt(String, std::num::TryFromIntError),

    #[error("dts {0:?} {1:?}")]
    Dts(UnixH264, DtsOffset),

    #[error("sub")]
    Sub,
}

fn generate_traf(
    muxer_start_time: UnixH264,
    frames: &[&VideoSample],
    data_offset: i32,
) -> Result<mp4::Boxes, GenerateTrafError> {
    use GenerateTrafError::*;
    /*
           traf
           - tfhd
           - tfdt
           - trun
    */

    let mut trun_entries = Vec::with_capacity(frames.len());
    for sample in frames {
        let flags = if sample.random_access_present {
            0
        } else {
            1 << 16 // sample_is_non_sync_sample
        };

        trun_entries.push(mp4::TrunEntryV1 {
            sample_duration: u32::try_from(*sample.duration)
                .map_err(|e| TryFromInt("duration".to_owned(), e))?,
            sample_size: u32::try_from(sample.avcc.len())
                .map_err(|e| TryFromInt("sample_size".to_owned(), e))?,
            sample_flags: flags,
            sample_composition_time_offset: *sample.dts_offset,
        });
    }

    let first_sample = &frames[0];
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
            flags: [0, 0, 0],
            // sum of decode durations of all earlier samples
            base_media_decode_time: TfdtBaseMediaDecodeTime::V1(base_media_decode_time_v1),
        }),
        // Trun.
        mp4::Boxes::new(mp4::Trun {
            flags: mp4::u32_to_flags(
                mp4::TRUN_DATA_OFFSET_PRESENT
                    | mp4::TRUN_SAMPLE_DURATION_PRESENT
                    | mp4::TRUN_SAMPLE_SIZE_PRESENT
                    | mp4::TRUN_SAMPLE_FLAGS_PRESENT
                    | mp4::TRUN_SAMPLE_COMPOSITION_TIME_OFFSET_PRESENT,
            ),
            data_offset,
            first_sample_flags: 0,
            entries: TrunEntries::V1(trun_entries),
        }),
    ))
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use common::time::{DurationH264, SECOND};
    use pretty_assertions::assert_eq;
    use pretty_hex::pretty_hex;
    use sentryshot_padded_bytes::PaddedBytes;

    #[test]
    #[allow(clippy::too_many_lines)]
    fn test_generate_init() {
        let params = TrackParameters {
            width: 650,
            height: 450,
            extra_data: vec![
                0x1, 0x64, 0x0, 0x16, 0x3, 0x1, 0x0, 0x1b, 0x67, 0x64, 0x0, 0x16, 0xac, 0xd9, 0x40,
                0xa4, 0x3b, 0xe4, 0x88, 0xc0, 0x44, 0x0, 0x0, 0x3, 0x0, 0x4, 0x0, 0x0, 0x3, 0x0,
                0x60, 0x3c, 0x58, 0xb6, 0x58, 0x1, 0x0, 0x0,
            ],
            codec: String::new(),
        };

        let got = generate_init(&params).unwrap();

        let want = vec![
            0, 0, 0, 0x20, b'f', b't', b'y', b'p', //
            b'm', b'p', b'4', b'2', // Major brand.
            0, 0, 0, 1, // Minor version.
            b'm', b'p', b'4', b'1', // Compatible brand.
            b'm', b'p', b'4', b'2', // Compatible brand.
            b'i', b's', b'o', b'm', // Compatible brand.
            b'h', b'l', b's', b'f', // Compatible brand.
            0, 0, 2, 0x89, b'm', b'o', b'o', b'v', //
            0, 0, 0, 0x6c, b'm', b'v', b'h', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Creation time.
            0, 0, 0, 0, // Modification time.
            0, 0, 3, 0xe8, // Time scale.
            0, 0, 0, 0, // Duration.
            0, 1, 0, 0, // Rate.
            1, 0, // Volume.
            0, 0, // Reserved.
            0, 0, 0, 0, 0, 0, 0, 0, // Reserved2.
            0, 1, 0, 0, // 1 Matrix.
            0, 0, 0, 0, // 2.
            0, 0, 0, 0, // 3.
            0, 0, 0, 0, // 4.
            0, 1, 0, 0, // 5.
            0, 0, 0, 0, // 6.
            0, 0, 0, 0, // 7.
            0, 0, 0, 0, // 8.
            0x40, 0, 0, 0, // 9.
            0, 0, 0, 0, // 1 Predefined.
            0, 0, 0, 0, // 2.
            0, 0, 0, 0, // 3.
            0, 0, 0, 0, // 4.
            0, 0, 0, 0, // 5.
            0, 0, 0, 0, // 6.
            0, 0, 0, 2, // Next track ID.
            0, 0, 1, 0xed, b't', b'r', b'a', b'k', // Video.
            0, 0, 0, 0x5c, b't', b'k', b'h', b'd', //
            0, 0, 0, 3, // FullBox.
            0, 0, 0, 0, // Creation time.
            0, 0, 0, 0, // Modification time.
            0, 0, 0, 1, // Track ID.
            0, 0, 0, 0, // Reserved0.
            0, 0, 0, 0, // Duration.
            0, 0, 0, 0, 0, 0, 0, 0, // Reserved1.
            0, 0, // Layer.
            0, 0, // Alternate group.
            0, 0, // Volume.
            0, 0, // Reserved2.
            0, 1, 0, 0, // 1 Matrix.
            0, 0, 0, 0, // 2.
            0, 0, 0, 0, // 3.
            0, 0, 0, 0, // 4.
            0, 1, 0, 0, // 5.
            0, 0, 0, 0, // 6.
            0, 0, 0, 0, // 7.
            0, 0, 0, 0, // 8.
            0x40, 0, 0, 0, // 9.
            2, 0x8a, 0, 0, // Width
            1, 0xc2, 0, 0, // Height
            0, 0, 1, 0x89, b'm', b'd', b'i', b'a', //
            0, 0, 0, 0x20, b'm', b'd', b'h', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Creation time.
            0, 0, 0, 0, // Modification time.
            0, 1, 0x5f, 0x90, // Time scale.
            0, 0, 0, 0, // Duration.
            0x55, 0xc4, // Language.
            0, 0, // Predefined.
            0, 0, 0, 0x2d, b'h', b'd', b'l', b'r', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Predefined.
            b'v', b'i', b'd', b'e', // Handler type.
            0, 0, 0, 0, // Reserved.
            0, 0, 0, 0, //
            0, 0, 0, 0, //
            b'V', b'i', b'd', b'e', b'o', b'H', b'a', b'n', b'd', b'l', b'e', b'r', 0, //
            0, 0, 1, 0x34, b'm', b'i', b'n', b'f', //
            0, 0, 0, 0x14, b'v', b'm', b'h', b'd', //
            0, 0, 0, 1, // FullBox.
            0, 0, // Graphics mode.
            0, 0, 0, 0, 0, 0, // OpColor.
            0, 0, 0, 0x24, b'd', b'i', b'n', b'f', //
            0, 0, 0, 0x1c, b'd', b'r', b'e', b'f', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 0xc, b'u', b'r', b'l', b' ', //
            0, 0, 0, 1, // FullBox.
            0, 0, 0, 0xf4, b's', b't', b'b', b'l', //
            0, 0, 0, 0xa8, b's', b't', b's', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 0x98, b'a', b'v', b'c', b'1', //
            0, 0, 0, 0, 0, 0, // Reserved.
            0, 1, // Data reference index.
            0, 0, // Predefined.
            0, 0, // Reserved.
            0, 0, 0, 0, // Predefined2.
            0, 0, 0, 0, 0, 0, 0, 0, 2, 0x8a, // Width.
            1, 0xc2, // Height.
            0, 0x48, 0, 0, // Horizresolution
            0, 0x48, 0, 0, // Vertresolution
            0, 0, 0, 0, // Reserved2.
            0, 1, // Frame count.
            0, 0, 0, 0, 0, 0, 0, 0, // Compressor name.
            0, 0, 0, 0, 0, 0, 0, 0, //
            0, 0, 0, 0, 0, 0, 0, 0, //
            0, 0, 0, 0, 0, 0, 0, 0, //
            0, 0x18, // Depth.
            0xff, 0xff, // Predefined3.
            0, 0, 0, 0x2e, b'a', b'v', b'c', b'C', //
            1,    // Configuration version.
            0x64, // Profile.
            0,    // Profile compatibility.
            0x16, // Level.
            3,    // Reserved, Length size minus one.
            1,    // Reserved, N sequence parameters.
            0, 0x1b, // Length 27.
            0x67, 0x64, 0, 0x16, 0xac, // Parameter set.
            0xd9, 0x40, 0xa4, 0x3b, 0xe4, //
            0x88, 0xc0, 0x44, 0, 0, //
            3, 0, 4, 0, 0, //
            3, 0, 0x60, 0x3c, 0x58, //
            0xb6, 0x58, //
            1,    // Reserved N sequence parameters.
            0, 0, // Length.
            0, 0, 0, 0x14, b'b', b't', b'r', b't', //
            0, 0, 0, 0, // Buffer size.
            0, 0xf, 0x42, 0x40, // Max bitrate.
            0, 0xf, 0x42, 0x40, // Average bitrate.
            0, 0, 0, 0x10, b's', b't', b't', b's', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Entry count.
            0, 0, 0, 0x10, b's', b't', b's', b'c', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Entry count.
            0, 0, 0, 0x14, b's', b't', b's', b'z', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Sample size.
            0, 0, 0, 0, // Sample count.
            0, 0, 0, 0x10, b's', b't', b'c', b'o', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Entry count.
            0, 0, 0, 0x28, b'm', b'v', b'e', b'x', //
            0, 0, 0, 0x20, b't', b'r', b'e', b'x', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Track ID.
            0, 0, 0, 1, // Default sample description index.
            0, 0, 0, 0, // Default sample duration.
            0, 0, 0, 0, // Default sample size.
            0, 0, 0, 0, // Default sample flags.
        ];
        if want != got {
            assert_eq!(pretty_hex(&want), pretty_hex(&got));
        }
    }

    #[test]
    fn test_generate_moof_minimal() {
        let got =
            generate_moof_and_empty_mdat(UnixH264::new(0), &[&VideoSample::default()]).unwrap();

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
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[test]
    fn test_generate_part_video_sample() {
        let samples = [&VideoSample {
            avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
            ..Default::default()
        }];

        let got = generate_moof_and_empty_mdat(UnixH264::new(0), &samples).unwrap();
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
            0, 0, 0, 0xc, b'm', b'd', b'a', b't', // Empty.
        ];
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[test]
    fn test_generate_part_multiple_video_samples() {
        let samples = [
            &VideoSample {
                avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
                random_access_present: true,
                ..Default::default()
            },
            &VideoSample {
                avcc: Arc::new(PaddedBytes::new(b"efgh".to_vec())),
                ..Default::default()
            },
            &VideoSample {
                avcc: Arc::new(PaddedBytes::new(b"ijkl".to_vec())),
                ..Default::default()
            },
        ];

        let got = generate_moof_and_empty_mdat(UnixH264::new(0), &samples).unwrap();

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
            0, 0, 0, 0x14, b'm', b'd', b'a', b't', // Empty.
        ];
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[test]
    fn test_generate_part_minimal_real() {
        let start_time = UnixH264::new(1_000_000_000 * SECOND);
        let samples = [
            &VideoSample {
                pts: start_time + UnixH264::new(54000),
                dts_offset: DtsOffset::new(54000 - 60000),
                avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
                random_access_present: true,
                duration: DurationH264::new(11999),
            },
            &VideoSample {
                pts: start_time + UnixH264::new(63000),
                dts_offset: DtsOffset::new(63000 - 72000),
                avcc: Arc::new(PaddedBytes::new(b"efgh".to_vec())),
                random_access_present: false,
                duration: DurationH264::new(9000),
            },
        ];

        let got = generate_moof_and_empty_mdat(start_time, &samples).unwrap();

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
            0, 0, 0, 0x10, b'm', b'd', b'a', b't', // Empty.
        ];
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }
}

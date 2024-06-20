// SPDX-License-Identifier: GPL-2.0-or-later

use crate::video::{Sample, TrackParameters};
use common::time::{DurationH264, UnixH264, H264_TIMESCALE};
use hls::VIDEO_TRACK_ID;
use mp4::{FullBox, ImmutableBox, Mp4Error};
use std::{cell::RefCell, io::Write, num::TryFromIntError, rc::Rc};
use thiserror::Error;

#[derive(Debug, Error)]
pub enum GenerateMp4Error {
    #[error("mp4: {0}")]
    Mp4(#[from] Mp4Error),

    #[error("from int: {0}")]
    FromInt(#[from] TryFromIntError),

    #[error("add")]
    Add,

    #[error("subtract")]
    Sub,

    #[error("write: {0}")]
    Write(#[from] std::io::Error),
}

#[derive(Default)]
struct Mp4Muxer {
    stts: Vec<mp4::SttsEntry>,
    stss: Vec<u32>,
    ctts: Vec<mp4::CttsEntry>,
    stsc: Vec<mp4::StscEntry>,
    stsz: Vec<u32>,
    stco: Rc<RefCell<Vec<u32>>>,
}

#[allow(clippy::items_after_statements, clippy::similar_names)]
pub fn generate_mp4(
    mut out: &mut dyn Write,
    start_time: UnixH264,
    samples: &[Sample],
    params: &TrackParameters,
) -> Result<u32, GenerateMp4Error> {
    use GenerateMp4Error::*;

    let mut m = Mp4Muxer::default();
    let mut mdat_pos: u32 = 0;
    let mut end_time = UnixH264::new(0);
    let mut dts_shift = DurationH264::new(0);
    let mut stco = m.stco.borrow_mut();

    for sample in samples {
        let delta = sample.duration.as_u32()?;
        match m.stts.last_mut() {
            Some(last) if last.sample_delta == delta => {
                last.sample_count += 1;
            }
            _ => m.stts.push(mp4::SttsEntry {
                sample_count: 1,
                sample_delta: delta,
            }),
        }

        let pts = sample.pts.checked_sub(start_time).ok_or(Sub)?.as_duration();
        let dts = sample
            .dts()
            .ok_or(Sub)?
            .checked_sub(start_time)
            .ok_or(Sub)?
            .as_duration();

        let first_sample = m.stsc.is_empty();
        if first_sample {
            dts_shift = pts.checked_sub(dts).ok_or(Sub)?;
        }

        let cts = i32::try_from(
            *pts.checked_sub(dts.checked_add(dts_shift).ok_or(Add)?)
                .ok_or(Add)?,
        )?;
        //cts := pts - (dts + m.dtsShift)

        match m.ctts.last_mut() {
            Some(last) if last.sample_offset_v1 == cts => {
                last.sample_count += 1;
            }
            _ => m.ctts.push(mp4::CttsEntry {
                sample_count: 1,
                sample_offset_v0: 0,
                sample_offset_v1: cts,
            }),
        }

        if let Some(last_stsc) = m.stsc.last_mut() {
            last_stsc.samples_per_chunk += 1;
        } else {
            stco.push(mdat_pos);
            m.stsc.push(mp4::StscEntry {
                first_chunk: 1,
                samples_per_chunk: 1,
                sample_description_index: 1,
            });
        }

        mdat_pos += sample.data_size;
        m.stsz.push(sample.data_size);

        if sample.random_access_present {
            m.stss.push(u32::try_from(m.stsz.len())?);
        }

        end_time = sample
            .dts()
            .ok_or(Sub)?
            .checked_add(sample.duration.into())
            .ok_or(Add)?;
    }
    drop(stco);

    let duration = DurationH264::from(
        end_time
            .checked_sub(start_time)
            .ok_or(GenerateMp4Error::Sub)?,
    );
    //duration := time.Duration(m.endTime - m.startTime)

    let moov = mp4::Boxes::new(mp4::Moov {}).with_children2(
        // Mvhd.
        mp4::Boxes::new(mp4::Mvhd {
            timescale: 1000,
            duration_v0: u32::try_from(duration.as_millis())?,
            rate: 65536,
            volume: 256,
            matrix: [0x0001_0000, 0, 0, 0, 0x0001_0000, 0, 0, 0, 0x4000_0000],
            next_track_id: VIDEO_TRACK_ID + 1,
            ..Default::default()
        }),
        // Trak.
        m.generate_trak(duration, params)?,
    );

    const FTYP_SIZE: u32 = 20;
    const MDAT_HEADER_SIZE: u32 = 8;
    let mdat_offset: u32 = FTYP_SIZE + u32::try_from(moov.size())? + MDAT_HEADER_SIZE;

    let mut stco = m.stco.borrow_mut();
    for i in 0..stco.len() {
        stco[i] += mdat_offset;
    }
    drop(stco);

    /*
       ftyp
       moov
       - mvhd
       - trak (video)
       mdat
    */

    let ftyp = mp4::Ftyp {
        major_brand: *b"iso4",
        minor_version: 512,
        compatible_brands: vec![mp4::CompatibleBrandElem(*b"iso4")],
    };
    mp4::write_single_box(&mut out, &ftyp)?;

    moov.marshal(&mut out)?;

    out.write_all(&(mdat_pos.checked_add(8).ok_or(GenerateMp4Error::Add)?).to_be_bytes())?;
    out.write_all(b"mdat")?;

    Ok(mdat_pos)
}

impl Mp4Muxer {
    #[allow(clippy::let_and_return)]
    fn generate_trak(
        &self,
        duration: DurationH264,
        params: &TrackParameters,
    ) -> Result<mp4::Boxes, TryFromIntError> {
        /*
           trak
           - tkhd
           - mdia
             - mdhd
             - hdlr
             - minf
        */

        let trak = mp4::Boxes::new(mp4::Trak).with_children2(
            // Tkhd.
            mp4::Boxes::new(mp4::Tkhd {
                full_box: mp4::FullBox {
                    version: 0,
                    flags: [0, 0, 3],
                },
                track_id: VIDEO_TRACK_ID,
                duration_v0: u32::try_from(duration.as_millis())?,
                width: u32::from(params.width) * 65536,
                height: u32::from(params.height) * 65536,
                matrix: [0x0001_0000, 0, 0, 0, 0x0001_0000, 0, 0, 0, 0x4000_0000],
                ..Default::default()
            }),
            // Mdia.
            mp4::Boxes::new(mp4::Mdia).with_children3(
                // Mdhd.
                mp4::Boxes::new(mp4::Mdhd {
                    timescale: H264_TIMESCALE,
                    language: *b"und",
                    duration_v0: duration.as_u32()?,
                    ..Default::default()
                }),
                // Hdlr.
                mp4::Boxes::new(mp4::Hdlr {
                    handler_type: *b"vide",
                    name: "VideoHandler".to_owned(),
                    ..Default::default()
                }),
                // Minf.
                self.generate_minf(params)?,
            ),
        );

        Ok(trak)
    }

    #[allow(clippy::let_and_return)]
    fn generate_minf(&self, params: &TrackParameters) -> Result<mp4::Boxes, TryFromIntError> {
        /*
           minf
           - vmhd
           - dinf
             - dref
               - url
             - stbl
               - stsd
               - stts
               - stss
               - ctts
               - stsc
               - stsz
               - stco
        */

        let stbl = mp4::Boxes::new(mp4::Stbl {}).with_children7(
            // Stsd.
            generate_stsd(params),
            // Stts.
            mp4::Boxes::new(mp4::Stts {
                full_box: mp4::FullBox::default(),
                entries: self.stts.clone(),
            }),
            // Stss.
            mp4::Boxes::new(mp4::Stss {
                full_box: mp4::FullBox::default(),
                sample_numbers: self.stss.clone(),
            }),
            // Ctts.
            mp4::Boxes::new(mp4::Ctts {
                full_box: mp4::FullBox {
                    version: 1,
                    flags: [0, 0, 0],
                },
                entries: self.ctts.clone(),
            }),
            // Stsc.
            mp4::Boxes::new(mp4::Stsc {
                full_box: mp4::FullBox::default(),
                entries: self.stsc.clone(),
            }),
            // Stsz.
            mp4::Boxes::new(mp4::Stsz {
                full_box: mp4::FullBox::default(),
                sample_size: 0,
                sample_count: u32::try_from(self.stsz.len())?,
                entry_sizes: self.stsz.clone(),
            }),
            // Stco.
            mp4::Boxes::new(MyStco {
                full_box: mp4::FullBox::default(),
                chunk_offsets: self.stco.clone(),
            }),
        );

        let minf = mp4::Boxes::new(mp4::Minf).with_children3(
            // Vmhd.
            mp4::Boxes::new(mp4::Vmhd::default()),
            // Dinf.
            mp4::Boxes::new(mp4::Dinf).with_child(
                // Dref.
                mp4::Boxes::new(mp4::Dref {
                    full_box: mp4::FullBox::default(),
                    entry_count: 1,
                })
                .with_child(
                    // Url.
                    mp4::Boxes::new(mp4::Url {
                        full_box: mp4::FullBox {
                            version: 0,
                            flags: [0, 0, 1],
                        },
                        location: String::new(),
                    }),
                ),
            ),
            // Stbl.
            stbl,
        );

        Ok(minf)
    }
}

#[allow(clippy::let_and_return)]
fn generate_stsd(params: &TrackParameters) -> mp4::Boxes {
    /*
       - stsd
         - avc1
           - avcC
    */

    let stsd = mp4::Boxes::new(mp4::Stsd {
        full_box: mp4::FullBox::default(),
        entry_count: 1,
    })
    .with_child(
        // Avc1.
        mp4::Boxes::new(mp4::Avc1 {
            sample_entry: mp4::SampleEntry {
                data_reference_index: 1,
                ..Default::default()
            },
            width: params.width,
            height: params.height,
            horiz_resolution: 4_718_592,
            vert_resolution: 4_718_592,
            frame_count: 1,
            depth: 24,
            pre_defined3: -1,
            ..Default::default()
        })
        .with_child(
            // AvcC.
            mp4::Boxes::new(MyAvcC(params.extra_data.clone())),
        ),
    );

    stsd
}

struct MyAvcC(Vec<u8>);

impl mp4::ImmutableBox for MyAvcC {
    fn box_type(&self) -> mp4::BoxType {
        mp4::TYPE_AVCC
    }

    fn size(&self) -> usize {
        self.0.len()
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), mp4::Mp4Error> {
        w.write_all(&self.0)?;
        Ok(())
    }
}

impl From<MyAvcC> for Box<dyn ImmutableBox> {
    fn from(value: MyAvcC) -> Self {
        Box::new(value)
    }
}

pub struct MyStco {
    pub full_box: FullBox,
    pub chunk_offsets: Rc<RefCell<Vec<u32>>>,
}

impl mp4::ImmutableBox for MyStco {
    fn box_type(&self) -> mp4::BoxType {
        mp4::TYPE_STCO
    }

    fn size(&self) -> usize {
        8 + (self.chunk_offsets.borrow().len()) * 4
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        let chunk_offsets = self.chunk_offsets.borrow().clone();
        w.write_all(
            &u32::try_from(chunk_offsets.len())
                .map_err(|e| Mp4Error::FromInt("stco".to_owned(), e))?
                .to_be_bytes(),
        )?;
        for offset in chunk_offsets {
            w.write_all(&offset.to_be_bytes())?;
        }
        Ok(())
    }
}

impl From<MyStco> for Box<dyn ImmutableBox> {
    fn from(value: MyStco) -> Self {
        Box::new(value)
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::time::DtsOffset;
    use pretty_assertions::assert_eq;
    use pretty_hex::pretty_hex;
    use std::io::Cursor;

    #[test]
    #[allow(clippy::too_many_lines)]
    fn test_generate_mp4() {
        let samples = vec![
            Sample {
                // VideoSample3. B-Frame.
                random_access_present: false,
                pts: UnixH264::new(18),
                dts_offset: DtsOffset::new(-9),
                duration: DurationH264::new(9),
                data_size: 2,
                data_offset: 0,
            },
            Sample {
                // VideoSample2. P-Frame.
                random_access_present: false,
                pts: UnixH264::new(27),
                dts_offset: DtsOffset::new(9),
                duration: DurationH264::new(9),
                data_size: 2,
                data_offset: 0,
            },
            Sample {
                // VideoSample1. I-Frame.
                random_access_present: true,
                pts: UnixH264::new(14),
                dts_offset: DtsOffset::new(5),
                duration: DurationH264::new(9),
                data_size: 2,
                data_offset: 0,
            },
        ];

        let mut buf = Cursor::new(Vec::new());

        let params = TrackParameters {
            width: 650,
            height: 450,
            extra_data: vec![
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
            ],
        };

        let start_time = UnixH264::new(1);
        let mdat_size = generate_mp4(&mut buf, start_time, &samples, &params).unwrap();
        assert_eq!(6, mdat_size);

        let want = vec![
            0, 0, 0, 0x14, b'f', b't', b'y', b'p', //
            b'i', b's', b'o', b'4', //
            0, 0, 2, 0, // Minor version.
            b'i', b's', b'o', b'4', //
            //
            0, 0, 2, 0xad, b'm', b'o', b'o', b'v', //
            0, 0, 0, 0x6c, b'm', b'v', b'h', b'd', //
            0, 0, 0, 0, // Fullbox.
            0, 0, 0, 0, // Creation time.
            0, 0, 0, 0, // Modification time.
            0, 0, 3, 0xe8, // Timescale.
            0, 0, 0, 0, // Duration.
            0, 1, 0, 0, // Rate.
            1, 0, // Volume.
            0, 0, // Reserved.
            0, 0, 0, 0, 0, 0, 0, 0, // Reserved2.
            0, 1, 0, 0, 0, 0, 0, 0, 0, // Matrix.
            0, 0, 0, 0, 0, 0, 0, 0, 1, //
            0, 0, 0, 0, 0, 0, 0, 0, 0, //
            0, 0, 0, 0, 0, 0x40, 0, 0, 0, //
            0, 0, 0, 0, 0, 0, // Pre-defined.
            0, 0, 0, 0, 0, 0, //
            0, 0, 0, 0, 0, 0, //
            0, 0, 0, 0, 0, 0, //
            0, 0, 0, 2, // Next track ID.
            //
            /* Video trak */
            0, 0, 2, 0x39, b't', b'r', b'a', b'k', //
            0, 0, 0, 0x5c, b't', b'k', b'h', b'd', //
            0, 0, 0, 3, // Fullbox.
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
            0, 1, 0, 0, 0, 0, 0, 0, 0, // Matrix.
            0, 0, 0, 0, 0, 0, 0, 0, 1, //
            0, 0, 0, 0, 0, 0, 0, 0, 0, //
            0, 0, 0, 0, 0, 0x40, 0, 0, 0, //
            2, 0x8a, 0, 0, // Width.
            1, 0xc2, 0, 0, // Height.
            0, 0, 1, 0xd5, b'm', b'd', b'i', b'a', //
            0, 0, 0, 0x20, b'm', b'd', b'h', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Creation time.
            0, 0, 0, 0, // Modification time.
            0, 1, 0x5f, 0x90, // Time scale.
            0, 0, 0, 0x11, // Duration.
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
            0, 0, 1, 0x80, b'm', b'i', b'n', b'f', //
            0, 0, 0, 0x14, b'v', b'm', b'h', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, // Graphics mode.
            0, 0, 0, 0, 0, 0, // OpColor.
            0, 0, 0, 0x24, b'd', b'i', b'n', b'f', //
            0, 0, 0, 0x1c, b'd', b'r', b'e', b'f', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 0xc, b'u', b'r', b'l', b' ', //
            0, 0, 0, 1, // FullBox.
            0, 0, 1, 0x40, b's', b't', b'b', b'l', //
            0, 0, 0, 0x94, b's', b't', b's', b'd', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 0x84, b'a', b'v', b'c', b'1', //
            0, 0, 0, 0, 0, 0, // Reserved.
            0, 1, // Data reference index.
            0, 0, // Predefined.
            0, 0, // Reserved.
            0, 0, 0, 0, // Predefined2.
            0, 0, 0, 0, //
            0, 0, 0, 0, //
            2, 0x8a, // Width.
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
            0, 0, 0, 0x18, b's', b't', b't', b's', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 3, // Entry1 sample count.
            0, 0, 0, 9, // Entry1 sample delta.
            0, 0, 0, 0x14, b's', b't', b's', b's', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 3, // Entry1.
            0, 0, 0, 0x28, b'c', b't', b't', b's', //
            1, 0, 0, 0, // FullBox.
            0, 0, 0, 3, // Entry count.
            0, 0, 0, 1, // Entry1 sample count.
            0, 0, 0, 0, // Entry1 sample offset
            0, 0, 0, 1, // Entry2 sample count.
            0, 0, 0, 0x12, // Entry2 sample offset
            0, 0, 0, 1, // Entry3 sample count.
            0, 0, 0, 0xe, // Entry3 sample offset
            0, 0, 0, 0x1c, b's', b't', b's', b'c', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 1, // Entry1 first chunk.
            0, 0, 0, 3, // Entry1 samples per chunk.
            0, 0, 0, 1, // Entry1 sample description index.
            0, 0, 0, 0x20, b's', b't', b's', b'z', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Sample size.
            0, 0, 0, 3, // Sample count.
            0, 0, 0, 2, // Entry1 size.
            0, 0, 0, 2, // Entry2 size.
            0, 0, 0, 2, // Entry3 size.
            0, 0, 0, 0x14, b's', b't', b'c', b'o', //
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 2, 0xc9, // Chunk offset1.
            //
            0, 0, 0, 0x0e, b'm', b'd', b'a', b't', //
        ];

        assert_eq!(pretty_hex(&want), pretty_hex(&buf.into_inner()));
    }
}

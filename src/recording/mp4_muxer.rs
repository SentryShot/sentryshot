// SPDX-License-Identifier: GPL-2.0-or-later

use crate::video::{Sample, TrackParameters};
use common::time::{DurationH264, UnixH264, H264_TIMESCALE};
use hls::VIDEO_TRACK_ID;
use mp4::Mp4Error;
use std::io::Write;
use thiserror::Error;

#[derive(Debug, Error)]
pub enum GenerateMp4Error {
    #[error("mp4: {0}")]
    Mp4(#[from] Mp4Error),
}

// Generates mp4 to writer from samples.
pub fn generate_mp4(
    out: &mut dyn Write,
    start_time: UnixH264,
    samples: Vec<Sample>,
    params: &TrackParameters,
) -> Result<u32, GenerateMp4Error> {
    //bw := bitio.NewByteWriter(out)
    let mut m = Muxer {
        out,
        //videoTrack: videoTrack,
        start_time,
        end_time: UnixH264::from(0),
        first_sample: true,
        dts_shift: 0,
        mdat_pos: 0,
        stts: Vec::new(),
        stss: Vec::new(),
        ctts: Vec::new(),
        stsc: Vec::new(),
        stsz: Vec::new(),
        stco: Vec::new(),
    };

    let ftyp = mp4::Ftyp {
        major_brand: *b"iso4",
        minor_version: 512,
        compatible_brands: vec![mp4::CompatibleBrandElem(*b"iso4")],
    };

    mp4::write_single_box(&mut m.out, &ftyp)?;

    for sample in samples {
        m.write_sample(sample);
    }

    m.finalize(params).unwrap();

    //return int64(m.mdatPos), nil
    Ok(m.mdat_pos)
}

struct Muxer<'a> {
    out: &'a mut dyn Write,

    start_time: UnixH264,
    end_time: UnixH264,
    first_sample: bool,
    dts_shift: i64,
    mdat_pos: u32,
    stts: Vec<mp4::SttsEntry>,
    stss: Vec<u32>,
    ctts: Vec<mp4::CttsEntry>,
    stsc: Vec<mp4::StscEntry>,
    stsz: Vec<u32>,
    stco: Vec<u32>,
}

impl Muxer<'_> {
    fn write_sample(&mut self, sample: Sample) {
        //duration := sample.Next - sample.DTS
        //delta := hls.NanoToTimescale(duration, hls.VideoTimescale)
        let delta = sample.duration.as_u32().unwrap();
        if !self.stts.is_empty() && self.stts.last().unwrap().sample_delta == delta {
            self.stts.last_mut().unwrap().sample_count += 1;
        } else {
            self.stts.push(mp4::SttsEntry {
                sample_count: 1,
                sample_delta: delta,
            })
        }

        let pts = sample.pts.checked_sub(self.start_time).unwrap();
        let dts = sample.dts().unwrap().checked_sub(self.start_time).unwrap();
        //pts := hls.NanoToTimescale(sample.PTS-m.startTime, hls.VideoTimescale)
        //dts := hls.NanoToTimescale(sample.DTS-m.startTime, hls.VideoTimescale)

        if self.first_sample {
            self.dts_shift = *pts.checked_sub(dts).unwrap();
        }
        /*if m.firstSample {
            m.dtsShift = pts - dts
        }*/

        let cts = i32::try_from(
            (*pts)
                .checked_sub(dts.checked_add(self.dts_shift).unwrap())
                .unwrap(),
        )
        .unwrap();
        //cts := pts - (dts + m.dtsShift)

        if !self.ctts.is_empty() && self.ctts.last().unwrap().sample_offset_v1 == cts {
            self.ctts.last_mut().unwrap().sample_count += 1;
        } else {
            self.ctts.push(mp4::CttsEntry {
                sample_count: 1,
                sample_offset_v0: 0,
                sample_offset_v1: cts,
            })
        }

        if self.first_sample {
            self.stco.push(self.mdat_pos);
            self.stsc.push(mp4::StscEntry {
                first_chunk: 1,
                samples_per_chunk: 1,
                sample_description_index: 1,
            });
        } else {
            self.stsc.last_mut().unwrap().samples_per_chunk += 1;
        }

        self.mdat_pos += sample.data_size;
        self.stsz.push(sample.data_size);

        if sample.random_access_present {
            self.stss.push(u32::try_from(self.stsz.len()).unwrap());
        }

        self.end_time = sample
            .dts()
            .unwrap()
            .checked_add_duration(sample.duration)
            .unwrap();

        self.first_sample = false
    }

    fn finalize(&mut self, params: &TrackParameters) -> Result<(), ()> {
        /*
           moov
           - mvhd
           - trak (video)
        */

        let duration = DurationH264::from(self.end_time.checked_sub(self.start_time).unwrap());
        //duration := time.Duration(m.endTime - m.startTime)

        let moov = mp4::Boxes {
            mp4_box: Box::new(mp4::Moov {}),
            children: vec![
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Mvhd {
                        timescale: 1000,
                        duration_v0: u32::try_from(duration.as_millis()).unwrap(),
                        rate: 65536,
                        volume: 256,
                        matrix: [0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000],
                        next_track_id: VIDEO_TRACK_ID + 1,
                        ..Default::default()
                    }),
                    children: vec![],
                },
                self.generate_trak(duration, params),
            ],
        };

        const FTYP_SIZE: u32 = 20;
        const MDAT_HEADER_SIZE: u32 = 8;
        let mdat_offset: u32 = FTYP_SIZE + u32::try_from(moov.size()).unwrap() + MDAT_HEADER_SIZE;

        for stco in self.stco.iter_mut() {
            *stco += mdat_offset;
        }
        /*for i := 0; i < len(m.stco); i++ {
            m.stco[i] += mdatOffset
        }*/

        let moov = mp4::Boxes {
            mp4_box: Box::new(mp4::Moov {}),
            children: vec![
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Mvhd {
                        timescale: 1000,
                        duration_v0: u32::try_from(duration.as_millis()).unwrap(),
                        rate: 65536,
                        volume: 256,
                        matrix: [0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000],
                        next_track_id: VIDEO_TRACK_ID + 1,
                        ..Default::default()
                    }),
                    children: vec![],
                },
                self.generate_trak(duration, params),
            ],
        };

        moov.marshal(&mut self.out).unwrap();

        self.out
            .write_all(&(self.mdat_pos.checked_add(8).unwrap()).to_be_bytes())
            .unwrap();
        self.out.write_all(b"mdat").unwrap();

        Ok(())
    }

    #[allow(clippy::let_and_return)]
    fn generate_trak(&self, duration: DurationH264, params: &TrackParameters) -> mp4::Boxes {
        /*
           trak
           - tkhd
           - mdia
             - mdhd
             - hdlr
             - minf
        */

        let trak = mp4::Boxes {
            mp4_box: Box::new(mp4::Trak {}),
            children: vec![
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Tkhd {
                        full_box: mp4::FullBox {
                            version: 0,
                            flags: [0, 0, 3],
                        },
                        track_id: VIDEO_TRACK_ID,
                        duration_v0: u32::try_from(duration.as_millis()).unwrap(),
                        width: u32::from(params.width) * 65536,
                        height: u32::from(params.height) * 65536,
                        matrix: [0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000],
                        ..Default::default()
                    }),
                    children: vec![],
                },
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Mdia {}),
                    children: vec![
                        mp4::Boxes {
                            mp4_box: Box::new(mp4::Mdhd {
                                timescale: H264_TIMESCALE,
                                language: *b"und",
                                duration_v0: duration.as_u32().unwrap(),
                                ..Default::default()
                            }),
                            children: vec![],
                        },
                        mp4::Boxes {
                            mp4_box: Box::new(mp4::Hdlr {
                                handler_type: *b"vide",
                                name: "VideoHandler".to_owned(),
                                ..Default::default()
                            }),
                            children: vec![],
                        },
                        self.generate_minf(params),
                    ],
                },
            ],
        };

        trak
    }

    #[allow(clippy::let_and_return)]
    fn generate_minf(&self, params: &TrackParameters) -> mp4::Boxes {
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

        let stbl = mp4::Boxes {
            mp4_box: Box::new(mp4::Stbl {}),
            children: vec![
                generate_stsd(params),
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Stts {
                        full_box: mp4::FullBox::default(),
                        entries: self.stts.to_owned(),
                    }),
                    children: vec![],
                },
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Stss {
                        full_box: mp4::FullBox::default(),
                        sample_numbers: self.stss.to_owned(),
                    }),
                    children: vec![],
                },
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Ctts {
                        full_box: mp4::FullBox {
                            version: 1,
                            flags: [0, 0, 0],
                        },
                        entries: self.ctts.to_owned(),
                    }),
                    children: vec![],
                },
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Stsc {
                        full_box: mp4::FullBox::default(),
                        entries: self.stsc.to_owned(),
                    }),
                    children: vec![],
                },
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Stsz {
                        full_box: mp4::FullBox::default(),
                        sample_size: 0,
                        sample_count: u32::try_from(self.stsz.len()).unwrap(),
                        entry_sizes: self.stsz.to_owned(),
                    }),
                    children: vec![],
                },
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Stco {
                        full_box: mp4::FullBox::default(),
                        chunk_offsets: self.stco.to_owned(),
                    }),
                    children: vec![],
                },
            ],
        };

        let minf = mp4::Boxes {
            mp4_box: Box::new(mp4::Minf {}),
            children: vec![
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Vmhd {
                        ..Default::default()
                    }),
                    children: vec![],
                },
                mp4::Boxes {
                    mp4_box: Box::new(mp4::Dinf {}),
                    children: vec![mp4::Boxes {
                        mp4_box: Box::new(mp4::Dref {
                            full_box: mp4::FullBox::default(),
                            entry_count: 1,
                        }),
                        children: vec![mp4::Boxes {
                            mp4_box: Box::new(mp4::Url {
                                full_box: mp4::FullBox {
                                    version: 0,
                                    flags: [0, 0, 1],
                                },
                                location: "".to_owned(),
                            }),
                            children: vec![],
                        }],
                    }],
                },
                stbl,
            ],
        };

        minf
    }
}

#[allow(clippy::let_and_return)]
fn generate_stsd(params: &TrackParameters) -> mp4::Boxes {
    /*
       - stsd
         - avc1
           - avcC
    */

    let stsd = mp4::Boxes {
        mp4_box: Box::new(mp4::Stsd {
            full_box: mp4::FullBox::default(),
            entry_count: 1,
        }),
        children: vec![mp4::Boxes {
            mp4_box: Box::new(mp4::Avc1 {
                sample_entry: mp4::SampleEntry {
                    data_reference_index: 1,
                    ..Default::default()
                },
                width: params.width,
                height: params.height,
                horiz_resolution: 4718592,
                vert_resolution: 4718592,
                frame_count: 1,
                depth: 24,
                pre_defined3: -1,
                ..Default::default()
            }),
            children: vec![mp4::Boxes {
                mp4_box: Box::new(MyAvcC(params.extra_data.to_owned())),
                children: vec![],
            }],
        }],
    };

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

#[cfg(test)]
mod tests {
    use super::*;
    use pretty_assertions::assert_eq;
    use pretty_hex::pretty_hex;
    use std::io::Cursor;

    #[test]
    fn test_generate_mp4() {
        let samples = vec![
            Sample {
                // VideoSample3. B-Frame.
                random_access_present: false,
                pts: UnixH264::from(18),
                dts_offset: DurationH264::from(-9),
                duration: DurationH264::from(9),
                data_size: 2,
                data_offset: 0,
            },
            Sample {
                // VideoSample2. P-Frame.
                random_access_present: false,
                pts: UnixH264::from(27),
                dts_offset: DurationH264::from(9),
                duration: DurationH264::from(9),
                data_size: 2,
                data_offset: 0,
            },
            Sample {
                // VideoSample1. I-Frame.
                random_access_present: true,
                pts: UnixH264::from(14),
                dts_offset: DurationH264::from(5),
                duration: DurationH264::from(9),
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

        let start_time = UnixH264::from(1);
        let mdat_size = generate_mp4(&mut buf, start_time, samples, &params).unwrap();
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

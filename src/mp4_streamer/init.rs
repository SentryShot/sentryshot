// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{error::GenerateInitError, types::VIDEO_TRACK_ID};
use bytes::Bytes;
use common::{time::H264_TIMESCALE, TrackParameters};
use mp4::{ImmutableBox, ImmutableBoxSync};

#[allow(clippy::module_name_repetitions)]
pub fn generate_init(params: &TrackParameters) -> Result<Bytes, GenerateInitError> {
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

    Ok(Bytes::from(buf))
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

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use pretty_assertions::assert_eq;
    use pretty_hex::pretty_hex;

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
}

#![allow(clippy::cast_possible_truncation, clippy::as_conversions)]

use crate::*;
use pretty_assertions::assert_eq;
use test_case::test_case;

#[test_case(
        Box::new(Btrt{
            buffer_size_db: 0x1234_5678,
            max_bitrate: 0x3456_789a,
            avg_bitrate: 0x5678_9abc,
        }),
        &[
            0x12, 0x34, 0x56, 0x78, // buffer_size_db.
            0x34, 0x56, 0x78, 0x9a, // max_bitrate.
            0x56, 0x78, 0x9a, 0xbc, // avg_bitrate.
        ]; "btrt"
    )]
#[test_case(
        Box::new(Ctts{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            entries: vec![
                CttsEntry{
                    sample_count: 0x0123_4567,
                    sample_offset_v0: 0x1234_5678,
                    sample_offset_v1: 0,
                },
                CttsEntry{
                    sample_count: 0x89ab_cdef,
                    sample_offset_v0: 0x789a_bcde,
                    sample_offset_v1: 0,
                },
            ],
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x00, 0x00, 0x00, 0x02, // entry count
            0x01, 0x23, 0x45, 0x67, // sample count
            0x12, 0x34, 0x56, 0x78, // sample offset
            0x89, 0xab, 0xcd, 0xef, // sample count
            0x78, 0x9a, 0xbc, 0xde, // sample offset
        ]; "ctts: version 0"
    )]
#[test_case(
        Box::new(Ctts{
            full_box: FullBox{
                version: 1,
                flags: [0, 0, 0],
            },
            entries: vec![
                CttsEntry{
                    sample_count: 0x0123_4567,
                    sample_offset_v0: 0,
                    sample_offset_v1: 0x1234_5678,
                },
                CttsEntry{
                    sample_count: 0x89ab_cdef,
                    sample_offset_v0: 0,
                    sample_offset_v1: -0x789a_bcde,
                },
            ],
        }),
        &[
            1,                // version
            0x00, 0x00, 0x00, // flags
            0x00, 0x00, 0x00, 0x02, // entry count
            0x01, 0x23, 0x45, 0x67, // sample count
            0x12, 0x34, 0x56, 0x78, // sample offset
            0x89, 0xab, 0xcd, 0xef, // sample count
            0x87, 0x65, 0x43, 0x22, // sample offset
        ]; "ctts: version 1"
    )]
#[test_case(Box::new(Dinf{}), &[]; "dinf")]
#[test_case(
        Box::new(Dref{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            entry_count: 0x1234_5678,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x12, 0x34, 0x56, 0x78, // entry count
        ]; "dref"
    )]
#[test_case(
        Box::new(Url{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 1],
            },
            location: String::new(),
        }),
        &[
            0,                // version
            0x00, 0x00, 0x01, // flags
        ]; "url"
    )]
#[test_case(
        Box::new(Ftyp{
            major_brand:   [b'a', b'b', b'e', b'm'],
            minor_version: 0x1234_5678,
            compatible_brands: vec![
                CompatibleBrandElem(*b"abcd"),
                CompatibleBrandElem(*b"efgh"),
            ],
        }),
        &[
            b'a', b'b', b'e', b'm', // major brand
            0x12, 0x34, 0x56, 0x78, // minor version
            b'a', b'b', b'c', b'd', // compatible brand
            b'e', b'f', b'g', b'h', // compatible brand
        ]; "ftyp"
    )]
#[test_case(
        Box::new(Hdlr{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            pre_defined: 0x1234_5678,
            handler_type: *b"abem",
            reserved: [0, 0, 0],
            name: "Abema".to_owned(),
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x12, 0x34, 0x56, 0x78, // pre-defined
            b'a', b'b', b'e', b'm', // handler type
            0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, // reserved
            b'A', b'b', b'e', b'm', b'a', 0x00, // name
        ]; "hdlr"
    )]
#[test_case(
        Box::new(Hdlr{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            pre_defined: 0,
            handler_type: *b"vide",
            reserved: [0, 0, 0],
            name: "VideoHandler".to_owned(),
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x00, 0x00, 0x00, 0x00, // pre-defined
            b'v', b'i', b'd', b'e', // handler type
            0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, // reserved
            b'V', b'i', b'd', b'e', b'o', b'H', b'a', b'n', b'd', b'l', b'e', b'r', 0x00, // name
        ];"hdlr2"
    )]
#[test_case(
        Box::new(Mdat(vec![0x11, 0x22, 0x33])),
        &[0x11, 0x22, 0x33];
        "mdat"
    )]
#[test_case(
        Box::new(Mdhd{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            creation_time_v0: 0x1234_5678,
            modification_time_v0: 0x2345_6789,
            creation_time_v1: 0,
            modification_time_v1: 0,
            timescale: 0x0102_0304,
            duration_v0: 0x0203_0405,
            duration_v1: 0,
            pad: true,
            language: [b'j' - 0x60, b'p' - 0x60, b'n' - 0x60], // 0x0a, 0x10, 0x0e
            pre_defined: 0,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x12, 0x34, 0x56, 0x78, // creation time
            0x23, 0x45, 0x67, 0x89, // modification time
            0x01, 0x02, 0x03, 0x04, // timescale
            0x02, 0x03, 0x04, 0x05, // duration
            0xaa, 0x0e, // pad, language (1 01010 10000 01110)
            0x00, 0x00, // pre defined
        ]; "mdhd: version 0"
    )]
#[test_case(
        Box::new(Mdhd{
            full_box: FullBox{
                version: 1,
                flags: [0, 0, 0],
            },
            creation_time_v0: 0,
            modification_time_v0: 0,
            creation_time_v1: 0x1234_5678_9abc_def0,
            modification_time_v1: 0x2345_6789_abcd_ef01,
            timescale: 0x0102_0304,
            duration_v0: 0,
            duration_v1: 0x0203_0405_0607_0809,
            pad: true,
            language: [b'j' - 0x60, b'p' - 0x60, b'n' - 0x60], // 0x0a, 0x10, 0x0e
            pre_defined: 0,
        }),
        &[
            1,                // version
            0x00, 0x00, 0x00, // flags
            0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, // creation time
            0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, // modification time
            0x01, 0x02, 0x03, 0x04, // timescale
            0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, // duration
            0xaa, 0x0e, // pad, language (1 01010 10000 01110)
            0x00, 0x00, // pre defined
        ]; "mdhd: version 1"
    )]
#[test_case(
        Box::new(Mdhd{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            creation_time_v0: 0,
            creation_time_v1: 0,
            modification_time_v0: 0,
            modification_time_v1: 0,
            timescale: 0x0102_0304,
            duration_v0: 0,
            duration_v1: 0,
            pad: false,
            language: *b"und",
            pre_defined: 0,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x0, 0x0, 0x0, 0x0, // creation time
            0x0, 0x0, 0x0, 0x0, // modification time
            0x01, 0x02, 0x03, 0x04, // timescale
            0x00, 0x00, 0x00, 0x00, // duration
            0x55, 0xc4, // pad, language
            0x00, 0x00, // pre defined
        ]; "mdhd: language"
    )]
#[test_case(Box::new(Mdia{}), &[]; "mdia")]
#[test_case(
        Box::new(Mfhd{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            sequence_number: 0x1234_5678,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x12, 0x34, 0x56, 0x78, // sequence number
        ]; "mfhd"
    )]
#[test_case(Box::new(Minf{}), &[]; "minf")]
#[test_case(Box::new(Moof{}), &[]; "moof")]
#[test_case(Box::new(Moov{}), &[]; "moov")]
#[test_case(Box::new(Mvex{}), &[]; "mvex")]
#[test_case(
        Box::new(Mvhd{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            creation_time_v0: 0x0123_4567,
            modification_time_v0: 0x2345_6789,
            creation_time_v1: 0,
            modification_time_v1: 0,
            timescale: 0x4567_89ab,
            duration_v0: 0x6789_abcd,
            duration_v1: 0,
            rate: -0x0123_4567,
            volume: 0x0123,
            reserved: 0,
            reserved2: [0; 2],
            matrix: [0; 9],
            pre_defined: [0;6],
            next_track_id: 0xabcd_ef01,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, // creation time
            0x23, 0x45, 0x67, 0x89, // modification time
            0x45, 0x67, 0x89, 0xab, // timescale
            0x67, 0x89, 0xab, 0xcd, // duration
            0xfe, 0xdc, 0xba, 0x99, // rate
            0x01, 0x23, // volume
            0x00, 0x00, // reserved
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // matrix
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pre-defined
            0xab, 0xcd, 0xef, 0x01, // next track ID
        ]; "mvhd: version 0"
    )]
#[test_case(
        Box::new(Mvhd{
            full_box: FullBox{
                version: 1,
                flags: [0, 0, 0],
            },
            creation_time_v0: 0,
            modification_time_v0: 0,
            creation_time_v1: 0x0123_4567_89ab_cdef,
            modification_time_v1: 0x2345_6789_abcd_ef01,
            timescale: 0x89ab_cdef,
            duration_v0: 0,
            duration_v1: 0x4567_89ab_cdef_0123,
            rate: -0x0123_4567,
            volume: 0x0123,
            reserved: 0,
            reserved2: [0; 2],
            matrix: [0; 9],
            pre_defined: [0; 6],
            next_track_id: 0xabcd_ef01,
        }),
        &[
            1,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, // creation time
            0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, // modification
            0x89, 0xab, 0xcd, 0xef, // timescale
            0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, // duration
            0xfe, 0xdc, 0xba, 0x99, // rate
            0x01, 0x23, // volume
            0x00, 0x00, // reserved
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // matrix
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pre-defined
            0xab, 0xcd, 0xef, 0x01, // next track ID
        ]; "mvhd: version 1"
    )]
#[test_case(
        Box::new(Avc1{
            sample_entry: SampleEntry{
                reserved: [0; 6],
                data_reference_index: 0x1234,
            },
            pre_defined: 0x0101,
            pre_defined2: [0x0100_0001, 0x0100_0002, 0x0100_0003],
            reserved: 0,
            width: 0x0102,
            height: 0x0103,
            horiz_resolution: 0x0100_0004,
            vert_resolution: 0x0100_0005,
            reserved2: 0x0100_0006,
            frame_count: 0x0104,
            compressor_name: [8, b'a', b'b', b'e', b'm', b'a', 0x00, b't', b'v',
                0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
            depth: 0x0105,
            pre_defined3: 1001,
        }),
        &[
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
            0x12, 0x34, // data reference index
            0x01, 0x01, // pre_defined
            0x00, 0x00, // reserved
            0x01, 0x00, 0x00, 0x01,
            0x01, 0x00, 0x00, 0x02,
            0x01, 0x00, 0x00, 0x03, // pre_defined2
            0x01, 0x02, // width
            0x01, 0x03, // height
            0x01, 0x00, 0x00, 0x04, // horiz_resolution
            0x01, 0x00, 0x00, 0x05, // vert_resolution
            0x01, 0x00, 0x00, 0x06, // reserved2
            0x01, 0x04, // frame_count
            8, b'a', b'b', b'e', b'm', b'a', 0x00, b't',
            b'v', 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // compressor_name
            0x01, 0x05, // depth
            0x03, 0xe9, // pre_defined3
        ]; "Avc1"
    )]
#[test_case(
        Box::new(AvcC{
            configuration_version: 0x12,
            profile: AVC_MAIN_PROFILE,
            profile_compatibility: 0x40,
            level: 0x1f,
            reserved: 0x3f,
            length_size_minus_one: 0x2,
            reserved2: 0x7,
            num_of_sequence_parameter_sets: 2,
            sequence_parameter_sets: vec![
                AvcParameterSet(vec![0x12, 0x34]),
                AvcParameterSet(vec![0x12, 0x34, 0x56]),
            ],
            num_of_picture_parameter_sets: 2,
            picture_parameter_sets: vec![
                AvcParameterSet(vec![0xab, 0xcd]),
                AvcParameterSet(vec![0xab, 0xcd, 0xef]),
            ],
            high_profile_fields_enabled: false,
            reserved3: 0,
            chroma_format: 0,
            reserved4: 0,
            bitdepth_luma_minus_8: 0,
            reserved5: 0,
            bitdepth_chroma_minus_8: 0,
            num_of_sequence_parameter_set_ext: 0,
            sequence_parameter_sets_ext: vec![],
        }),
        &[
            0x12,       // configuration version
            0x4d,       // profile
            0x40,       // profile compatibility
            0x1f,       // level
            0xfe,       // reserved,  lengthSizeMinusOne
            0xe2,       // reserved, numOfsequence_parameter_sets
            0x00, 0x02, // length
            0x12, 0x34, // nalUnit
            0x00, 0x03, // length
            0x12, 0x34, 0x56, // nalUnit
            0x02,       // reserved, numOfsequence_parameter_sets
            0x00, 0x02, // length
            0xab, 0xcd, // nalUnit
            0x00, 0x03, // length
            0xab, 0xcd, 0xef, // nalUnit
        ]; "AvcC main profile"
    )]
#[test_case(
        Box::new(AvcC{
            configuration_version: 0x12,
            profile: AVC_HIGH_PROFILE,
            profile_compatibility: 0x00,
            level: 0x28,
            reserved: 0x3f,
            length_size_minus_one: 0x2,
            reserved2: 0x7,
            num_of_sequence_parameter_sets: 2,
            sequence_parameter_sets: vec![
                AvcParameterSet(vec![0x12, 0x34]),
                AvcParameterSet(vec![0x12, 0x34, 0x56]),
            ],
            num_of_picture_parameter_sets: 2,
            picture_parameter_sets: vec![
                AvcParameterSet(vec![0xab, 0xcd]),
                AvcParameterSet(vec![0xab, 0xcd, 0xef]),
            ],
            high_profile_fields_enabled: false,
            reserved3: 0,
            chroma_format: 0,
            reserved4: 0,
            bitdepth_luma_minus_8: 0,
            reserved5: 0,
            bitdepth_chroma_minus_8: 0,
            num_of_sequence_parameter_set_ext: 0,
            sequence_parameter_sets_ext: vec![],
        }),
        &[
            0x12,       // configuration version
            0x64,       // profile
            0x00,       // profile compatibility
            0x28,       // level
            0xfe,       // reserved,  lengthSizeMinusOne
            0xe2,       // reserved, numOfsequence_parameter_sets
            0x00, 0x02, // length
            0x12, 0x34, // nalUnit
            0x00, 0x03, // length
            0x12, 0x34, 0x56, // nalUnit
            0x02,       // reserved, numOfsequence_parameter_sets
            0x00, 0x02, // length
            0xab, 0xcd, // nalUnit
            0x00, 0x03, // length
            0xab, 0xcd, 0xef, // nalUnit
        ]; "AvcC high profile old spec"
    )]
#[test_case(
        Box::new(AvcC{
            configuration_version: 0x12,
            profile: AVC_HIGH_PROFILE,
            profile_compatibility: 0x00,
            level: 0x28,
            reserved: 0x3f,
            length_size_minus_one: 0x2,
            reserved2: 0x7,
            num_of_sequence_parameter_sets: 2,
            sequence_parameter_sets: vec![
                AvcParameterSet(vec![0x12, 0x34]),
                AvcParameterSet(vec![0x12, 0x34, 0x56]),
            ],
            num_of_picture_parameter_sets: 2,
            picture_parameter_sets: vec![
                AvcParameterSet(vec![0xab, 0xcd]),
                AvcParameterSet(vec![0xab, 0xcd, 0xef]),
            ],
            high_profile_fields_enabled: true,
            reserved3: 0x3f,
            chroma_format: 0x1,
            reserved4: 0x1f,
            bitdepth_luma_minus_8: 0x2,
            reserved5: 0x1f,
            bitdepth_chroma_minus_8: 0x3,
            num_of_sequence_parameter_set_ext: 2,
            sequence_parameter_sets_ext: vec![
                AvcParameterSet(vec![0x12, 0x34]),
                AvcParameterSet(vec![0x12, 0x34, 0x56]),
            ],
        }),
        &[
            0x12,       // configuration version
            0x64,       // profile
            0x00,       // profile compatibility
            0x28,       // level
            0xfe,       // reserved,  lengthSizeMinusOne
            0xe2,       // reserved, numOfsequence_parameter_sets
            0x00, 0x02, // length
            0x12, 0x34, // nalUnit
            0x00, 0x03, // length
            0x12, 0x34, 0x56, // nalUnit
            0x02,       // numOfsequence_parameter_sets
            0x00, 0x02, // length
            0xab, 0xcd, // nalUnit
            0x00, 0x03, // length
            0xab, 0xcd, 0xef, // nalUnit
            0xfd,       // reserved, chromaFormat
            0xfa,       // reserved, bitdepthLumaMinus8
            0xfb,       // reserved, bitdepthChromaMinus8
            0x02,       // numOfsequence_parameter_sets
            0x00, 0x02, // length
            0x12, 0x34, // nalUnit
            0x00, 0x03, // length
            0x12, 0x34, 0x56, // nalUnit
        ]; "AvcC high profile new spec"
    )]
#[test_case(Box::new(Stbl{}), &[]; "stbl")]
#[test_case(
        Box::new(Stco{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            chunk_offsets: vec![0x0123_4567, 0x89ab_cdef],
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x00, 0x00, 0x00, 0x02, // entry count
            0x01, 0x23, 0x45, 0x67, // chunk offset
            0x89, 0xab, 0xcd, 0xef, // chunk offset
        ]; "stco"
    )]
#[test_case(
        Box::new(Stsc{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            entries: vec![
                StscEntry{first_chunk: 0x0123_4567, samples_per_chunk: 0x2345_6789, sample_description_index: 0x4567_89ab},
                StscEntry{first_chunk: 0x6789_abcd, samples_per_chunk: 0x89ab_cdef, sample_description_index: 0xabcd_ef01},
            ],
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x00, 0x00, 0x00, 0x02, // entry count
            0x01, 0x23, 0x45, 0x67, // first chunk
            0x23, 0x45, 0x67, 0x89, // sample per chunk
            0x45, 0x67, 0x89, 0xab, // sample description index
            0x67, 0x89, 0xab, 0xcd, // first chunk
            0x89, 0xab, 0xcd, 0xef, // sample per chunk
            0xab, 0xcd, 0xef, 0x01, // sample description index
        ]; "stsc"
    )]
#[test_case(
        Box::new(Stsd{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            entry_count: 0x0123_4567,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, // entry count
        ]; "stsd"
    )]
#[test_case(
        Box::new(Stss{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            sample_numbers: vec![0x0123_4567, 0x89ab_cdef],
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x00, 0x00, 0x00, 0x02, // entry count
            0x01, 0x23, 0x45, 0x67, // sample number
            0x89, 0xab, 0xcd, 0xef, // sample number
        ]; "stss"
    )]
#[test_case(
        Box::new(Stsz{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            sample_size:  0x0123_4567,
            sample_count: 2,
            entry_sizes: vec![],
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, // sample size
            0x00, 0x00, 0x00, 0x02, // sample count
        ]; "stsz: common sample size"
    )]
#[test_case(
        Box::new(Stsz{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            sample_size: 0,
            sample_count: 2,
            entry_sizes:  vec![0x0123_4567, 0x2345_6789],
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x00, 0x00, 0x00, 0x00, // sample size
            0x00, 0x00, 0x00, 0x02, // sample count
            0x01, 0x23, 0x45, 0x67, // entry size
            0x23, 0x45, 0x67, 0x89, // entry size
        ]; "stsz: sample size array"
    )]
#[test_case(
        Box::new(Stts{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            entries: vec![
                SttsEntry{sample_count: 0x0123_4567, sample_delta: 0x2345_6789},
                SttsEntry{sample_count: 0x4567_89ab, sample_delta: 0x6789_abcd},
            ],
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x00, 0x00, 0x00, 0x02, // entry count
            0x01, 0x23, 0x45, 0x67, // sample count
            0x23, 0x45, 0x67, 0x89, // sample delta
            0x45, 0x67, 0x89, 0xab, // sample count
            0x67, 0x89, 0xab, 0xcd, // sample delta
        ]; "stts"
    )]
#[test_case(
        Box::new(Tfdt{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            base_media_decode_time_v0: 0x0123_4567,
            base_media_decode_time_v1: 0,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, // base media decode time
        ]; "tfdt: version 0"
    )]
#[test_case(
        Box::new(Tfdt{
            full_box: FullBox{
                version: 1,
                flags: [0, 0, 0],
            },
            base_media_decode_time_v0: 0,
            base_media_decode_time_v1: 0x0123_4567_89ab_cdef,
        }),
        &[
            1,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, // base media decode time
        ]; "tfdt: version 1"
    )]
#[test_case(
        Box::new(Tfhd{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            track_id: 0x0840_4649,
            base_data_offset: 0,
            sample_descroption_index: 0,
            default_sample_duration: 0,
            default_sample_size: 0,
            default_sample_flags: 0,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x08, 0x40, 0x46, 0x49, // track ID
        ]; "tfhd: no flags"
    )]
#[test_case(
        Box::new(Tfhd{
            full_box: FullBox{
                version: 0,
                flags: [
                    0,
                    0,
                    (TFHD_BASE_DATA_OFFSET_PRESENT | TFHD_DEFAULT_SAMPLE_DURATION_PRESENT) as u8,
                ],
            },
            track_id: 0x0840_4649,
            base_data_offset: 0x0123_4567_89ab_cdef,
            sample_descroption_index: 0,
            default_sample_duration: 0x2345_6789,
            default_sample_size: 0,
            default_sample_flags: 0,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x09, // flags (0000 0000 1001)
            0x08, 0x40, 0x46, 0x49, // track ID
            0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
            0x23, 0x45, 0x67, 0x89,
        ]; "tfhd: base data offset & default sample duration"
    )]
#[test_case(
        Box::new(Tkhd{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            creation_time_v0: 0x0123_4567,
            modification_time_v0: 0x1234_5678,
            creation_time_v1: 0,
            modification_time_v1: 0,
            track_id: 0x2345_6789,
            reserved0: 0x3456_789a,
            duration_v0: 0x4567_89ab,
            duration_v1: 0,
            reserved1: [0, 0],
            layer: 23456,  // 0x5ba0
            alternate_group: -23456, // 0xdba0
            volume: 0x0100,
            reserved2: 0,
            matrix: [
                0x0001_0000, 0, 0,
                0, 0x0001_0000, 0,
                0, 0, 0x4000_0000,
            ],
            width:  125_829_120,
            height: 70_778_880,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, // creation time
            0x12, 0x34, 0x56, 0x78, // modification time
            0x23, 0x45, 0x67, 0x89, // track ID
            0x34, 0x56, 0x78, 0x9a, // reserved
            0x45, 0x67, 0x89, 0xab, // duration
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
            0x5b, 0xa0, // layer
            0xa4, 0x60, // alternate group
            0x01, 0x00, // volume
            0x00, 0x00, // reserved
            0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, // matrix
            0x07, 0x80, 0x00, 0x00, // width
            0x04, 0x38, 0x00, 0x00, // height
        ]; "tkhd version 0"
    )]
#[test_case(
        Box::new(Tkhd{
            full_box: FullBox{
                version: 1,
                flags: [0, 0, 0],
            },
            creation_time_v0: 0,
            modification_time_v0: 0,
            creation_time_v1: 0x0123_4567_89ab_cdef,
            modification_time_v1: 0x1234_5678_9abc_def0,
            track_id: 0x2345_6789,
            reserved0: 0x3456_789a,
            duration_v0: 0,
            duration_v1: 0x4567_89ab_cdef_0123,
            reserved1: [0, 0],
            layer: 23456,  // 0x5ba0
            alternate_group: -23456, // 0xdba0
            volume: 0x0100,
            reserved2: 0,
            matrix: [
                0x0001_0000, 0, 0,
                0, 0x0001_0000, 0,
                0, 0, 0x4000_0000,
            ],
            width:  125_829_120,
            height: 70_778_880,
        }),
        &[
            1,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, // creation time
            0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, // modification time
            0x23, 0x45, 0x67, 0x89, // track ID
            0x34, 0x56, 0x78, 0x9a, // reserved
            0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, // duration
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
            0x5b, 0xa0, // layer
            0xa4, 0x60, // alternate group
            0x01, 0x00, // volume
            0x00, 0x00, // reserved
            0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, // matrix
            0x07, 0x80, 0x00, 0x00, // width
            0x04, 0x38, 0x00, 0x00, // height
        ]; "tkhd version 1"
    )]
#[test_case(Box::new(Traf{}), &[]; "traf")]
#[test_case(Box::new(Trak{}), &[]; "trak")]
#[test_case(
        Box::new(Trex{
            full_box: FullBox{
                version: 0,
                flags: [0, 0, 0],
            },
            track_id: 0x0123_4567,
            default_sample_description_index: 0x2345_6789,
            default_sample_duration: 0x4567_89ab,
            default_sample_size: 0x6789_abcd,
            default_sample_flags: 0x89ab_cdef,
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, 0x45, 0x67, // track ID
            0x23, 0x45, 0x67, 0x89, // default sample description index
            0x45, 0x67, 0x89, 0xab, // default sample duration
            0x67, 0x89, 0xab, 0xcd, // default sample size
            0x89, 0xab, 0xcd, 0xef, // default sample flags
        ]; "trex"
    )]
#[test_case(
        Box::new(Trun{
            full_box: FullBox{
                version: 0,
                flags: [0, 1, 1],
            },
            data_offset: 50,
            first_sample_flags: 0,
            entries: vec![
                TrunEntry{
                    sample_duration: 100,
                    sample_size: 0,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: 0,
                },
                TrunEntry{
                    sample_duration: 101,
                    sample_size: 0,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: 0,
                },
                TrunEntry{
                    sample_duration: 102,
                    sample_size: 0,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: 0,
                },
            ],
        }),
        &[
            0,                // version
            0x00, 0x01, 0x01, // flags
            0x00, 0x00, 0x00, 0x03, // sample count
            0x00, 0x00, 0x00, 0x32, // data offset
            0x00, 0x00, 0x00, 0x64, // sample duration
            0x00, 0x00, 0x00, 0x65, // sample duration
            0x00, 0x00, 0x00, 0x66, // sample duration
        ]; "trun: version=0 flag=0x101"
    )]
#[test_case(
        Box::new(Trun{
            full_box: FullBox{
                version: 0,
                flags: [0, 2, 4],
            },
            data_offset: 0,
            first_sample_flags: 0x0246_8ace,
            entries: vec![
                TrunEntry{
                    sample_duration: 0,
                    sample_size: 100,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: 0,
                },
                TrunEntry{
                    sample_duration: 0,
                    sample_size: 101,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: 0,
                },
                TrunEntry{
                    sample_duration: 0,
                    sample_size: 102,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: 0,
                },
            ],
        }),
        &[
            0,                // version
            0x00, 0x02, 0x04, // flags
            0x00, 0x00, 0x00, 0x03, // sample count
            0x02, 0x46, 0x8a, 0xce, // first sample flags
            0x00, 0x00, 0x00, 0x64, // sample size
            0x00, 0x00, 0x00, 0x65, // sample size
            0x00, 0x00, 0x00, 0x66, // sample size
        ]; "trun: version=0 flag=0x204"
    )]
#[test_case(
        Box::new(Trun{
            full_box: FullBox{
                version: 0,
                flags: [0x00, 0x0c, 0x00],
            },
            data_offset: 0,
            first_sample_flags: 0,
            entries: vec![
                TrunEntry{
                    sample_duration: 0,
                    sample_size: 0,
                    sample_flags: 100,
                    sample_composition_time_offset_v0: 200,
                    sample_composition_time_offset_v1: 0,
                },
                TrunEntry{
                    sample_duration: 0,
                    sample_size: 0,
                    sample_flags: 101,
                    sample_composition_time_offset_v0: 201,
                    sample_composition_time_offset_v1: 0,
                },
                TrunEntry{
                    sample_duration: 0,
                    sample_size: 0,
                    sample_flags: 102,
                    sample_composition_time_offset_v0: 202,
                    sample_composition_time_offset_v1: 0,
                },
            ],
        }),
        &[
            0,                // version
            0x00, 0x0c, 0x00, // flags
            0x00, 0x00, 0x00, 0x03, // sample count
            0x00, 0x00, 0x00, 0x64, // sample flags
            0x00, 0x00, 0x00, 0xc8, // sample composition time offset
            0x00, 0x00, 0x00, 0x65, // sample flags
            0x00, 0x00, 0x00, 0xc9, // sample composition time offset
            0x00, 0x00, 0x00, 0x66, // sample flags
            0x00, 0x00, 0x00, 0xca, // sample composition time offset
        ]; "trun: version=0 flag=0xc00"
    )]
#[test_case(
        Box::new(Trun{
            full_box: FullBox{
                version: 1,
                flags:   [0, 8, 0],
            },
            data_offset: 0,
            first_sample_flags: 0,
            entries: vec![
                TrunEntry{
                    sample_duration: 0,
                    sample_size: 0,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: 200,
                },
                TrunEntry{                    sample_duration: 0,
                    sample_size: 0,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: 201,
                },
                TrunEntry{                    sample_duration: 0,
                    sample_size: 0,
                    sample_flags: 0,
                    sample_composition_time_offset_v0: 0,
                    sample_composition_time_offset_v1: -202,
                },
            ],
        }),
        &[
            1,                // version
            0x00, 0x08, 0x00, // flags
            0x00, 0x00, 0x00, 0x03, // sample count
            0x00, 0x00, 0x00, 0xc8, // sample composition time offset
            0x00, 0x00, 0x00, 0xc9, // sample composition time offset
            0xff, 0xff, 0xff, 0x36, // sample composition time offset
        ]; "trun: version=1 flag=0x800"
    )]
#[test_case(
        Box::new(Vmhd{
            full_box: FullBox{
                version: 0,
                flags:   [0, 0, 0],
            },
            graphics_mode: 0x0123,
            opcolor:      [0x2345, 0x4567, 0x6789],
        }),
        &[
            0,                // version
            0x00, 0x00, 0x00, // flags
            0x01, 0x23, // graphics mode
            0x23, 0x45, 0x45, 0x67, 0x67, 0x89, // opcolor
        ]; "vmhd"
    )]
fn test_box_types(src: Box<dyn ImmutableBox>, bin: &[u8]) {
    let size = src.size();
    let boxes = Boxes {
        mp4_box: src,
        children: vec![],
    };

    let mut buf = Vec::<u8>::with_capacity(size);
    boxes.mp4_box.marshal(&mut buf).unwrap();

    assert_eq!({ size }, buf.len());
    assert_eq!(bin, buf);
}

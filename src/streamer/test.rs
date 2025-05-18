#![allow(clippy::unwrap_used)]

use crate::muxer::StartSessionResponseReady;

use super::*;
use common::{
    DummyLogger, StreamerMuxer, VideoSample,
    monitor::H264WriterImpl,
    time::{DtsOffset, DurationH264, UnixH264},
};
use pretty_assertions::assert_eq;
use pretty_hex::pretty_hex;

use sentryshot_padded_bytes::PaddedBytes;

fn test_params() -> TrackParameters {
    TrackParameters {
        width: 64,
        height: 64,
        codec: "test_codec".to_owned(),
        extra_data: Vec::new(),
    }
}

fn m_id(s: &'static str) -> MonitorId {
    s.to_owned().try_into().unwrap()
}

#[tokio::test]
async fn test_new_muxer() {
    let token = CancellationToken::new();
    let streamer = Streamer::new(token.clone(), DummyLogger::new());

    let first_sample = H264Data {
        random_access_present: true,
        ..Default::default()
    };

    let (_, _writer) = streamer
        .new_muxer2(
            token.clone(),
            m_id("test"),
            false,
            test_params(),
            UnixH264::new(0),
            first_sample,
        )
        .await
        .unwrap()
        .unwrap();

    assert!(streamer.muxer(m_id("x"), false).await.unwrap().is_none());
    assert!(streamer.muxer(m_id("test"), false).await.unwrap().is_some());

    token.cancel();
    while streamer.muxer(m_id("x"), false).await.is_some() {}
}

#[tokio::test]
async fn test_start_session() {
    let token = CancellationToken::new();
    let streamer = Streamer::new(token.clone(), DummyLogger::new());

    let first_sample = H264Data {
        pts: UnixH264::new(3),
        random_access_present: true,
        ..Default::default()
    };
    let second_sample = H264Data {
        pts: UnixH264::new(4),
        random_access_present: true,
        ..Default::default()
    };

    let (muxer, mut writer) = streamer
        .new_muxer2(
            token.clone(),
            m_id("test"),
            false,
            test_params(),
            UnixH264::new(0),
            first_sample,
        )
        .await
        .unwrap()
        .unwrap();

    assert_eq!(0, muxer.debug_state().await.num_sessions);
    assert_eq!(
        streamer.start_session(m_id("test"), false, 123).await,
        StartSessionReponse::NotReady
    );
    assert_eq!(0, muxer.debug_state().await.num_sessions);

    writer.write_h264(second_sample).await.unwrap();

    assert!(matches!(
        streamer.start_session(m_id("test"), false, 123).await,
        StartSessionReponse::Ready(_)
    ));
    assert_eq!(1, muxer.debug_state().await.num_sessions);
    assert_eq!(
        streamer.start_session(m_id("test"), false, 123).await,
        StartSessionReponse::SessionAlreadyExist
    );
    assert_eq!(1, muxer.debug_state().await.num_sessions);

    for i in 0..Muxer::MAX_SESSIONS + 5 {
        assert!(matches!(
            streamer
                .start_session(m_id("test"), false, u32::try_from(i).unwrap())
                .await,
            StartSessionReponse::Ready(_)
        ));
    }
    assert_eq!(Muxer::MAX_SESSIONS, muxer.debug_state().await.num_sessions);
}

#[tokio::test]
async fn test_next_segment() {
    let token = CancellationToken::new();
    let streamer = Streamer::new(token.clone(), DummyLogger::new());

    let first_sample = H264Data {
        pts: UnixH264::new(5),
        dts_offset: DtsOffset::new(0),
        random_access_present: true,
        avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
    };
    let second_sample = H264Data {
        pts: UnixH264::new(6),
        dts_offset: DtsOffset::new(0),
        random_access_present: true,
        avcc: Arc::new(PaddedBytes::new(b"efgh".to_vec())),
    };
    let third_sample = H264Data {
        pts: UnixH264::new(7),
        dts_offset: DtsOffset::new(0),
        random_access_present: true,
        avcc: Arc::new(PaddedBytes::new(b"ijkl".to_vec())),
    };

    let (muxer, mut writer) = streamer
        .new_muxer2(
            token.clone(),
            m_id("test"),
            false,
            test_params(),
            UnixH264::new(3),
            first_sample,
        )
        .await
        .unwrap()
        .unwrap();

    assert_eq!(0, muxer.debug_state().await.gop_count);
    writer.write_h264(second_sample).await.unwrap();
    assert_eq!(1, muxer.debug_state().await.gop_count);

    let seg = muxer.next_segment(None).await.unwrap();
    let got: Vec<_> = seg.frames().collect();
    let want = [&VideoSample {
        pts: UnixH264::new(3),
        dts_offset: DtsOffset::new(0),
        random_access_present: true,
        duration: DurationH264::new(3),
        avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
    }];
    assert_eq!(want.as_slice(), got.as_slice());

    let muxer2 = muxer.clone();
    let seg = tokio::spawn(async move { muxer2.next_segment(Some(seg)).await });
    while muxer.debug_state().await.next_seg_on_hold_count != 1 {}

    writer.write_h264(third_sample).await.unwrap();
    assert_eq!(2, muxer.debug_state().await.gop_count);

    let seg = seg.await.unwrap().unwrap();
    assert_eq!(muxer.debug_state().await.next_seg_on_hold_count, 0);
    let got: Vec<_> = seg.frames().collect();
    let want = [&VideoSample {
        pts: UnixH264::new(6),
        dts_offset: DtsOffset::new(0),
        random_access_present: true,
        duration: DurationH264::new(1),
        avcc: Arc::new(PaddedBytes::new(b"efgh".to_vec())),
    }];
    assert_eq!(want.as_slice(), got.as_slice());
}

#[tokio::test]
#[allow(clippy::too_many_lines)]
async fn test_play() {
    let token = CancellationToken::new();
    let streamer = Streamer::new(token.clone(), DummyLogger::new());

    let first_sample = H264Data {
        pts: UnixH264::new(5),
        dts_offset: DtsOffset::new(0),
        random_access_present: true,
        avcc: Arc::new(PaddedBytes::new(b"abcd".to_vec())),
    };
    let second_sample = H264Data {
        pts: UnixH264::new(6),
        dts_offset: DtsOffset::new(0),
        random_access_present: true,
        avcc: Arc::new(PaddedBytes::new(b"efgh".to_vec())),
    };
    let third_sample = H264Data {
        pts: UnixH264::new(7),
        dts_offset: DtsOffset::new(0),
        random_access_present: false,
        avcc: Arc::new(PaddedBytes::new(b"ijkl".to_vec())),
    };
    let fourth_sample = H264Data {
        pts: UnixH264::new(9),
        dts_offset: DtsOffset::new(0),
        random_access_present: false,
        avcc: Arc::new(PaddedBytes::new(b"mnop".to_vec())),
    };

    let (muxer, mut writer) = streamer
        .new_muxer2(
            token.clone(),
            m_id("test"),
            false,
            test_params(),
            UnixH264::new(3),
            first_sample,
        )
        .await
        .unwrap()
        .unwrap();

    writer.write_h264(second_sample).await.unwrap();

    assert_eq!(
        StartSessionResponseReady {
            start_time: UnixH264::new(3).into(),
            codecs: "test_codec".to_owned(),
        },
        streamer
            .start_session(m_id("test"), false, 123)
            .await
            .unwrap()
    );

    let want_bytes = &[
        0, 0, 0, 0x20, b'f', b't', b'y', b'p', //
        b'm', b'p', b'4', b'2', // Major brand.
        0, 0, 0, 1, // Minor version.
        b'm', b'p', b'4', b'1', // Compatible brand.
        b'm', b'p', b'4', b'2', // Compatible brand.
        b'i', b's', b'o', b'm', // Compatible brand.
        b'h', b'l', b's', b'f', // Compatible brand.
        0, 0, 2, 0x63, b'm', b'o', b'o', b'v', //
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
        0, 0, 1, 0xc7, b't', b'r', b'a', b'k', // Video.
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
        0, 0x40, 0, 0, // Width
        0, 0x40, 0, 0, // Height
        0, 0, 1, 0x63, b'm', b'd', b'i', b'a', //
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
        0, 0, 1, 0x0e, b'm', b'i', b'n', b'f', //
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
        0, 0, 0, 0xce, b's', b't', b'b', b'l', //
        0, 0, 0, 0x82, b's', b't', b's', b'd', //
        0, 0, 0, 0, // FullBox.
        0, 0, 0, 1, // Entry count.
        0, 0, 0, 0x72, b'a', b'v', b'c', b'1', //
        0, 0, 0, 0, 0, 0, // Reserved.
        0, 1, // Data reference index.
        0, 0, // Predefined.
        0, 0, // Reserved.
        0, 0, 0, 0, // Predefined2.
        0, 0, 0, 0, 0, 0, 0, 0, 0, 0x40, // Width.
        0, 0x40, // Height.
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
        0, 0, 0, 0x8, b'a', b'v', b'c', b'C', //
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
        //
        //
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
        0, 0, 0, 3, // Entry1 sample duration.
        0, 0, 0, 4, // Entry1 sample size.
        0, 0, 0, 0, // Entry1 sample flags.
        0, 0, 0, 0, // 1 Entry SampleCompositionTimeOffset
        0, 0, 0, 0xc, b'm', b'd', b'a', b't', //
        b'a', b'b', b'c', b'd', // Samples
    ];

    let got = streamer.play(m_id("test"), false, 123).await.unwrap();
    assert_eq!(
        pretty_hex(&want_bytes.as_slice()),
        pretty_hex(&got.collect().await)
    );

    writer.write_h264(third_sample).await.unwrap();
    let want_bytes = [
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
        0, 0, 0, 0, 0, 0, 0, 3, // BaseMediaDecodeTime.
        0, 0, 0, 0x24, b't', b'r', b'u', b'n', // Video trun.
        1, 0, 0xf, 1, // FullBox.
        0, 0, 0, 1, // Sample count.
        0, 0, 0, 0x70, // Data offset.
        0, 0, 0, 1, // Entry1 sample duration.
        0, 0, 0, 4, // Entry1 sample size.
        0, 0, 0, 0, // Entry1 sample flags.
        0, 0, 0, 0, // 1 Entry SampleCompositionTimeOffset
        0, 0, 0, 0xc, b'm', b'd', b'a', b't', //
        b'e', b'f', b'g', b'h', // Samples
    ];
    let got = streamer.play(m_id("test"), false, 123).await.unwrap();
    assert_eq!(
        pretty_hex(&want_bytes.as_slice()),
        pretty_hex(&got.collect().await)
    );

    let want_bytes = [
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
        0, 0, 0, 0, 0, 0, 0, 4, // BaseMediaDecodeTime.
        0, 0, 0, 0x24, b't', b'r', b'u', b'n', // Video trun.
        1, 0, 0xf, 1, // FullBox.
        0, 0, 0, 1, // Sample count.
        0, 0, 0, 0x70, // Data offset.
        0, 0, 0, 2, // Entry1 sample duration.
        0, 0, 0, 4, // Entry1 sample size.
        0, 1, 0, 0, // Entry1 sample flags.
        0, 0, 0, 0, // 1 Entry SampleCompositionTimeOffset
        0, 0, 0, 0xc, b'm', b'd', b'a', b't', //
        b'i', b'j', b'k', b'l', // Samples
    ];

    let streamer2 = streamer.clone();
    let got = tokio::spawn(async move { streamer2.play(m_id("test"), false, 123).await.unwrap() });
    while muxer.debug_state().await.frame_on_hold_count != 1 {}

    writer.write_h264(fourth_sample).await.unwrap();

    assert_eq!(
        pretty_hex(&want_bytes.as_slice()),
        pretty_hex(&got.await.unwrap().collect().await)
    );
}

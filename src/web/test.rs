// SPDX-License-Identifier: GPL-2.0-or-later

use crate::serve_mp4_content;
use http::{header, HeaderMap, HeaderValue, Method, StatusCode};
use http_body::Body;
use std::{io::Cursor, time::UNIX_EPOCH};
use test_case::test_case;

#[tokio::test]
async fn test_serve_mp4() {
    let file = vec![0, 1, 2, 3, 4, 5, 6, 7, 8, 9];

    let headers = HeaderMap::new();
    let response = serve_mp4_content(
        &Method::GET,
        &headers,
        UNIX_EPOCH,
        10,
        Cursor::new(file.to_owned()),
    )
    .await;

    let body = response.into_body().data().await.unwrap().unwrap().to_vec();
    assert_eq!(file, body);
}

const TEST_FILE_LEN: usize = 11;

struct WantRange {
    start: usize,
    end: usize,
}

#[test_case("", StatusCode::OK, vec![])]
#[test_case("bytes=0-4", StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: 0, end: 5 }])]
#[test_case("bytes=2-" , StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: 2, end: TEST_FILE_LEN }])]
#[test_case("bytes=-5" , StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: TEST_FILE_LEN - 5, end: TEST_FILE_LEN }])]
#[test_case("bytes=3-7", StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: 3, end: 8 }])]
//#[test_case("bytes=0-0,-2", StatusCode::PARTIAL_CONTENT, vec![wantRange{{0, 1}, {testFileLen - 2, testFileLen}}}
//#[test_case("bytes=0-1,5-8", StatusCode::PARTIAL_CONTENT, ranges: []wantRange{{0, 2}, {5, 9}}}
//#[test_case("bytes=0-1,5-", StatusCode::PARTIAL_CONTENT, ranges: []wantRange{{0, 2}, {5, testFileLen}}}
#[test_case("bytes=5-1000", StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: 5, end: TEST_FILE_LEN }])]
// Ignore wasteful range request.
#[test_case("bytes=0-,1-,2-,3-,4-", StatusCode::OK, vec![])]
#[test_case("bytes=0-9"  , StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: 0, end: TEST_FILE_LEN-1 }])]
#[test_case("bytes=0-10" , StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: 0, end: TEST_FILE_LEN }])]
#[test_case("bytes=0-11" , StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: 0, end: TEST_FILE_LEN }])]
#[test_case("bytes=10-11", StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: TEST_FILE_LEN-1, end: TEST_FILE_LEN }])]
#[test_case("bytes=10-"  , StatusCode::PARTIAL_CONTENT, vec![WantRange{ start: TEST_FILE_LEN-1, end: TEST_FILE_LEN }])]
#[test_case("bytes=11-"  , StatusCode::RANGE_NOT_SATISFIABLE, vec![] )]
#[test_case("bytes=11-12", StatusCode::RANGE_NOT_SATISFIABLE, vec![] )]
#[test_case("bytes=12-12", StatusCode::RANGE_NOT_SATISFIABLE, vec![] )]
#[test_case("bytes=11-100"  , StatusCode::RANGE_NOT_SATISFIABLE, vec![] )]
#[test_case("bytes=12-100"  , StatusCode::RANGE_NOT_SATISFIABLE, vec![] )]
#[test_case("bytes=100-"    , StatusCode::RANGE_NOT_SATISFIABLE, vec![] )]
#[test_case("bytes=100-1000", StatusCode::RANGE_NOT_SATISFIABLE, vec![] )]
#[tokio::test]
async fn test_serve_mp4_range(r: &str, code: StatusCode, ranges: Vec<WantRange>) {
    let mut headers = HeaderMap::new();
    if !r.is_empty() {
        headers.insert(header::RANGE, HeaderValue::from_str(r).unwrap());
    }

    let file = b"0123456789.";

    let response = serve_mp4_content(
        &Method::GET,
        &headers,
        UNIX_EPOCH,
        11,
        Cursor::new(file.to_owned()),
    )
    .await;

    assert_eq!(code, response.status());

    let got_headers = response.headers();

    if ranges.len() == 1 {
        let want_content_range = format!(
            "bytes {}-{}/{}",
            ranges[0].start,
            ranges[0].end - 1,
            TEST_FILE_LEN
        );
        let got_content_range = got_headers
            .get(header::CONTENT_RANGE)
            .unwrap()
            .to_str()
            .unwrap();
        assert_eq!(want_content_range, got_content_range);
    }

    let got_content_type = got_headers
        .get(header::CONTENT_TYPE)
        .unwrap()
        .to_str()
        .unwrap()
        .to_owned();
    if ranges.len() == 1 {
        let rng = &ranges[0];
        let want_body = &file[rng.start..rng.end];
        let got_body = response.into_body().data().await.unwrap().unwrap().to_vec();
        assert_eq!(want_body, got_body);

        if got_content_type == "multipart/byteranges" {
            panic!(
                "range={} content-type = {}; unexpected multipart/byteranges",
                r, got_content_type,
            );
        }
    }
}

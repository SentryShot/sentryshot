// SPDX-License-Identifier: GPL-2.0-or-later

use fs::{MapEntry, MapFs};
use pretty_assertions::assert_eq;
use recording::RecordingData;
use std::{collections::HashMap, path::PathBuf};
use test_case::test_case;

use crate::{Crawler, CrawlerQuery};

fn map_fs_item(path: &str) -> (PathBuf, MapEntry) {
    (
        PathBuf::from(path),
        MapEntry {
            is_file: true,
            ..Default::default()
        },
    )
}

fn crawler_test_fs() -> MapFs {
    MapFs(HashMap::from([
        map_fs_item("2000/01/01/m1/2000-01-01_01-01-11_m1.json"),
        map_fs_item("2000/01/01/m1/2000-01-01_01-01-22_m1.json"),
        map_fs_item("2000/01/02/m1/2000-01-02_01-01-11_m1.json"),
        map_fs_item("2000/02/01/m1/2000-02-01_01-01-11_m1.json"),
        map_fs_item("2001/02/01/m1/2001-02-01_01-01-11_m1.json"),
        map_fs_item("2002/01/01/m1/2002-01-01_01-01-11_m1.json"),
        map_fs_item("2003/01/01/m1/2003-01-01_01-01-11_m1.json"),
        map_fs_item("2003/01/01/m2/2003-01-01_01-01-11_m2.json"),
        map_fs_item("2004/01/01/m1/2004-01-01_01-01-11_m1.json"),
        map_fs_item("2004/01/01/m1/2004-01-01_01-01-22_m1.json"),
        (
            PathBuf::from("2099/01/01/m1/2099-01-01_01-01-11_m1.json"),
            MapEntry {
                data: CRAWLER_TEST_DATA.as_bytes().to_owned(),
                is_file: true,
                ..Default::default()
            },
        ),
    ]))
}

const CRAWLER_TEST_DATA: &str = "
    {
        \"start\": 4073680922000000000,
        \"end\": 4073680924000000000,
        \"events\": [
            {
                \"time\": 4073680922000000000,
                \"detections\": [
                    {
                        \"label\": \"a\",
                        \"score\": 1,
                        \"region\": {
                            \"rect\": [2, 3, 4, 5]
                        }
                    }
                ],
                \"duration\": 6
            }
        ]
    }";

#[test_case("0000-01-01_01-01-01_m1",    "";                       "no files")]
#[test_case("1999-01-01_01-01-01_m1",    "";                       "EOF")]
#[test_case("9999-01-01_01-01-01_m1",    "2099-01-01_01-01-11_m1"; "latest")]
#[test_case("2000-01-01_01-01-22_m1", "2000-01-01_01-01-11_m1"; "prev")]
#[test_case("2000-01-02_01-01-11_m1", "2000-01-01_01-01-22_m1"; "prev day")]
#[test_case("2000-02-01_01-01-11_m1", "2000-01-02_01-01-11_m1"; "prev month")]
#[test_case("2001-01-01_01-01-11_m1", "2000-02-01_01-01-11_m1"; "prev year")]
#[test_case("2002-12-01_01-01-01_m1",    "2002-01-01_01-01-11_m1"; "empty prev day")]
#[test_case("2004-01-01_01-01-22_m1",    "2004-01-01_01-01-11_m1"; "same day")]
#[tokio::test]
async fn test_recording_by_query(input: &str, want: &str) {
    let query = CrawlerQuery {
        recording_id: input.parse().unwrap(),
        limit: 1,
        reverse: false,
        monitors: Vec::new(),
        include_data: false,
    };
    let recordings = match Crawler::new(Box::new(crawler_test_fs()))
        .recordings_by_query(&query)
        .await
    {
        Ok(v) => v,
        Err(e) => {
            println!("{e}");
            panic!("{e}");
        }
    };

    if want.is_empty() {
        assert!(recordings.is_empty());
    } else {
        let got = &recordings.first().unwrap().id;
        assert_eq!(want, got);
    }
}

#[test_case("1111-01-01_01-01-01_m1",    "2000-01-01_01-01-11_m1"; "latest")]
#[test_case("2000-01-01_01-01-11_m1", "2000-01-01_01-01-22_m1"; "next")]
#[test_case("2000-01-01_01-01-22_m1", "2000-01-02_01-01-11_m1"; "next day")]
#[test_case("2000-01-02_01-01-11_m1", "2000-02-01_01-01-11_m1"; "next month")]
#[test_case("2000-02-01_01-01-11_m1", "2001-02-01_01-01-11_m1"; "next year")]
#[test_case("2001-12-01_01-01-01_m1",    "2002-01-01_01-01-11_m1"; "empty next day")]
#[tokio::test]
async fn test_recording_by_query_reverse(input: &str, want: &str) {
    let query = CrawlerQuery {
        recording_id: input.parse().unwrap(),
        limit: 1,
        reverse: true,
        monitors: Vec::new(),
        include_data: false,
    };
    let recordings = match Crawler::new(Box::new(crawler_test_fs()))
        .recordings_by_query(&query)
        .await
    {
        Ok(v) => v,
        Err(e) => {
            println!("{e}");
            panic!("{e}");
        }
    };

    let got = &recordings.first().unwrap().id;
    assert_eq!(want, got);
}

#[tokio::test]
async fn test_recording_by_query_multiple() {
    let c = Crawler::new(Box::new(crawler_test_fs()));
    let recordings = c
        .recordings_by_query(&CrawlerQuery {
            recording_id: "9999-01-01_01-01-01_".parse().unwrap(),
            limit: 5,
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        })
        .await
        .unwrap();

    let mut ids = Vec::new();
    for rec in recordings {
        ids.push(rec.id);
    }

    let want = vec![
        "2099-01-01_01-01-11_m1",
        "2004-01-01_01-01-22_m1",
        "2004-01-01_01-01-11_m1",
        "2003-01-01_01-01-11_m2",
        "2003-01-01_01-01-11_m1",
    ];
    assert_eq!(want, ids);
}

#[tokio::test]
async fn test_recording_by_query_monitors() {
    let c = Crawler::new(Box::new(crawler_test_fs()));
    let recordings = c
        .recordings_by_query(&CrawlerQuery {
            recording_id: "2003-02-01_01-01-11_m1".parse().unwrap(),
            limit: 1,
            reverse: false,
            monitors: vec!["m1".to_owned()],
            include_data: false,
        })
        .await
        .unwrap();
    assert_eq!("2003-01-01_01-01-11_m1", recordings[0].id);
    assert_eq!(1, recordings.len());
}

/*
t.Run("emptyMonitorsNoPanic", func(t *testing.T) {
    c := NewCrawler(crawlerTestFS)
    c.RecordingByQuery(
        &CrawlerQuery{
            Time:     "2003-02-01_1_m1",
            Limit:    1,
            Monitors: []string{""},
        },
    )
})
t.Run("invalidTimeErr", func(t *testing.T) {
    c := NewCrawler(crawlerTestFS)
    _, err := c.RecordingByQuery(
        &CrawlerQuery{Time: "", Limit: 1},
    )
    require.Error(t, err)
})*/

#[tokio::test]
async fn test_recording_by_query_data() {
    let c = Crawler::new(Box::new(crawler_test_fs()));
    let rec = c
        .recordings_by_query(&CrawlerQuery {
            recording_id: "9999-01-01_01-01-01_m1".parse().unwrap(),
            limit: 1,
            reverse: false,
            monitors: Vec::new(),
            include_data: true,
        })
        .await
        .unwrap();

    let want: RecordingData = serde_json::from_str(CRAWLER_TEST_DATA).unwrap();
    println!("{rec:?}");
    let got = rec[0].data.as_ref().unwrap();
    assert_eq!(&want, got);
}

#[tokio::test]
async fn test_recording_by_query_missing_data() {
    let c = Crawler::new(Box::new(crawler_test_fs()));
    let rec = c
        .recordings_by_query(&CrawlerQuery {
            recording_id: "2002-01-01_01-01-01_m1".parse().unwrap(),
            limit: 1,
            reverse: true,
            monitors: Vec::new(),
            include_data: true,
        })
        .await
        .unwrap();
    assert!(rec[0].data.is_none());
}

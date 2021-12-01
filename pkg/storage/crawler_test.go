// Copyright 2020-2021 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; version 2.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package storage

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

/* /testdata/
├── 2000
│   ├── 01
│   │   ├── 01
│   │   │   └── m1
│   │   │       ├── 2000-01-01_1_m1.jpeg
│   │   │       └── 2000-01-01_2_m1.jpeg
│   │   └── 02
│   │       └── m1
│   │           └── 2000-01-02_1_m1.jpeg
│   └── 02
│       └── 01
│           └── m1
│               └── 2000-02-01_1_m1.jpeg
├── 2001
│   └── 01
│       └── 01
│           └── m1
│               └── 2001-02-01_1_m1.jpeg
├── 2002
│   └── 01
│       ├── 01
│       │   └── m1
│       │       └── 2002-01-01_1_m1.jpeg
│       └── 02
├── 2003
│   └── 01
│       └── 01
│           ├── m1
│           │   └── 2003-01-01_1_m1.jpeg
│           └── m2
│               └── 2003-01-01_1_m2.jpeg
├── 2004
│   └── 01
│       └── 01
│           ├── 2004-01-01_1_m1.jpeg
│           └── 2004-01-01_2_m1.jpeg
└── 2099
    └── 01
        └── 01
            └── m1
                ├── 2099-01-01_1_m1.jpeg
                └── 2099-01-01_1_m1.json


*/
func TestRecordingByQuery(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")

		cases := []struct{ name, time, expected string }{
			{"noFiles", "0000-01-01", ""},
			{"EOF", "1999-01-01", ""},
			{"latest", "9999-01-01", "2099-01-01_1_m1"},
			{"prev", "2000-01-01_2_m1", "2000-01-01_1_m1"},
			{"prevDay", "2000-01-02_1_m1", "2000-01-01_2_m1"},
			{"prevMonth", "2000-02-01_1_m1", "2000-01-02_1_m1"},
			{"prevYear", "2001-01-01_1_m1", "2000-02-01_1_m1"},
			{"emptyPrevDay", "2002-12-01", "2002-01-01_1_m1"},
			{"sameDay", "2004-01-01_2", "2004-01-01_1_m1"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				query := &CrawlerQuery{
					Time:  tc.time,
					Limit: 1,
				}
				recordings, _ := c.RecordingByQuery(query)
				var actual string
				if len(recordings) != 0 {
					actual = recordings[0].ID
				}
				if actual != tc.expected {
					t.Fatalf("%s, expected:\n%v.\ngot:\n%v.", tc.name, tc.expected, actual)
				}
			})
		}
	})
	t.Run("reverse", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")

		cases := []struct{ name, time, expected string }{
			{"latest", "1111-01-01", "2000-01-01_1_m1"},
			{"next", "2000-01-01_1_m1", "2000-01-01_2_m1"},
			{"nextDay", "2000-01-01_2_m1", "2000-01-02_1_m1"},
			{"nextMonth", "2000-01-02_1_m1", "2000-02-01_1_m1"},
			{"nextYear", "2000-02-01_1_m1", "2001-02-01_1_m1"},
			{"emptyNextDay", "2001-12-01", "2002-01-01_1_m1"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				query := &CrawlerQuery{
					Time:    tc.time,
					Limit:   1,
					Reverse: true,
				}
				recordings, _ := c.RecordingByQuery(query)
				var actual string
				if len(recordings) != 0 {
					actual = recordings[0].ID
				}
				if actual != tc.expected {
					t.Fatalf("%s, expected:\n%v.\ngot:\n%v.", tc.name, tc.expected, actual)
				}
			})
		}
	})
	t.Run("multiple", func(t *testing.T) {
		expected := strings.ReplaceAll(`
			2099-01-01_1_m1
			2004-01-01_2_m1
			2004-01-01_1_m1
			2003-01-01_1_m2
			2003-01-01_1_m1`,
			"\t", "")

		c := NewCrawler("./testdata/recordings")
		recordings, _ := c.RecordingByQuery(
			&CrawlerQuery{
				Time:  "9999-01-01",
				Limit: 5,
			},
		)

		var actual string
		for _, rec := range recordings {
			actual += "\n" + rec.ID
		}

		if actual != expected {
			t.Fatalf("expected: %v \n got: %v", expected, actual)
		}
	})
	t.Run("monitors", func(t *testing.T) {
		expected := "\n2003-01-01_1_m1"

		c := NewCrawler("./testdata/recordings")
		recordings, _ := c.RecordingByQuery(
			&CrawlerQuery{
				Time:     "2003-02-01_1_m1",
				Limit:    1,
				Monitors: []string{"m1"},
			},
		)

		var actual string
		for _, rec := range recordings {
			actual += "\n" + rec.ID
		}

		if actual != expected {
			t.Fatalf("getAll expected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("emptyMonitorsPanic", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")
		c.RecordingByQuery(
			&CrawlerQuery{
				Time:     "2003-02-01_1_m1",
				Limit:    1,
				Monitors: []string{""},
			},
		)
	})
	t.Run("invalidTimeErr", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")
		_, err := c.RecordingByQuery(
			&CrawlerQuery{Time: "", Limit: 1},
		)
		if err == nil {
			t.Fatal("expected error got: nil")
		}
	})
	t.Run("paths", func(t *testing.T) {
		paths := []string{
			"testdata/recordings",
			"./testdata/recordings",
			"./testdata/recordings/",
		}
		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				c := NewCrawler(path)
				query := &CrawlerQuery{
					Time:  "2003-01-01_1_m1",
					Limit: 1,
				}
				recordings, _ := c.RecordingByQuery(query)
				var actual string
				if len(recordings) != 0 {
					actual = recordings[0].Path
				}

				expected := "storage/recordings/2002/01/01/m1/2002-01-01_1_m1"

				if actual != expected {
					t.Fatalf("%v, expected:\n%v.\ngot:\n%v.", path, expected, actual)
				}
			})
		}
	})
	t.Run("data", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")
		rec, err := c.RecordingByQuery(
			&CrawlerQuery{
				Time:  "9999-01-01",
				Limit: 1,
				Data:  true,
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dataFile, err := os.ReadFile("./testdata/recordings/2099/01/01/m1/2099-01-01_1_m1.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var expected RecordingData
		if err := json.Unmarshal(dataFile, &expected); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := *rec[0].Data

		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("expected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("missingData", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")
		rec, err := c.RecordingByQuery(
			&CrawlerQuery{
				Time:  "2002-01-01",
				Limit: 1,
				Data:  true,
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := rec[0].Data
		if actual != nil {
			t.Fatalf("expected: nil, got: %v", actual)
		}
	})
}

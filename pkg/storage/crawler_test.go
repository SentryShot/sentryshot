// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

		cases := map[string]struct{ time, expected string }{
			"noFiles":      {"0000-01-01", ""},
			"EOF":          {"1999-01-01", ""},
			"latest":       {"9999-01-01", "2099-01-01_1_m1"},
			"prev":         {"2000-01-01_2_m1", "2000-01-01_1_m1"},
			"prevDay":      {"2000-01-02_1_m1", "2000-01-01_2_m1"},
			"prevMonth":    {"2000-02-01_1_m1", "2000-01-02_1_m1"},
			"prevYear":     {"2001-01-01_1_m1", "2000-02-01_1_m1"},
			"emptyPrevDay": {"2002-12-01", "2002-01-01_1_m1"},
			"sameDay":      {"2004-01-01_2", "2004-01-01_1_m1"},
		}

		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				query := &CrawlerQuery{
					Time:  tc.time,
					Limit: 1,
				}
				recordings, _ := c.RecordingByQuery(query)
				var id string
				if len(recordings) != 0 {
					id = recordings[0].ID
				}
				require.Equal(t, id, tc.expected)
			})
		}
	})
	t.Run("reverse", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")

		cases := map[string]struct{ time, expected string }{
			"latest":       {"1111-01-01", "2000-01-01_1_m1"},
			"next":         {"2000-01-01_1_m1", "2000-01-01_2_m1"},
			"nextDay":      {"2000-01-01_2_m1", "2000-01-02_1_m1"},
			"nextMonth":    {"2000-01-02_1_m1", "2000-02-01_1_m1"},
			"nextYear":     {"2000-02-01_1_m1", "2001-02-01_1_m1"},
			"emptyNextDay": {"2001-12-01", "2002-01-01_1_m1"},
		}

		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				query := &CrawlerQuery{
					Time:    tc.time,
					Limit:   1,
					Reverse: true,
				}
				recordings, _ := c.RecordingByQuery(query)
				var id string
				if len(recordings) != 0 {
					id = recordings[0].ID
				}
				require.Equal(t, id, tc.expected)
			})
		}
	})
	t.Run("multiple", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")
		recordings, _ := c.RecordingByQuery(
			&CrawlerQuery{
				Time:  "9999-01-01",
				Limit: 5,
			},
		)

		var id string
		for _, rec := range recordings {
			id += "\n" + rec.ID
		}

		expected := strings.ReplaceAll(`
			2099-01-01_1_m1
			2004-01-01_2_m1
			2004-01-01_1_m1
			2003-01-01_1_m2
			2003-01-01_1_m1`,
			"\t", "")

		require.Equal(t, id, expected)
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

		var id string
		for _, rec := range recordings {
			id += "\n" + rec.ID
		}

		require.Equal(t, id, expected)
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
		require.Error(t, err)
	})
	t.Run("data", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")
		rec, err := c.RecordingByQuery(
			&CrawlerQuery{
				Time:        "9999-01-01",
				Limit:       1,
				IncludeData: true,
			},
		)
		require.NoError(t, err)

		data, err := os.ReadFile("./testdata/recordings/2099/01/01/m1/2099-01-01_1_m1.json")
		require.NoError(t, err)

		var expected RecordingData
		if err := json.Unmarshal(data, &expected); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := *rec[0].Data
		require.Equal(t, actual, expected)
	})
	t.Run("missingData", func(t *testing.T) {
		c := NewCrawler("./testdata/recordings")
		rec, err := c.RecordingByQuery(
			&CrawlerQuery{
				Time:        "2002-01-01",
				Limit:       1,
				IncludeData: true,
			},
		)
		require.NoError(t, err)
		require.Nil(t, rec[0].Data)
	})
}

func TestRecordingIDToPath(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		id := "2001-02-03_04-05-06_x"
		actual, err := RecordingIDToPath(id)
		require.NoError(t, err)

		expected := "2001/02/03/x/2001-02-03_04-05-06_x"
		require.Equal(t, expected, actual)
	})
	t.Run("err", func(t *testing.T) {
		_, err := RecordingIDToPath("")
		require.ErrorIs(t, err, ErrInvalidRecordingID)
	})
}

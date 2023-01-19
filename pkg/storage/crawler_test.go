// SPDX-License-Identifier: GPL-2.0-or-later

package storage

import (
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

var crawlerTestFS = fstest.MapFS{
	"2000/01/01/m1/2000-01-01_1_m1.json": {},
	"2000/01/01/m1/2000-01-01_2_m1.json": {},
	"2000/01/02/m1/2000-01-02_1_m1.json": {},
	"2000/02/01/m1/2000-02-01_1_m1.json": {},
	"2001/02/01/m1/2001-02-01_1_m1.json": {},
	"2002/01/01/m1/2002-01-01_1_m1.json": {},
	"2003/01/01/m1/2003-01-01_1_m1.json": {},
	"2003/01/01/m2/2003-01-01_1_m2.json": {},
	"2004/01/01/m1/2004-01-01_1_m1.json": {},
	"2004/01/01/m1/2004-01-01_2_m1.json": {},
	"2099/01/01/m1/2099-01-01_1_m1.json": {Data: crawlerTestData},
}

var crawlerTestData = []byte(`
{
	"start": "2099-02-02T02:02:02.000000000Z",
	"end": "2099-02-02T02:02:04.000000000Z",
	"events": [
		{
			"time": "2099-02-02T02:02:02.000000000Z",
			"detections": [
				{
					"label": "a",
					"score": 1,
					"region": {
						"rect": [2, 3, 4, 5]
					}
				}
			],
			"duration": 6
		}
	]
}`)

func TestRecordingByQuery(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		cases := map[string]struct{ input, expected string }{
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
					Time:  tc.input,
					Limit: 1,
				}
				recordings, _ := NewCrawler(crawlerTestFS).RecordingByQuery(query)
				var id string
				if len(recordings) != 0 {
					id = recordings[0].ID
				}
				require.Equal(t, tc.expected, id)
			})
		}
	})
	t.Run("reverse", func(t *testing.T) {
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
				recordings, _ := NewCrawler(crawlerTestFS).RecordingByQuery(query)
				var id string
				if len(recordings) != 0 {
					id = recordings[0].ID
				}
				require.Equal(t, id, tc.expected)
			})
		}
	})
	t.Run("multiple", func(t *testing.T) {
		c := NewCrawler(crawlerTestFS)
		recordings, _ := c.RecordingByQuery(
			&CrawlerQuery{
				Time:  "9999-01-01",
				Limit: 5,
			},
		)

		var ids []string
		for _, rec := range recordings {
			ids = append(ids, rec.ID)
		}

		expected := []string{
			"2099-01-01_1_m1",
			"2004-01-01_2_m1",
			"2004-01-01_1_m1",
			"2003-01-01_1_m2",
			"2003-01-01_1_m1",
		}

		require.Equal(t, expected, ids)
	})
	t.Run("monitors", func(t *testing.T) {
		c := NewCrawler(crawlerTestFS)
		recordings, _ := c.RecordingByQuery(
			&CrawlerQuery{
				Time:     "2003-02-01_1_m1",
				Limit:    1,
				Monitors: []string{"m1"},
			},
		)
		require.Equal(t, "2003-01-01_1_m1", recordings[0].ID)
		require.Equal(t, 1, len(recordings))
	})
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
	})
	t.Run("data", func(t *testing.T) {
		c := NewCrawler(crawlerTestFS)
		rec, err := c.RecordingByQuery(
			&CrawlerQuery{
				Time:        "9999-01-01",
				Limit:       1,
				IncludeData: true,
			},
		)
		require.NoError(t, err)

		var expected RecordingData
		if err := json.Unmarshal(crawlerTestData, &expected); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := *rec[0].Data
		require.Equal(t, actual, expected)
	})
	t.Run("missingData", func(t *testing.T) {
		c := NewCrawler(crawlerTestFS)
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

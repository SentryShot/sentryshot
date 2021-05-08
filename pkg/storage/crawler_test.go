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
│               └── 2000-02-01_1_m1.jpeg
├── 2002
│   └── 01
│       ├── 01
│       │   └── m1
│       │       └── 2002-01-01_1_m1.jpeg
│       └── 02
└── 2099
    └── 01
        └── 01
            └── m1
                └── 2099-01-01_1_m1.jpeg
*/
func TestVideosByQuery(t *testing.T) {
	c := NewCrawler("./testdata")

	cases := []struct{ name, query, expected string }{
		{"noFiles", "0000-01-01", ""},
		{"EOF", "1999-01-01", ""},
		{"latest", "9999-01-01", "2099-01-01_1_m1"},
		{"prev", "2000-01-01_2_m1", "2000-01-01_1_m1"},
		{"prevDay", "2000-01-02_1_m1", "2000-01-01_2_m1"},
		{"prevMonth", "2000-02-01_1_m1", "2000-01-02_1_m1"},
		{"prevYear", "2001-01-01_1_m1", "2000-02-01_1_m1"},
		{"emptyPrevDay", "2002-12-01", "2002-01-01_1_m1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recordings, _ := c.RecordingByQuery(1, tc.query)
			var actual string
			if len(recordings) != 0 {
				actual = recordings[0].ID
			}
			if actual != tc.expected {
				t.Fatalf("%s, expected: %v, got: %v", tc.name, tc.expected, actual)
			}
		})
	}

	t.Run("getAll", func(t *testing.T) {
		expected := strings.ReplaceAll(`
			2099-01-01_1_m1
			2002-01-01_1_m1
			2000-02-01_1_m1
			2000-02-01_1_m1
			2000-01-02_1_m1
			2000-01-01_2_m1
			2000-01-01_1_m1`,
			"\t", "")

		recordings, _ := c.RecordingByQuery(10, "9999-01-01")

		var actual string
		for _, rec := range recordings {
			actual += "\n" + rec.ID
		}

		if actual != expected {
			t.Fatalf("getAll expected: %v \n got: %v", expected, actual)
		}
	})
}

func TestPrevSibling(t *testing.T) {
	d := &dir{
		depth:  1,
		parent: &dir{},
	}
	s := d.prevSibling()

	if !s.isNil() {
		t.Fatalf("expected nil, got %v", s)
	}
}

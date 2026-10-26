// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"testing"
	"time"
)

func TestParse_TheTimeAFileSaysItWasRecordedAt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, output string
		want         time.Time
	}{
		{
			"the container's creation_time", `{"format":{"tags":{"creation_time":"2019-07-14T10:31:02.000000Z"}}}`,
			time.Date(2019, 7, 14, 10, 31, 2, 0, time.UTC),
		},
		{
			"a QuickTime creationdate before its creation_time",
			`{"format":{"tags":{"creation_time":"2020-01-01T00:00:00.000000Z",
			"com.apple.quicktime.creationdate":"2019-07-14T12:31:02+0200"}}}`,
			time.Date(2019, 7, 14, 10, 31, 2, 0, time.UTC),
		},
		{
			"the first video track's, when the container says none",
			`{"streams":[{"index":0,"codec_type":"audio","tags":{"creation_time":"2001-01-01T00:00:00Z"}},
			{"index":1,"codec_type":"video","tags":{"creation_time":"2003-05-06 07:08:09"}}]}`,
			time.Date(2003, 5, 6, 7, 8, 9, 0, time.UTC),
		},
		{"QuickTime's zero", `{"format":{"tags":{"creation_time":"1904-01-01T00:00:00.000000Z"}}}`, time.Time{}},
		{"a Unix clock's zero", `{"format":{"tags":{"creation_time":"1970-01-01T00:00:00.000000Z"}}}`, time.Time{}},
		{"nonsense", `{"format":{"tags":{"creation_time":"yesterday"}}}`, time.Time{}},
		{"none", `{"format":{}}`, time.Time{}},
	}
	for _, testCase := range cases {
		record, err := Parse([]byte(testCase.output))
		if err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if !testCase.want.IsZero() {
			want = testCase.want.UnixMilli()
		}
		if got := record.Format.RecordedAtMs; got != want {
			t.Errorf("%s: recorded at %d, want %d", testCase.name, got, want)
		}
	}
}

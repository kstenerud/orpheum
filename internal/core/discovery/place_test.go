// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// A place's namedAt is written in Unix milliseconds, as every other time is, and a trust-discovery a
// core before v0.3.3 wrote, its namedAt an RFC 3339 time, still reads (the owner, 2026-11-09).
func TestPlace_NamedAtIsWrittenInMillisecondsAndAnOlderFilesTimeStillReads(t *testing.T) {
	t.Parallel()
	namedAt := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	encoded, err := json.Marshal(place{URL: "https://discovery.example", NamedAtMs: namedAt.UnixMilli()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := `{"url":"https://discovery.example","namedAt":1794225600000}`; string(encoded) != want {
		t.Errorf("written: %s; want %s", encoded, want)
	}
	for _, written := range []string{
		string(encoded),
		`{"url":"https://discovery.example","namedAt":"2026-11-09T12:00:00Z"}`,
		`{"url":"https://discovery.example","namedAt":"2026-11-09T13:00:00.000000123+01:00"}`,
	} {
		var read place
		if err := strictjson.Unmarshal([]byte(written), &read); err != nil {
			t.Fatalf("read %s: %v", written, err)
		}
		if read.URL != "https://discovery.example" || read.NamedAtMs != namedAt.UnixMilli() {
			t.Errorf("read %s: %+v; want named at %d", written, read, namedAt.UnixMilli())
		}
	}
	var refused place
	if err := strictjson.Unmarshal([]byte(`{"url":"u","namedAt":"yesterday"}`), &refused); err == nil {
		t.Errorf("a namedAt that is no time read as %+v; want an error", refused)
	}
}

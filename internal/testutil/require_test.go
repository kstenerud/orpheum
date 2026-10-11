// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// recorder is a testing.TB whose Fatalf and Skipf are recorded instead of ending the test.
type recorder struct {
	testing.TB

	fatal   string
	skipped string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatalf(format string, args ...any) { r.fatal = fmt.Sprintf(format, args...) }

func (r *recorder) Skipf(format string, args ...any) { r.skipped = fmt.Sprintf(format, args...) }

var errNoFFmpeg = errors.New("no ffprobe in /opt/ffmpeg")

func TestRequire_AnAbsentCapabilityFailsUnlessTheJobListsIt(t *testing.T) {
	cases := []struct {
		name         string
		uncontrolled string
		absent       error
		wantFatal    bool
		wantSkipped  bool
	}{
		{name: "present", uncontrolled: "", absent: nil},
		{name: "present and listed", uncontrolled: "ffmpeg", absent: nil},
		{name: "absent, nothing listed", uncontrolled: "", absent: errNoFFmpeg, wantFatal: true},
		{name: "absent, another listed", uncontrolled: "landlock,ffmpeg-qsv", absent: errNoFFmpeg, wantFatal: true},
		{name: "absent and listed", uncontrolled: "landlock, ffmpeg ,", absent: errNoFFmpeg, wantSkipped: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{TB: t}
			require(rec, tc.uncontrolled, "ffmpeg", tc.absent)
			if got := rec.fatal != ""; got != tc.wantFatal {
				t.Errorf("failed = %v (%q), want %v", got, rec.fatal, tc.wantFatal)
			}
			if got := rec.skipped != ""; got != tc.wantSkipped {
				t.Errorf("skipped = %v (%q), want %v", got, rec.skipped, tc.wantSkipped)
			}
			for _, message := range []string{rec.fatal, rec.skipped} {
				if message != "" && !strings.Contains(message, errNoFFmpeg.Error()) {
					t.Errorf("message %q does not say why the capability is absent", message)
				}
			}
		})
	}
}

func TestRequire_APresentCapabilityPasses(t *testing.T) {
	rec := &recorder{TB: t}
	Require(rec, "ffmpeg", nil)
	if rec.fatal != "" || rec.skipped != "" {
		t.Errorf("a present capability ended the test: fatal %q, skipped %q", rec.fatal, rec.skipped)
	}
}

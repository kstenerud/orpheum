// SPDX-License-Identifier: AGPL-3.0-or-later

package localtime

import (
	"errors"
	"testing"
	"time"
)

// A zone is named, never the machine's or none, and its wall clock follows summer time; the
// moment it reads converts back to the same UTC one (D246 §7, D204).
func TestZone_ReadsTheWallClockOfANamedZoneAndConvertsBack(t *testing.T) {
	t.Parallel()
	for _, refused := range []string{"", "Local", "Europe/Berln", "../etc/passwd"} {
		if _, err := Named(refused); !errors.Is(err, ErrNotAZone) {
			t.Errorf("Named(%q) = %v; want ErrNotAZone", refused, err)
		}
	}
	berlin, err := Named("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	summer := time.Date(2026, 10, 5, 2, 0, 0, 0, time.UTC)
	winter := time.Date(2026, 11, 5, 2, 0, 0, 0, time.UTC)
	if hour := berlin.Of(summer).Hour(); hour != 4 {
		t.Errorf("02:00 UTC in October = %d:00 in Berlin; want 4, summer time", hour)
	}
	if hour := berlin.Of(winter).Hour(); hour != 3 {
		t.Errorf("02:00 UTC in November = %d:00 in Berlin; want 3", hour)
	}
	if back := berlin.Of(summer).UTC(); !back.Equal(summer) || back.Location() != time.UTC {
		t.Errorf("back from Berlin = %v; want %v in UTC", back, summer)
	}
	if name := (Zone{}).Name(); name != "UTC" || UTC().Name() != "UTC" {
		t.Errorf("the zero zone and UTC are named %q and %q; want UTC", name, UTC().Name())
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The hours between a library's checks are at most a week wherever they are set (D541 §3): a new
// library's starting hours, which start at 6, and a watched library's, which start at 24, take 168
// and refuse 169.
func TestRegistry_TheHoursBetweenALibrarysScansAreAtMostAWeek(t *testing.T) {
	t.Parallel()
	if MostHoursBetweenScans != 168 {
		t.Errorf("MostHoursBetweenScans = %d, want 168, a week", MostHoursBetweenScans)
	}
	service, _ := newSeededService(t)
	ctx := t.Context()
	for _, scan := range []struct {
		key      Key
		starting int64
	}{{NewLibraryDefaultScanEveryHours, 6}, {WatchedLibraryScanEveryHours, 24}} {
		key := scan.key
		if got := StartingInt(key); got != scan.starting {
			t.Errorf("%s starts at %d, want %d", key, got, scan.starting)
		}
		if err := service.Set(ctx, jsonByKey(string(key), "169"), nil, false); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("%s set to 169: %v, want ErrInvalidValue", key, err)
		}
		if err := service.Set(ctx, jsonByKey(string(key), "168"), nil, false); err != nil {
			t.Errorf("%s set to 168: %v, want it taken", key, err)
		}
	}
}

// A person who set a library's hours above a week before the most was 168 keeps them: the setting is
// read as it is stored, and the server starts (D541 §3).
func TestRead_HoursSetAboveAWeekBeforeTheMostWasAWeekAreReadAsStored(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	ctx := t.Context()
	storetest.ExecRaw(t, service.database.Path(),
		"UPDATE setting SET value_json = '8760', is_modified = 1 WHERE key = 'watchedLibraryScanEveryHours'")
	if _, err := service.Seed(ctx); err != nil {
		t.Fatalf("seeding over a stored 8760: %v, want the server to start", err)
	}
	if hours, err := service.Int(ctx, WatchedLibraryScanEveryHours); err != nil || hours != 8760 {
		t.Errorf("a stored 8760 reads as %d, %v; want 8760", hours, err)
	}
}

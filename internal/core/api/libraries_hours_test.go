// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

// Every part agrees on the most hours between a library's checks, a week (D541 §3): the API's
// fields for a new library and for a choice made again take from 1 to settings.MostHoursBetweenScans,
// and so do the two settings of a library's scans.
func TestLibraries_TheHoursBetweenChecksAreAtMostTheSettingsWeek(t *testing.T) {
	t.Parallel()
	most := strconv.FormatInt(settings.MostHoursBetweenScans, 10)
	for name, body := range map[string]any{
		"create": api.CreateLibraryInput{}.Body, "change detection": api.ChangeDetectionInput{}.Body,
	} {
		field, ok := reflect.TypeOf(body).FieldByName("CheckEveryHours")
		if !ok {
			t.Fatalf("the %s body has no CheckEveryHours", name)
		}
		if field.Tag.Get("minimum") != "1" || field.Tag.Get("maximum") != most {
			t.Errorf("the %s body's checkEveryHours takes %s to %s; want 1 to %s", name,
				field.Tag.Get("minimum"), field.Tag.Get("maximum"), most)
		}
	}
	scanSettings := []settings.Key{settings.NewLibraryDefaultScanEveryHours, settings.WatchedLibraryScanEveryHours}
	for _, key := range scanSettings {
		definition, ok := settings.Lookup(key)
		if !ok || definition.Minimum != 1 || definition.Maximum != settings.MostHoursBetweenScans {
			t.Errorf("%s takes %d to %d; want 1 to %d", key, definition.Minimum, definition.Maximum,
				settings.MostHoursBetweenScans)
		}
	}
}

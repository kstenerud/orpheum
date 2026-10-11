// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !dev

package main

import "testing"

// A release build sees nothing of its environment but TZ, the machine's time zone, which is only
// the server's default (D204, D246 §7). Not parallel: it sets TZ.
func TestEnvironmentLookup_SeesOnlyTZInARelease(t *testing.T) {
	// PATH is set for every process that can run this test; a release build must still not see it.
	for _, name := range []string{"PATH", "HOME", "ORPHEUM_DEV_TMDB_KEY"} {
		if value, found := environmentLookup()(name); found || value != "" {
			t.Errorf("environmentLookup()(%q) = %q, %v; a release build reads no environment but TZ (D204)",
				name, value, found)
		}
	}
	t.Setenv(timeZoneVariable, "Europe/Berlin")
	if value, found := environmentLookup()(timeZoneVariable); !found || value != "Europe/Berlin" {
		t.Errorf("environmentLookup()(TZ) = %q, %v; want the machine's zone, the default's (D246 §7)", value, found)
	}
}

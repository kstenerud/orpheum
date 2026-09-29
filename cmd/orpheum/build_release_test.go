// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !dev

package main

import "testing"

func TestEnvironmentLookup_IsEmptyInARelease(t *testing.T) {
	// PATH is set for every process that can run this test; a release build must still not see it.
	for _, name := range []string{"PATH", "HOME", "ORPHEUM_DEV_TMDB_KEY"} {
		if value, found := environmentLookup()(name); found || value != "" {
			t.Errorf("environmentLookup()(%q) = %q, %v; a release build reads no environment (D204)",
				name, value, found)
		}
	}
}

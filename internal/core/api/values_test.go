// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "testing"

// A value the database keeps in snake case leaves the API in camelCase, and one the API takes
// reaches the database as it is kept (the owner, 2026-11-09): a job's kind, a library's metadata
// source and an override's field.
func TestValues_TheAPISaysAStoredValueInCamelCaseAndKeepsWhatItTakesAsStored(t *testing.T) {
	t.Parallel()
	for kept, said := range map[string]string{
		"read_through": "readThrough", "make_artwork_sizes": "makeArtworkSizes", "scan": "scan",
		"nfo_then_tmdb": "nfoThenTmdb", "tmdb": "tmdb", "runtime_ms": "runtimeMs", "recorded_at": "recordedAt",
	} {
		if got := saidValue(kept); got != said {
			t.Errorf("saidValue(%q) = %q; want %q", kept, got, said)
		}
		if got := keptValue(said); got != kept {
			t.Errorf("keptValue(%q) = %q; want %q", said, got, kept)
		}
	}
}

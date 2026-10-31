// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// A playback a film's page warmed up, which never played, is no one watching: its file's work is
// the page's to raise, at a look's priority, not a play's (D324 §2; the owner's staging, 2026-10-21,
// where opening 12 Patient Sailors's page raised its read-through to a play's).
func TestFilesPlayed_AreThoseOfThePlaybacksThatPlayed(t *testing.T) {
	t.Parallel()
	files := filesPlayed([]playback.NowPlaying{{FileID: 1, HasPlayed: true}, {FileID: 2}})
	if !slices.Equal(files, []int64{1}) {
		t.Errorf("files played %v; want file 1 alone, file 2 only warmed up", files)
	}
}

// The server wires a way to throw out every thing derived from a file's probe, so a file read again
// whose probe changed leaves nothing made from its old facts (D392 §3).
func TestBuild_EveryThingDerivedFromAProbeCanBeThrownOut(t *testing.T) {
	t.Parallel()
	services, err := Build(t.Context(), testParts(t))
	if err != nil {
		t.Fatal(err)
	}
	if unwired := services.ingester.UnwiredDroppers(); len(unwired) != 0 {
		t.Errorf("the ingest has no way to throw out %v", unwired)
	}
}

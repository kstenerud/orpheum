// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// A playback a film's page warmed up, which never played, is no one watching: its file's work is
// the page's to raise, at a look's priority, not a play's (D324 §2; the owner's staging, 2026-10-21,
// where opening 12 Marrow Larks's page raised its read-through to a play's).
func TestFilesPlayed_AreThoseOfThePlaybacksThatPlayed(t *testing.T) {
	t.Parallel()
	files := filesPlayed([]playback.NowPlaying{{FileID: 1, HasPlayed: true}, {FileID: 2}})
	if !slices.Equal(files, []int64{1}) {
		t.Errorf("files played %v; want file 1 alone, file 2 only warmed up", files)
	}
}

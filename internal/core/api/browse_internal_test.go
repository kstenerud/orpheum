// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// A list's entry says the pictures it shows: its poster, an other video's 16:9 picture, and the still
// a film or series with no poster shows in its place (D349 §3, D362 §2).
func TestEntryBody_SaysEveryPictureTheEntryShows(t *testing.T) {
	t.Parallel()
	body := entryBody(library.Entry{
		ID: 1, Kind: library.ListFilms, PosterSha256: "p", LandscapeSha256: "l", NoPosterStillSha256: "s",
	}, func(int64) string { return "" })
	if body.PosterSha256 != "p" || body.LandscapeSha256 != "l" || body.NoPosterStillSha256 != "s" {
		t.Errorf("the entry's body says poster %q, landscape %q, still %q; want p, l and s", body.PosterSha256,
			body.LandscapeSha256, body.NoPosterStillSha256)
	}
}

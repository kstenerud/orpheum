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
	body := itemEntryBody(library.Entry{
		ID: 1, Kind: library.ListFilms, PosterSha256: "p", LandscapeSha256: "l", NoPosterStillSha256: "s",
	}, func(int64) string { return "" })
	if body.PosterSha256 != "p" || body.LandscapeSha256 != "l" || body.NoPosterStillSha256 != "s" {
		t.Errorf("the entry's body says poster %q, landscape %q, still %q; want p, l and s", body.PosterSha256,
			body.LandscapeSha256, body.NoPosterStillSha256)
	}
}

// A film's or series' page says the pictures it shows, the still a film or series with no poster
// shows in its place among them, as its card in a list does (D362 §2).
func TestArtBody_SaysEveryPictureThePageShows(t *testing.T) {
	t.Parallel()
	body := artBody(library.Art{PosterSha256: "p", BackdropSha256: "b", StillSha256: "s", NoPosterStillSha256: "n"})
	if body != (ArtBody{PosterSha256: "p", BackdropSha256: "b", StillSha256: "s", NoPosterStillSha256: "n"}) {
		t.Errorf("the page's art body is %+v; want p, b, s and n", body)
	}
}

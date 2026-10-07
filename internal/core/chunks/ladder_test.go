// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
)

// startedOnLadder is film 1's playback started on its ladder at 95 s, on a link remembered at
// linkKbps, 0 for none, its picture decodable or not; it fails the test on an error.
func startedOnLadder(t *testing.T, service *Service, linkKbps int64, isDecodable bool) Offer {
	t.Helper()
	offer, err := service.StartOnLadder(t.Context(), Playback{
		FileID: 1, IsSourcePictureDecodable: isDecodable, HasStartingAudio: true,
	}, 95_000, linkKbps)
	if err != nil {
		t.Fatal(err)
	}
	return offer
}

// With no link remembered, a playback starts at the top of its film's ladder, its own picture
// copied, and starts that picture's run and its sound's from where it resumes (D260 §2).
func TestStartOnLadder_StartsAtTheFilmsOwnPictureWithNoLinkRemembered(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	offer := startedOnLadder(t, service, 0, true)
	if offer.Quality != (qualities.Quality{Kbps: 4004, Height: 1080}) || offer.IsBelowTheLowest {
		t.Errorf("the offer = %+v; want the film's own picture, 4 Mb/s at 1080p", offer)
	}
	want := []askedRun{{"picture", 9, 38}, {"sound0-96k", 9, 38}}
	if runs := queue.runs(t, service); !slices.Equal(runs, want) {
		t.Errorf("the runs asked for = %+v; want the picture's and the sound's, from chunk 9", runs)
	}
}

// A link remembered starts the playback at the highest picture within its share: the film's own on
// a fast link, a lower quality on a slow one, the lowest, said to be beyond it, on one too slow for
// any (D257 §1, D260 §2).
func TestStartOnLadder_StartsAtTheHighestPictureTheRememberedLinkCarries(t *testing.T) {
	t.Parallel()
	for _, start := range []struct {
		linkKbps    int64
		want        qualities.Quality
		isBeyondAll bool
	}{
		{linkKbps: 20_000, want: qualities.Quality{Kbps: 4004, Height: 1080}},
		{linkKbps: 2000, want: qualities.Quality{Kbps: 1200, Height: 480}},
		{linkKbps: 300, want: qualities.Quality{Kbps: 400, Height: 360}, isBeyondAll: true},
	} {
		service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
		offer := startedOnLadder(t, service, start.linkKbps, true)
		if offer.Quality != start.want || offer.IsBelowTheLowest != start.isBeyondAll {
			t.Errorf("on a link of %d kb/s, the offer = %+v; want %+v, beyond the link %t", start.linkKbps,
				offer, start.want, start.isBeyondAll)
		}
	}
}

// A player that cannot decode the film's own picture starts at the highest quality made from it.
func TestStartOnLadder_APlayerThatCannotDecodeThePictureStartsAtTheHighestQuality(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	if offer := startedOnLadder(t, service, 0, false); offer.Quality != (qualities.Quality{Kbps: 3000, Height: 720}) {
		t.Errorf("the offer = %+v; want the highest quality below the picture, 3000k at 720p", offer)
	}
}

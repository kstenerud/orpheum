// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// stalledAt is film 1's playback, started with its first audio track, stalled at 95 s on a link of
// linkKbps; it fails the test on an error.
func stalledAt(t *testing.T, service *Service, linkKbps int64) Offer {
	t.Helper()
	offer, err := service.Stalled(t.Context(), Playback{
		FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true, StartingAudioTrackIndex: 0,
	}, 95_000, linkKbps)
	if err != nil {
		t.Fatal(err)
	}
	return offer
}

// A stall offers the highest quality within 70% of the link, named by its playlist's BANDWIDTH, and
// starts its picture's run and the starting sound's from the stall's chunk (D247 §2, §6).
func TestStalled_OffersTheHighestQualityTheLinkCarriesAndStartsItWhereThePlayerIs(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	offer := stalledAt(t, service, 2000) // 1400 kb/s for the picture: 1200k, not 2000k
	if offer.Quality != (qualities.Quality{Kbps: 1200, Height: 480}) || offer.StartAtBps != 1_896_000 ||
		offer.IsBelowTheLowest || offer.SoundCarriedGainDB != -3.5 || offer.FilmPictureKbps != 4004 {
		t.Errorf("the offer = %+v; want 1200k at 480p, its BANDWIDTH 1.5 × 1.2 Mb/s and the sound's, "+
			"the sound's levelling, -3.5 dB, carried in it, and the fixture film's own picture, 4 Mb/s", offer)
	}
	// The stall at 95 s is in chunk 9, from 90 s.
	want := []askedRun{{"q1200k480p", 9, 38}, {"sound0-96k", 9, 38}}
	if runs := queue.runs(t, service); !slices.Equal(runs, want) {
		t.Errorf("the runs asked for = %+v; want the quality's and the sound's, from chunk 9", runs)
	}
}

// With the link not measured the lowest quality is offered; with one too slow even for it, the
// lowest is offered and said to be beyond the link (D247 §8).
func TestStalled_OffersTheLowestWhenTheLinkIsUnknownOrTooSlowForAny(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	if offer := stalledAt(t, service, 0); offer.Quality.Kbps != 400 || offer.IsBelowTheLowest {
		t.Errorf("with the link not measured, the offer = %+v; want the lowest, 400k", offer)
	}
	if offer := stalledAt(t, service, 500); offer.Quality.Kbps != 400 || !offer.IsBelowTheLowest {
		t.Errorf("on a link of 500 kb/s, the offer = %+v; want the lowest, said to be beyond it", offer)
	}
}

// A quality one below the one chosen, which another player's stall is already making there, is
// offered instead (D247 §7).
func TestStalled_TakesTheQualityBelowThatAnotherStallIsAlreadyMaking(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	stalledAt(t, service, 2000) // 1200k, on its way from chunk 9
	if shared := stalledAt(t, service, 3000); shared.Quality.Kbps != 1200 {
		t.Errorf("a second stall on a link for 2000k = %+v; want the 1200k already on its way", shared)
	}
	if offer := stalledAt(t, service, 5000); offer.Quality.Kbps != 3000 {
		t.Errorf("a stall on a link for 3000k = %+v; want 3000k, the one below it not being made", offer)
	}
}

// A film that offers no quality below its own has nothing lower to go to.
func TestStalled_AFilmWithNoLowerQualitySaysSo(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	settingsService, isService := service.settings.(*settings.Service)
	if !isService {
		t.Fatal("the fixture's settings are not the settings service")
	}
	settingstest.Set(t, settingsService, settings.ChunkQualities, `"6000k@1080p"`)
	_, err := service.Stalled(t.Context(), Playback{FileID: 1, IsSourcePictureDecodable: true}, 95_000, 2000)
	if !errors.Is(err, ErrNoQualityOffered) {
		t.Errorf("a stall on a film with no lower quality: %v; want ErrNoQualityOffered", err)
	}
}

// A playback whose link, remembered, is too slow for the film's own picture starts on the quality a
// stall there would, its runs on their way; one whose link carries the picture starts nothing
// (D257 §2). Film 1's picture averages 4 Mb/s, more than 70% of 2 Mb/s and less than 70% of 20.
func TestStartLower_StartsAPlaybackOnALowerQualityOnlyWhenItsLinkIsTooSlowForThePicture(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	playback := Playback{FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true}
	offer, isLower, err := service.StartLower(t.Context(), playback, 95_000, 20_000)
	if err != nil || isLower || len(queue.runs(t, service)) != 0 {
		t.Errorf("on a link of 20 Mb/s: %+v, %t, %v, runs %v; want the original, nothing started", offer, isLower, err,
			queue.runs(t, service))
	}
	offer, isLower, err = service.StartLower(t.Context(), playback, 95_000, 2000)
	if err != nil || !isLower || offer.Quality != (qualities.Quality{Kbps: 1200, Height: 480}) {
		t.Errorf("on a link of 2 Mb/s: %+v, %t, %v; want 1200k at 480p", offer, isLower, err)
	}
	want := []askedRun{{"q1200k480p", 9, 38}, {"sound0-96k", 9, 38}}
	if runs := queue.runs(t, service); !slices.Equal(runs, want) {
		t.Errorf("the runs asked for = %+v; want the quality's and the sound's, from chunk 9", runs)
	}
}

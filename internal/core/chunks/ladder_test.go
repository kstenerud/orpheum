// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
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

// A player that cannot decode the film's own picture starts at its twin, H.264 at its own height at
// the twin's rate, the highest quality made from it, not at the highest quality below the picture
// (D323 §1).
func TestStartOnLadder_APlayerThatCannotDecodeThePictureStartsAtItsTwin(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	offer := startedOnLadder(t, service, 0, false)
	if offer.Quality != (qualities.Quality{Kbps: 4000, Height: 1080}) || offer.StreamName != "q4000k1080p" {
		t.Errorf("the offer = %+v; want the twin, H.264 at the picture's 1080 lines at its 4 Mb/s", offer)
	}
}

// A start bounded by what is left of the server's upload takes the highest picture whose average
// with its sound's is within the bound, says it was held lower than its link carries, and names its
// stream and rate; with no bound it starts as its link says (D286 §5).
func TestStartOnLadder_StartsWithinTheUploadLeft(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	offer, err := service.StartOnLadder(t.Context(), Playback{
		FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true, StartAtMostKbps: 1500,
	}, 95_000, 20_000)
	if err != nil {
		t.Fatal(err)
	}
	if offer.Quality != (qualities.Quality{Kbps: 1200, Height: 480}) || !offer.IsHeldByUpload ||
		offer.StreamName != "q1200k480p" || offer.AverageKbps != 1296 || offer.IsBelowTheLowest {
		t.Errorf("within 1500 kb/s, the offer = %+v; want 480p, 1296 with its sound, held by the upload", offer)
	}
	unbound := startedOnLadder(t, service, 20_000, true)
	if unbound.IsHeldByUpload || unbound.Quality.Height != 1080 {
		t.Errorf("with no bound, the offer = %+v; want the film's own picture, not held", unbound)
	}
	tiny, err := service.StartOnLadder(t.Context(), Playback{
		FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true, StartAtMostKbps: 1,
	}, 95_000, 20_000)
	if err != nil {
		t.Fatal(err)
	}
	if tiny.Quality.Kbps != 400 || !tiny.IsHeldByUpload || tiny.IsBelowTheLowest {
		t.Errorf("with nothing left, the offer = %+v; want the lowest, held by the upload, not beyond the link", tiny)
	}
}

// onAGPU says every node encodes on a GPU, or none does.
type onAGPU bool

func (gpu onAGPU) EncodesOnAGPU(int64) bool { return bool(gpu) }

// A film whose node encodes on a GPU is cut at chunk_qualities_hardware's qualities, one encoding in
// software at chunk_qualities' (D323 §3).
func TestOffered_TheLadderFollowsTheFilmsNodesEncoder(t *testing.T) {
	t.Parallel()
	names := func(gpu onAGPU) []string {
		service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
		service.encoders = gpu
		seeded, isSeeded := service.settings.(*settings.Service)
		if !isSeeded {
			t.Fatalf("the fixture's settings are a %T", service.settings)
		}
		settingstest.Set(t, seeded, settings.ChunkQualitiesHardware, `"2500k@900p,400k@360p"`)
		offered, err := service.Offered(t.Context(), Playback{FileID: 1, IsSourcePictureDecodable: true})
		if err != nil {
			t.Fatal(err)
		}
		var named []string
		for _, picture := range offered.Pictures {
			named = append(named, picture.Name())
		}
		return named
	}
	if got := names(true); !slices.Equal(got, []string{"picture", "q2500k900p", "q400k360p"}) {
		t.Errorf("on a GPU the film offers %v; want its picture and chunk_qualities_hardware's", got)
	}
	if got := names(false); !slices.Equal(got,
		[]string{"picture", "q3000k720p", "q2000k540p", "q1200k480p", "q700k432p", "q400k360p"}) {
		t.Errorf("in software the film offers %v; want its picture and chunk_qualities'", got)
	}
}

// Where the twin plays in an HEVC original's place, the qualities below it step down from the twin's
// rate, not from the original's: a 2.2 Mb/s HEVC film's twin takes 3.3 Mb/s, and the 3 Mb/s quality,
// which is above the original's rate, comes next (the owner, 2026-10-20; D323 §1).
func TestOffered_BelowTheTwinTheQualitiesStepDownFromItsRate(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	picture := nodeproto.ProbeStream{
		Kind: "video", Codec: "hevc", Video: nodeproto.ProbeVideo{Width: 1920, Height: 1080},
	}
	film := cutFilm{
		probe: nodeproto.Probe{
			Format:  nodeproto.ProbeFormat{BitRateBps: 2_200_000},
			Streams: []nodeproto.ProbeStream{picture},
		},
		picture: picture, bytesInEachSecond: []int64{275_000}, kind: OnKeyframes,
	}
	offered, err := service.offerOf(t.Context(), film, Playback{FileID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var named []string
	for _, stream := range offered.pictures {
		named = append(named, stream.Name())
	}
	want := []string{"q3300k1080p", "q3000k720p", "q2000k540p", "q1200k480p", "q700k432p", "q400k360p"}
	if !slices.Equal(named, want) {
		t.Errorf("an HEVC film's player that cannot decode it is offered %v; want %v", named, want)
	}
}

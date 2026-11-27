// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"
	"slices"
	"strings"
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

// A picture whose copy its node failed on twice at one chunk, as file 17099's was refused at the
// bounds of every run (staging, 2026-11-16), is encoded instead: a playback started anew starts at its
// twin, and its master playlist names no copy; a playback offered the copy before still has it, so
// its player is refused the chunk given up, and gives the copy up, rather than losing its playback
// (D500 §2).
func TestStartOnLadder_APictureWhoseCopyCannotBeMadeStartsAtItsTwin(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	copyDir := testStoreDir(t, service, Stream{Kind: nodeproto.StreamPicture}, mustGrid(t, service).ID())
	for range runFailuresToGiveUp {
		service.runs.failed(copyDir, 4, "raw 00000.mp4 holds 0 to 12400 ms of the run, not its chunk 4's")
	}
	if offer := startedOnLadder(t, service, 0, true); offer.StreamName != "q4000k1080p" {
		t.Errorf("the offer = %+v; want the twin, H.264 at the picture's 1080 lines", offer)
	}
	playback := Playback{FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true}
	playlist, _, err := service.MasterPlaylist(t.Context(), playback)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(playlist, "picture/stream.m3u8") || !strings.Contains(playlist, "q4000k1080p/stream.m3u8") {
		t.Errorf("the master playlist names the copy, or not the twin:\n%s", playlist)
	}
	if _, err := service.StoreFileOf(t.Context(), playback, "picture", "000003.m4s"); err != nil {
		t.Errorf("a chunk of the copy made before the one given up is %v; want it the playback's still", err)
	}
}

// giveUpTwin has film 1's twin, H.264 at its 1080 lines at 4 Mb/s, given up at each of chunks, its
// runs having failed there often enough; a run of it was asked for first, as a failure follows one.
func giveUpTwin(t *testing.T, service *Service, chunks ...int) {
	t.Helper()
	twin := Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 4000, Height: 1080}}
	ensured(t, service, twin, chunks[0])
	dir := testStoreDir(t, service, twin, mustGrid(t, service).ID())
	service.runs.forget(dir, chunks[0]) // the run failed
	for _, chunk := range chunks {
		for range runFailuresToGiveUp {
			service.runs.failed(dir, chunk, "ffmpeg: invalid data found when processing input")
		}
	}
}

// A picture encoded whose chunks at the start were given up on starts its playback at the first
// chunk after them not given up, its sound's run too, and the playback wants the film from there, as
// its player, refused the chunks given up, steps over them to play on from it (skip-unmade.ts). Each
// start of a film whose chunk 0 was given up was refused, a 500 (staging, 2026-11-16; the owner,
// 2026-11-18: "skip it").
func TestStartOnLadder_AStartSkipsTheChunksOfItsPictureGivenUpOn(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	giveUpTwin(t, service, 9, 10)
	before := len(queue.runs(t, service))
	offer := startedOnLadder(t, service, 0, false)
	if offer.StreamName != "q4000k1080p" || offer.FromMs != 110_000 {
		t.Errorf("the offer = %+v; want the twin, wanted from chunk 11, at 110 s", offer)
	}
	want := []askedRun{{"q4000k1080p", 11, 40}, {"sound0-96k", 11, 40}}
	if runs := queue.runs(t, service)[before:]; !slices.Equal(runs, want) {
		t.Errorf("the runs asked for = %+v; want the twin's and the sound's, from chunk 11", runs)
	}
}

// A start whose picture's every chunk from where it starts was given up on is refused as its player
// would be, ErrCannotBeMade: there is nothing after them to play.
func TestStartOnLadder_AStartWhoseEveryChunkAheadIsGivenUpCannotBeMade(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	ahead := []int{}
	for chunk := 9; chunk <= mustGrid(t, service).lastChunk(); chunk++ {
		ahead = append(ahead, chunk)
	}
	giveUpTwin(t, service, ahead...)
	_, err := service.StartOnLadder(t.Context(), Playback{FileID: 1, HasStartingAudio: true}, 95_000, 0)
	if !errors.Is(err, ErrCannotBeMade) {
		t.Errorf("a start with every chunk ahead given up = %v; want ErrCannotBeMade", err)
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

func (onAGPU) EncodeSpeedPercent(int64) int { return 0 }

func (onAGPU) SpeedTrials(int64) []nodeproto.SpeedTrial { return nil }

func (onAGPU) SpeedKey(int64) string { return "" }

// A film whose node encodes on a GPU is cut at chunkQualitiesHardware's qualities, one encoding in
// software at chunkQualities' (D323 §3).
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
		t.Errorf("on a GPU the film offers %v; want its picture and chunkQualitiesHardware's", got)
	}
	if got := names(false); !slices.Equal(got,
		[]string{"picture", "q3000k720p", "q2000k540p", "q1200k480p", "q700k432p", "q400k360p"}) {
		t.Errorf("in software the film offers %v; want its picture and chunkQualities'", got)
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

// encodesAt says every node encodes on its processor, the lowest quality at this percent of real
// time; zero is unmeasured.
type encodesAt int

func (encodesAt) EncodesOnAGPU(int64) bool { return false }

func (speed encodesAt) EncodeSpeedPercent(int64) int { return int(speed) }

func (encodesAt) SpeedTrials(int64) []nodeproto.SpeedTrial { return nil }

func (encodesAt) SpeedKey(int64) string { return "" }

// A film whose node encodes the lowest quality slower than chunkRunSlowestSpeedPercent offers
// only its own picture, copied, so a player that decodes it plays it with no encode, and one that
// cannot is refused as too slow (D457): converted, it would stall over and over. A node fast enough,
// or one whose speed is not known, offers its whole ladder; the setting is the line.
func TestStartOnLadder_ANodeTooSlowToEncodeOffersOnlyThePictureCopied(t *testing.T) {
	t.Parallel()
	fixture := func(speed encodesAt) *Service {
		service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
		service.encoders = speed
		return service
	}
	offeredBy := func(service *Service) []string {
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
	slow := fixture(60)
	if got := offeredBy(slow); !slices.Equal(got, []string{"picture"}) {
		t.Errorf("a node encoding at 60%% offers %v; want only the film's picture, copied", got)
	}
	if offer := startedOnLadder(t, slow, 0, true); offer.StreamName != "picture" {
		t.Errorf("on a node too slow, a player that decodes the picture starts at %+v; want it copied", offer)
	}
	if _, err := slow.StartOnLadder(t.Context(), Playback{FileID: 1, HasStartingAudio: true}, 95_000,
		0); !errors.Is(err, ErrTooSlowToEncode) {
		t.Errorf("on a node too slow, a player that cannot decode the picture starts with %v; want "+
			"ErrTooSlowToEncode", err)
	}
	for _, speed := range []encodesAt{0, 110, 400} {
		if got := offeredBy(fixture(speed)); len(got) != 6 {
			t.Errorf("a node encoding at %d%% offers %v; want its picture and every quality", speed, got)
		}
	}
	lowered := fixture(105)
	seeded, isSeeded := lowered.settings.(*settings.Service)
	if !isSeeded {
		t.Fatalf("the fixture's settings are a %T", lowered.settings)
	}
	settingstest.Set(t, seeded, settings.ChunkRunSlowestSpeedPercent, "100")
	if got := offeredBy(lowered); len(got) != 6 {
		t.Errorf("with chunkRunSlowestSpeedPercent at 100, a node at 105%% offers %v; want every quality", got)
	}
}

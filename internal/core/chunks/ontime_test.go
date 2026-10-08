// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestTimeGridOf_StartsAChunkAtEachMultipleOfTheLengthAndMergesAShortLastOne(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		durationMs int64
		want       []int64
	}{
		{10_000, []int64{0, 2000, 4000, 6000, 8000}},
		{9000, []int64{0, 2000, 4000, 6000}}, // the last second is merged into the chunk before
		{1500, []int64{0}},
	} {
		got, err := TimeGridOf(each.durationMs, 2000)
		if err != nil || !slices.Equal(got, each.want) {
			t.Errorf("a film of %d ms cut on time every 2 s = %v, %v; want %v", each.durationMs, got, err, each.want)
		}
	}
	if _, err := TimeGridOf(0, 2000); !errors.Is(err, ErrCannotBeCut) {
		t.Errorf("a film of no known length cut on time = %v; want ErrCannotBeCut", err)
	}
}

// A film whose file has no index of its keyframes is cut on time at once, its playback offered the
// qualities and not the picture copied, and read through ahead of the sweep for its keyframes; with
// no quality below its own picture it is offered one anyway, so it has a picture (D269 §1, §2, §4).
func TestMasterPlaylist_AFilmWhoseFileHasNoIndexIsCutOnTimeWithoutItsPictureCopied(t *testing.T) {
	t.Parallel()
	service, reader := chunksFixture(t, newFakeQueue(), fakeNode{})
	reader.noIndex = [][]byte{{0x02}}
	playback := Playback{FileID: 2, IsSourcePictureDecodable: true}
	text, kind, err := service.MasterPlaylist(t.Context(), playback)
	if err != nil || kind != OnTime || strings.Contains(text, "picture/") || !strings.Contains(text, "q1200k480p/") {
		t.Fatalf("film 2's master playlist =\n%s\n%s, %v; want it cut on time, its qualities and no picture copied",
			text, kind, err)
	}
	if len(reader.asked) != 0 || !slices.Equal(reader.readFirst, []int64{2}) {
		t.Errorf("film 2 was read for play %v and ahead of the sweep %v; want only ahead of the sweep",
			reader.asked, reader.readFirst)
	}
	stream, err := service.MediaPlaylist(t.Context(), playback, "q1200k480p")
	if err != nil || !strings.HasPrefix(strings.SplitN(stream, "#EXTINF:", 2)[1], "2.000,") ||
		strings.Count(stream, "#EXTINF:") != filmMinutes*60/2 {
		t.Errorf("its quality's playlist =\n%s\n%v; want a chunk every 2 s", stream, err)
	}
	settingsService, isService := service.settings.(*settings.Service)
	if !isService {
		t.Fatal("the fixture's settings are not the settings service")
	}
	// No quality is below the film's four megabits: the highest no taller than its 1080p is offered.
	settingstest.Set(t, settingsService, settings.ChunkQualities, `"8000k@2160p,6000k@1080p"`)
	if text, _, err = service.MasterPlaylist(t.Context(), playback); err != nil ||
		!strings.Contains(text, "q6000k1080p/") || strings.Contains(text, "q8000k2160p/") {
		t.Errorf("with no quality below its picture, film 2 is offered\n%s, %v; want 6000k at 1080p", text, err)
	}
}

// A playback that began on its film cut on time stays so though the film's keyframes are known; one
// that did not is offered the film on its keyframes, its picture copied (D269 §3).
func TestMasterPlaylist_APlaybackThatBeganOnTimeStaysOnTimeOnceTheKeyframesAreKnown(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	onTime := Playback{FileID: 1, IsSourcePictureDecodable: true, IsOnTime: true}
	text, kind, err := service.MasterPlaylist(t.Context(), onTime)
	if err != nil || kind != OnTime || strings.Contains(text, "picture/") {
		t.Errorf("film 1 to a playback that began on time =\n%s\n%s, %v; want it on time, no picture copied",
			text, kind, err)
	}
	stream, err := service.MediaPlaylist(t.Context(), onTime, "q1200k480p")
	if err != nil || strings.Count(stream, "#EXTINF:") != filmMinutes*60/2 {
		t.Errorf("its quality's playlist =\n%s\n%v; want a chunk every 2 s", stream, err)
	}
	onKeyframes := Playback{FileID: 1, IsSourcePictureDecodable: true}
	if text, kind, err = service.MasterPlaylist(t.Context(), onKeyframes); err != nil || kind != OnKeyframes ||
		!strings.Contains(text, "picture/") {
		t.Errorf("film 1 to a new playback =\n%s\n%s, %v; want it on its keyframes, its picture copied",
			text, kind, err)
	}
}

// A run asked for on a film cut on time is made on that grid, though the film's keyframes are known
// (D269 §3).
func TestHandleMakeChunks_ARunOnTimeIsMadeOnTheTimeGrid(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	starts, err := TimeGridOf(filmMinutes*60_000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	gridID := Grid{StartsMs: starts, FilmDurationMs: filmMinutes * 60_000}.ID()
	fixture.start(t, 7, testRun{"q1200k480p", gridID, runArgs{FirstChunk: 3, LastChunk: 5}})
	sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
	if sent == nil || !slices.Equal(sent.BoundsMs, []int64{6000, 8000, 10_000, 12_000}) ||
		sent.Stream.Height != 480 || sent.PictureSeekMs != 0 {
		t.Errorf("the node was sent %+v; want chunks 3 to 5 at 6, 8, 10 and 12 s, encoded", sent)
	}
}

func TestOnlyQuality_IsTheHighestNoTallerThanThePictureElseTheLowest(t *testing.T) {
	t.Parallel()
	all := []qualities.Quality{{Kbps: 3000, Height: 720}, {Kbps: 1200, Height: 480}, {Kbps: 700, Height: 432}}
	for _, each := range []struct {
		height int
		want   qualities.Quality
	}{
		{1080, all[0]}, {480, all[1]}, {360, all[2]}, {0, all[0]},
	} {
		if got := onlyQuality(all, each.height); got != each.want {
			t.Errorf("the one quality for a picture %d high = %+v; want %+v", each.height, got, each.want)
		}
	}
}

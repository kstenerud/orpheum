// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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

// A film whose file has no index of its keyframes is cut on time at once, its playback offered its
// twin and the qualities below it, not the picture copied, though its player decodes it, and read
// through ahead of the sweep for its keyframes (D269 §1, §2, §4, D344 §3).
func TestMasterPlaylist_AFilmWhoseFileHasNoIndexIsCutOnTimeWithoutItsPictureCopied(t *testing.T) {
	t.Parallel()
	service, reader := chunksFixture(t, newFakeQueue(), fakeNode{})
	reader.noIndex = [][]byte{{0x02}}
	playback := Playback{FileID: 2, IsSourcePictureDecodable: true}
	text, kept, err := service.MasterPlaylist(t.Context(), playback)
	if err != nil || !kept.IsOnTime || strings.Contains(text, "picture/") || !strings.Contains(text, "q1200k480p/") ||
		!strings.Contains(text, "q4000k1080p/") {
		t.Fatalf("film 2's master playlist =\n%s\n%+v, %v; want it cut on time, its twin and qualities and no "+
			"picture copied", text, kept, err)
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
	if offer, startErr := service.StartOnLadder(t.Context(), playback, 0, 0); startErr != nil || !offer.IsOnTime {
		t.Errorf("film 2's start on its ladder = %+v, %v; want it said to be on time, which its playback keeps",
			offer, startErr)
	}
}

// A picture too small for a twin, which its player cannot decode, is offered the one quality a film
// cut on time is offered with none below its own picture, the highest no taller than it, else the
// lowest, so its playback has a picture (D269 §1, D344 §4).
func TestStartOnLadder_APictureTooSmallForATwinIsOfferedOneQuality(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	storetest.ExecRaw(t, service.database.Path(), `UPDATE file_probe SET probe_json = replace(replace(`+
		`probe_json, '"height":1080', '"height":120'), '"width":1920', '"width":160') WHERE file_id = 1`)
	offer, err := service.StartOnLadder(t.Context(), Playback{FileID: 1, HasStartingAudio: true}, 0, 0)
	if err != nil || offer.Quality != (qualities.Quality{Kbps: 400, Height: 360}) {
		t.Errorf("a picture 120 lines high its player cannot decode starts at %+v, %v; want the lowest quality",
			offer, err)
	}
}

// A run asked for on the time grid of a film whose keyframes cannot cut it, FFmpeg having failed to
// read it through, is made on that grid (D344 §2).
func TestHandleMakeChunks_ARunOfAFilmItsKeyframesCannotCutIsMadeOnTime(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	failure := "Invalid data found when processing input"
	err := fixture.service.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertVideoTimeline(t.Context(), sqlcgen.UpsertVideoTimelineParams{
			PayloadHash: []byte{0x01}, KeyframesMsJSON: "[]", BytesInEachSecondJSON: "[]", Failure: &failure,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	starts, err := TimeGridOf(filmMinutes*60_000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	gridID := Grid{StartsMs: starts, FilmDurationMs: filmMinutes * 60_000}.ID()
	fixture.start(t, 7, testRun{"q1200k480p", gridID, runArgs{FirstChunk: 3, LastChunk: 4}})
	sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
	if sent == nil || !slices.Equal(sent.BoundsMs, []int64{6000, 8000, 10_000}) {
		t.Errorf("the node was sent %+v; want chunks 3 and 4 at 6, 8 and 10 s", sent)
	}
}

// A playback that began on its film cut on time stays so though the film's keyframes are known; one
// that did not is offered the film on its keyframes, its picture copied (D269 §3).
func TestMasterPlaylist_APlaybackThatBeganOnTimeStaysOnTimeOnceTheKeyframesAreKnown(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	onTime := Playback{FileID: 1, IsSourcePictureDecodable: true, IsOnTime: true}
	text, kept, err := service.MasterPlaylist(t.Context(), onTime)
	if err != nil || !kept.IsOnTime || strings.Contains(text, "picture/") {
		t.Errorf("film 1 to a playback that began on time =\n%s\n%+v, %v; want it on time, no picture copied",
			text, kept, err)
	}
	stream, err := service.MediaPlaylist(t.Context(), onTime, "q1200k480p")
	if err != nil || strings.Count(stream, "#EXTINF:") != filmMinutes*60/2 {
		t.Errorf("its quality's playlist =\n%s\n%v; want a chunk every 2 s", stream, err)
	}
	onKeyframes := Playback{FileID: 1, IsSourcePictureDecodable: true}
	if text, kept, err = service.MasterPlaylist(t.Context(), onKeyframes); err != nil || kept.IsOnTime ||
		!strings.Contains(text, "picture/") {
		t.Errorf("film 1 to a new playback =\n%s\n%+v, %v; want it on its keyframes, its picture copied",
			text, kept, err)
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

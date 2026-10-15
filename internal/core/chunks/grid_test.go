// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestGridOf_StartsAChunkAtTheFirstKeyframeAtOrAfterEachMultipleOfTheLength(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		what                  string
		keyframesMs           []int64
		durationMs, segmentMs int64
		want                  []int64
	}{
		{
			"dense keyframes, every half second, are merged into chunks of two seconds",
			[]int64{0, 500, 1000, 1500, 2000, 2500, 3000, 3500, 4000, 4500, 5000, 5500},
			6000, 2000,
			[]int64{0, 2000, 4000},
		},
		{
			"keyframes ten seconds apart are each a chunk's start",
			[]int64{0, 10_010, 20_020, 30_030},
			40_000, 2000,
			[]int64{0, 10_010, 20_020, 30_030},
		},
		{
			"uneven keyframes, some a second apart, start a chunk at the first past each two seconds",
			[]int64{0, 1000, 10_000, 11_000, 12_500, 20_000},
			30_000, 2000,
			[]int64{0, 10_000, 12_500, 20_000},
		},
		{
			"a last chunk shorter than the length is merged into the one before it",
			[]int64{0, 2000, 4000, 6000},
			7000, 2000,
			[]int64{0, 2000, 4000},
		},
		{
			"a last chunk exactly the length is kept",
			[]int64{0, 2000, 4000},
			6000, 2000,
			[]int64{0, 2000, 4000},
		},
		{
			"a keyframe just short of a multiple starts no chunk: the one after the multiple does",
			[]int64{0, 2500, 4400, 4600},
			9000, 2000,
			[]int64{0, 2500, 4400},
		},
		{"a single keyframe is one chunk to the end", []int64{0}, 90_000, 2000, []int64{0}},
		{"a film shorter than the length is one chunk", []int64{0, 500}, 1500, 2000, []int64{0}},
		{
			"a picture whose first keyframe is late starts there",
			[]int64{80, 2080, 4080},
			6000, 2000,
			[]int64{80, 2080},
		},
		{
			"a keyframe at or past the film's end starts nothing",
			[]int64{0, 4000, 8000, 9000},
			8000, 2000,
			[]int64{0, 4000},
		},
	} {
		got, err := GridOf(check.keyframesMs, check.durationMs, check.segmentMs)
		if err != nil || !slices.Equal(got, check.want) {
			t.Errorf("%s: %v, %v; want %v", check.what, got, err, check.want)
		}
	}
}

func TestGridOf_APictureWithNoKeyframeBeforeItsEndCannotBeCut(t *testing.T) {
	t.Parallel()
	for what, check := range map[string]struct {
		keyframesMs           []int64
		durationMs, segmentMs int64
	}{
		"no keyframes":                    {nil, 6000, 2000},
		"an empty list of keyframes":      {[]int64{}, 6000, 2000},
		"its first keyframe past its end": {[]int64{7000}, 6000, 2000},
		"a film of no length":             {[]int64{0}, 0, 2000},
		"a chunk of no length":            {[]int64{0, 1000}, 6000, 0},
	} {
		if starts, err := GridOf(check.keyframesMs, check.durationMs, check.segmentMs); !errors.Is(err,
			ErrCannotBeCut) {
			t.Errorf("%s: %v, %v; want ErrCannotBeCut", what, starts, err)
		}
	}
}

// A stretch of a film cut alone, from any keyframe that starts a chunk, has the same starts as the
// whole film's grid from there: what lets a film be cut as its keyframes are found (D259 §1).
func TestGridOf_AStretchCutAloneAgreesWithTheWholeFilmsGrid(t *testing.T) {
	t.Parallel()
	keyframesMs := []int64{0, 900, 2100, 2500, 3999, 4001, 5300, 7900, 8100, 9700, 12_000, 12_400, 13_100, 16_050}
	whole, err := GridOf(keyframesMs, 20_000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	for index, start := range whole {
		from := slices.Index(keyframesMs, start)
		stretch, err := GridOf(keyframesMs[from:], 20_000, 2000)
		if err != nil || !slices.Equal(stretch, whole[index:]) {
			t.Errorf("cut alone from %d ms: %v, %v; want the whole grid's %v", start, stretch, err, whole[index:])
		}
	}
}

// serviceWithFilms is a chunks service over five films. Film 1's picture is stream 1, after a
// cover, and both have timelines; film 2 is probed without a length; film 3 has no timeline yet;
// film 4 is one FFmpeg could not read; film 5 has only sound. The segment length is two seconds.
func serviceWithFilms(t *testing.T) *Service {
	t.Helper()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/m"})
	probes := map[int64]string{
		1: `{"format":{"durationMs":7000},"streams":[{"kind":"video","codec":"mjpeg","disposition":["attached_pic"]},` +
			`{"streamIndex":1,"kind":"video","codec":"h264"}]}`,
		2: `{"format":{},"streams":[{"kind":"video","codec":"h264"}]}`,
		3: `{"format":{"durationMs":7000},"streams":[{"kind":"video","codec":"h264"}]}`,
		4: `{"format":{"durationMs":7000},"streams":[{"kind":"video","codec":"h264"}]}`,
		5: `{"format":{"durationMs":7000},"streams":[{"kind":"audio","codec":"aac"}]}`,
	}
	for fileID, probe := range probes {
		storetest.ExecRaw(t, database.Path(), fmt.Sprintf("INSERT INTO file (id, library_id, node_id, path, "+
			"payload_hash, filing, first_seen_at, last_seen_at) "+
			"VALUES (%d, %d, %d, '/m/f%d.mkv', X'0%d', 'unidentified', 1, 1)",
			fileID, libraryID, nodeID, fileID, fileID),
			fmt.Sprintf("INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (%d, '%s', 7000)",
				fileID, probe))
	}
	failure := "Invalid data found"
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		for _, row := range []sqlcgen.UpsertVideoTimelineParams{
			{PayloadHash: []byte{0x01}, VideoStreamIndex: 0, KeyframesMsJSON: "[0]", BytesInEachSecondJSON: "[]"},
			{
				PayloadHash: []byte{0x01}, VideoStreamIndex: 1, KeyframesMsJSON: "[0,1000,2000,4000,6000]",
				BytesInEachSecondJSON: "[1,1,1,1,1,1,1]",
			},
			{PayloadHash: []byte{0x02}, KeyframesMsJSON: "[0,3000]", BytesInEachSecondJSON: "[1,1,1,1,1]"},
			{PayloadHash: []byte{0x04}, KeyframesMsJSON: "[]", BytesInEachSecondJSON: "[]", Failure: &failure},
		} {
			if err := queries.UpsertVideoTimeline(t.Context(), row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	seeded := settingstest.Seeded(t, database)
	settingstest.Set(t, seeded, settings.HLSSegmentLengthSeconds, "2")
	return NewService(Parts{
		Database: database, Settings: seeded, Clock: clocktest.NewFake(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)),
	})
}

func TestGrid_IsTheMainPicturesKeptTimelineMergedToTheSegmentLength(t *testing.T) {
	t.Parallel()
	service := serviceWithFilms(t)
	if grid, isKept, err := service.Grid(t.Context(), 1); err != nil || !isKept ||
		!slices.Equal(grid.StartsMs, []int64{0, 2000, 4000}) || grid.FilmDurationMs != 7000 {
		t.Errorf("film 1's grid = %+v, %t, %v; want its picture's keyframes, not its cover's, merged to two "+
			"seconds, its last short chunk merged, to its seven seconds", grid, isKept, err)
	}
	if grid, isKept, err := service.Grid(t.Context(), 2); err != nil || !isKept ||
		!slices.Equal(grid.StartsMs, []int64{0, 3000}) || grid.FilmDurationMs != 5000 {
		t.Errorf("film 2's grid = %+v, %t, %v; want its film's length from its timeline's five seconds",
			grid, isKept, err)
	}
}

func TestGrid_SaysThereIsNoneYetOrThatTheFilmCannotBeCut(t *testing.T) {
	t.Parallel()
	service := serviceWithFilms(t)
	if grid, isKept, err := service.Grid(t.Context(), 3); err != nil || isKept {
		t.Errorf("film 3's grid = %+v, %t, %v; want none yet, and no error", grid, isKept, err)
	}
	for _, fileID := range []int64{4, 5} {
		if grid, isKept, err := service.Grid(t.Context(), fileID); !errors.Is(err, ErrCannotBeCut) || isKept {
			t.Errorf("film %d's grid = %+v, %t, %v; want ErrCannotBeCut", fileID, grid, isKept, err)
		}
	}
}

// The keyframe at or before a position is the start of the chunk holding it, a chunk's own start
// itself; a film with no grid says it does not know (D285 §1).
func TestKeyframeAtOrBefore_IsTheStartOfTheChunkHoldingThePosition(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	grid := mustGrid(t, service)
	if len(grid.StartsMs) < 3 {
		t.Fatalf("film 1's grid has %d chunks; want at least 3", len(grid.StartsMs))
	}
	for _, at := range []int64{grid.StartsMs[2], grid.StartsMs[2] + 1, grid.StartsMs[3] - 1} {
		keyframe, isKnown := service.KeyframeAtOrBefore(t.Context(), 1, at)
		if !isKnown || keyframe != grid.StartsMs[2] {
			t.Errorf("the keyframe at or before %d ms = %d, %t; want chunk 2's start, %d", at, keyframe, isKnown,
				grid.StartsMs[2])
		}
	}
	if _, isKnown := service.KeyframeAtOrBefore(t.Context(), 99, 1000); isKnown {
		t.Error("a film that does not exist has a keyframe")
	}
}

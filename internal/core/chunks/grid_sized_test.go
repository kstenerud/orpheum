// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// mebibytes is n MiB, a picture's bytes in a second.
func mebibytes(n float64) int64 { return int64(n * (1 << 20)) }

// everySecond is a picture's bytes in each of its seconds, from runs of seconds at a rate in MiB.
func everySecond(runs ...rateRun) []int64 {
	var seconds []int64
	for _, each := range runs {
		for range each.seconds {
			seconds = append(seconds, mebibytes(each.rate))
		}
	}
	return seconds
}

// keyframesEvery are keyframes stepMs apart from 0 to before endMs.
func keyframesEvery(stepMs, endMs int64) []int64 {
	var keyframes []int64
	for at := int64(0); at < endMs; at += stepMs {
		keyframes = append(keyframes, at)
	}
	return keyframes
}

// rateRun is a run of seconds at a rate in MiB a second.
type rateRun struct {
	seconds int
	rate    float64
}

// A heavy stretch of the film is cut into chunks of at most largestChunkBytes at its keyframes, a
// light one as GridOf cuts it (the owner, 2026-11-11: "I just want to make sure we're not locking
// away full quality unnecessarily").
func TestSizedGridOf_EndsAChunkAtTheKeyframeBeforeItWouldHoldMoreThanTheLargest(t *testing.T) {
	t.Parallel()
	if largestChunkBytes != 5<<20 {
		t.Fatalf("largestChunkBytes is %d; this test reckons with 5 MiB", largestChunkBytes)
	}
	keyframes := keyframesEvery(1000, 30_000)
	// 1 MiB a second, then 0.2, then 1 again: 10 MiB, 2 MiB and 10 MiB in each ten seconds.
	seconds := everySecond(rateRun{10, 1}, rateRun{10, 0.2}, rateRun{10, 1})
	got, err := SizedGridOf(keyframes, seconds, 30_000, 10_000)
	// From 0 the chunk would pass 5 MiB at 6 s, so it ends at 5 s; the light stretch is one chunk;
	// the last ten seconds are two, the last not merged back, as the two would pass 5 MiB.
	if want := []int64{0, 5000, 10_000, 20_000, 25_000}; err != nil || !slices.Equal(got, want) {
		t.Errorf("the sized grid = %v, %v; want %v", got, err, want)
	}
	if plain, err := GridOf(keyframes, 30_000, 10_000); err != nil || !slices.Equal(plain, []int64{0, 10_000, 20_000}) {
		t.Errorf("GridOf = %v, %v; want a chunk every ten seconds, whatever their bytes", plain, err)
	}
}

func TestSizedGridOf_NeverSplitsTheStretchBetweenTwoKeyframes(t *testing.T) {
	t.Parallel()
	// Keyframes ten seconds apart, each stretch 20 MiB: nothing can be cut between them.
	got, err := SizedGridOf([]int64{0, 10_000, 20_000}, everySecond(rateRun{30, 2}), 30_000, 10_000)
	if want := []int64{0, 10_000, 20_000}; err != nil || !slices.Equal(got, want) {
		t.Errorf("the sized grid = %v, %v; want each keyframe's own chunk, %v", got, err, want)
	}
}

func TestSizedGridOf_MergesAShortLastChunkOnlyWithinTheLargest(t *testing.T) {
	t.Parallel()
	keyframes := []int64{0, 10_000, 20_000}
	light, err := SizedGridOf(keyframes, everySecond(rateRun{25, 0.1}), 25_000, 10_000)
	if want := []int64{0, 10_000}; err != nil || !slices.Equal(light, want) {
		t.Errorf("a light film's grid = %v, %v; want its short last chunk merged, %v", light, err, want)
	}
	heavy, err := SizedGridOf(keyframes, everySecond(rateRun{25, 0.4}), 25_000, 10_000)
	if want := []int64{0, 10_000, 20_000}; err != nil || !slices.Equal(heavy, want) {
		t.Errorf("a heavier film's grid = %v, %v; want its short last chunk kept, as 6 MiB would pass the largest, %v",
			heavy, err, want)
	}
}

func TestSizedGridOf_WithNoBytesIsGridOf(t *testing.T) {
	t.Parallel()
	keyframes := keyframesEvery(1000, 30_000)
	got, err := SizedGridOf(keyframes, nil, 30_000, 10_000)
	if want := []int64{0, 10_000, 20_000}; err != nil || !slices.Equal(got, want) {
		t.Errorf("the grid of a film with no bytes read = %v, %v; want GridOf's, %v", got, err, want)
	}
}

// A stretch of a film cut alone, from any chunk's start, a sized one's too, has the same starts as
// the whole film's sized grid from there (D259 §1).
func TestSizedGridOf_AStretchCutAloneAgreesWithTheWholeFilmsGrid(t *testing.T) {
	t.Parallel()
	keyframes := []int64{
		0, 900, 2100, 2500, 3999, 4001, 5300, 7900, 8100, 9700, 12_000, 12_400, 13_100, 16_050,
		17_000, 18_500, 21_000, 22_200, 23_900, 25_100, 27_700, 29_000,
	}
	seconds := everySecond(rateRun{4, 0.5}, rateRun{6, 2.5}, rateRun{5, 0.3}, rateRun{10, 3}, rateRun{5, 1})
	whole, err := SizedGridOf(keyframes, seconds, 30_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := GridOf(keyframes, 30_000, 10_000); err != nil || len(whole) <= len(plain) {
		t.Fatalf("the sized grid %v cuts no more than GridOf's %v: the test proves nothing (%v)", whole, plain, err)
	}
	for index, start := range whole {
		from := slices.Index(keyframes, start)
		stretch, err := SizedGridOf(keyframes[from:], seconds, 30_000, 10_000)
		if err != nil || !slices.Equal(stretch, whole[index:]) {
			t.Errorf("cut alone from %d ms: %v, %v; want the whole grid's %v", start, stretch, err, whole[index:])
		}
	}
}

// A playback that began before its film's picture's bytes were read stays on its keyframes alone,
// as its chunks would otherwise move under it; one begun after is offered the film sized.
func TestMasterPlaylist_APlaybackBegunBeforeThePicturesBytesAreReadStaysOnItsKeyframesAlone(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	// Film 2: a keyframe every second, 4 MiB in each, cut every 2 s alone and every second sized.
	keyframes := keyframesEvery(1000, filmMinutes*60_000)
	setTimeline := func(seconds []int64) {
		t.Helper()
		keyframesJSON, err := json.Marshal(keyframes)
		if err != nil {
			t.Fatal(err)
		}
		secondsJSON, err := json.Marshal(seconds)
		if err != nil {
			t.Fatal(err)
		}
		err = service.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.UpsertVideoTimeline(t.Context(), sqlcgen.UpsertVideoTimelineParams{
				PayloadHash: []byte{0x02}, KeyframesMsJSON: string(keyframesJSON),
				BytesInEachSecondJSON: string(secondsJSON),
			})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	chunksOf := func(playback Playback) int {
		t.Helper()
		stream, err := service.MediaPlaylist(t.Context(), playback, "q1200k480p")
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(stream, "#EXTINF:")
	}
	setTimeline([]int64{})
	early := Playback{FileID: 2, IsSourcePictureDecodable: true}
	_, kept, err := service.MasterPlaylist(t.Context(), early)
	if err != nil || !kept.IsOnKeyframesAlone {
		t.Fatalf("a playback before the bytes are read keeps %+v, %v; want it on its keyframes alone", kept, err)
	}
	early.IsOnKeyframesAlone = true
	setTimeline(everySecond(rateRun{filmMinutes * 60, 4}))
	if got := chunksOf(early); got != filmMinutes*60/2 {
		t.Errorf("the early playback has %d chunks once the bytes are read; want its grid kept, one every 2 s", got)
	}
	late := Playback{FileID: 2, IsSourcePictureDecodable: true}
	if _, kept, err = service.MasterPlaylist(t.Context(), late); err != nil || kept.IsOnKeyframesAlone {
		t.Errorf("a playback after the bytes are read keeps %+v, %v; want it sized", kept, err)
	}
	if got := chunksOf(late); got != filmMinutes*60 {
		t.Errorf("the later playback has %d chunks; want one every second, as 8 MiB in 2 s passes the largest", got)
	}
}

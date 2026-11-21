// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/fingerprint"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeFingerprinter answers each fingerprint asked for with the next of its prints, in the order the
// node asks for them, and records each stretch asked for.
type fakeFingerprinter struct {
	mutex  sync.Mutex
	prints [][]uint32
	asked  []fingerprintAsk
}

type fingerprintAsk struct {
	streamIndex  int
	from, length time.Duration
}

func (fake *fakeFingerprinter) Fingerprint(
	_ context.Context, _ execx.Film, audioStreamIndex int, from, length time.Duration,
) ([]uint32, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.asked = append(fake.asked, fingerprintAsk{streamIndex: audioStreamIndex, from: from, length: length})
	next := fake.prints[0]
	fake.prints = fake.prints[1:]
	return next, nil
}

// pointsAt is a fingerprint of before random points, then shared, then after random points.
func pointsAt(random *rand.Rand, before int, shared []uint32, after int) []uint32 {
	found := make([]uint32, 0, before+len(shared)+after)
	for range before {
		found = append(found, random.Uint32())
	}
	found = append(found, shared...)
	for range after {
		found = append(found, random.Uint32())
	}
	return found
}

// D101, D115: the node fingerprints the opening and closing of each video of a season the core
// named, checked to be the one named, and answers the stretch the others share in each in its
// video's time, with how many it was heard in of how many compared; a video changed since is refused
// and compared with none, and one that shares nothing has no stretch.
func TestAgent_FindsTheStretchOfEachOfASeasonsVideosTheOthersShare(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewPCG(7, 8)) //nolint:gosec // test data, not a secret
	intro, credits := pointsAt(random, 300, nil, 0), pointsAt(random, 250, nil, 0)
	fake := &fakeFingerprinter{prints: [][]uint32{
		pointsAt(random, 40, intro, 100), pointsAt(random, 10, credits, 30),
		pointsAt(random, 90, intro, 50), pointsAt(random, 60, credits, 0),
		pointsAt(random, 500, nil, 0), pointsAt(random, 20, credits, 60), // an episode with no intro
	}}
	core, socket := startFakeCore(t)
	root, _ := startAgentWithWorkers(t, socket,
		Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}, SoundFingerprinter: fake})
	var videos []nodeproto.RepeatVideo
	for index, name := range []string{"first.mkv", "second.mkv", "film.mkv", "episode.mkv"} {
		if err := os.WriteFile(filepath.Join(root.Path(), name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		named := name
		if index == 2 {
			named = "what it was when scanned" // changed since
		}
		videos = append(videos, nodeproto.RepeatVideo{
			FileID: int64(100 + index),
			Video: nodeproto.BytesFile{
				RootPath: root.Path(), Path: []byte(name), PayloadHash: nodetest.PayloadHashHex(t, named),
			},
			AudioStreamIndex: index + 1, DurationMs: int64(1200000 + index*1000),
		})
	}
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type:        nodeproto.CommandFindRepeats,
		FindRepeats: &nodeproto.FindRepeats{Videos: videos, OpeningMs: 600000, ClosingMs: 360000},
	}}
	answer := *testutil.Receive(t, core.repeats, "the season's repeats").Repeats
	if len(answer.Videos) != len(videos) || answer.Refusal != "" || answer.Stall != nil {
		t.Fatalf("the answer = %+v; want one for each of the %d videos", answer, len(videos))
	}
	span := func(from time.Duration, start, length, heardIn int) *nodeproto.RepeatedSpan {
		begins, ends := fingerprint.Span{Start: start, End: start + length}.Times()
		return &nodeproto.RepeatedSpan{
			StartMs: (from + begins).Milliseconds(), EndMs: (from + ends).Milliseconds(),
			HeardIn: heardIn, ComparedWith: 2,
		}
	}
	closingOf := func(index int) time.Duration {
		return time.Duration(videos[index].DurationMs)*time.Millisecond - 6*time.Minute
	}
	for _, check := range []struct {
		what   string
		got    *nodeproto.RepeatedSpan
		wanted *nodeproto.RepeatedSpan
	}{
		{"the first's intro", answer.Videos[0].Opening, span(0, 40, len(intro), 1)},
		{"the first's credits", answer.Videos[0].Closing, span(closingOf(0), 10, len(credits), 2)},
		{"the second's intro", answer.Videos[1].Opening, span(0, 90, len(intro), 1)},
		{"the second's credits", answer.Videos[1].Closing, span(closingOf(1), 60, len(credits), 2)},
		{"the last's intro", answer.Videos[3].Opening, nil},
		{"the last's credits", answer.Videos[3].Closing, span(closingOf(3), 20, len(credits), 2)},
	} {
		checkSpan(t, check.what, check.got, check.wanted)
	}
	checkFilesSaidAgain(t, answer.Videos)
	if changed := answer.Videos[2]; changed.RefusalCode != nodeproto.RefusedChanged || changed.Opening != nil ||
		changed.Closing != nil {
		t.Errorf("the changed video = %+v; want it refused as changed, unread", changed)
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	want := []fingerprintAsk{
		{streamIndex: 1, length: 10 * time.Minute}, {streamIndex: 1, from: closingOf(0), length: 6 * time.Minute},
	}
	if len(fake.asked) != 6 || !slices.Equal(fake.asked[:2], want) || fake.asked[5].streamIndex != 4 {
		t.Errorf("asked for %+v; want each read video's opening and closing, from its audio stream", fake.asked)
	}
}

// checkSpan fails unless a repeated span is the one wanted, or both are none.
func checkSpan(t *testing.T, what string, got, wanted *nodeproto.RepeatedSpan) {
	t.Helper()
	if (got == nil) != (wanted == nil) || (got != nil && *got != *wanted) {
		t.Errorf("%s = %+v; want %+v", what, got, wanted)
	}
}

// checkFilesSaidAgain fails unless each answer says the file id of the command's video in its place.
func checkFilesSaidAgain(t *testing.T, answers []nodeproto.VideoRepeats) {
	t.Helper()
	for index, answer := range answers {
		if answer.FileID != int64(100+index) {
			t.Errorf("answer %d is for file %d; want the command's file %d", index, answer.FileID, 100+index)
		}
	}
}

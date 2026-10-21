// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// stallingRunner records how each run was watched, and answers as a hung FFmpeg the watchdog
// stopped.
type stallingRunner struct{ watches *[]execx.Watch }

func (runner stallingRunner) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	*runner.watches = append(*runner.watches, run.Watch)
	return execx.Ran{}, &execx.StalledError{Tool: execx.FFmpeg, Cause: execx.StallStoodStill}
}

func TestRead_IsWatchedForAPositionThatStandsStillAndSaysWhenItStalled(t *testing.T) {
	t.Parallel()
	var watches []execx.Watch
	// A large film, without taking the disk: its deadline is longer than the shortest any run gets.
	video := filepath.Join(t.TempDir(), "film.mkv")
	film, err := os.Create(video)
	if err != nil {
		t.Fatal(err)
	}
	_, err = film.WriteAt([]byte{0}, 60<<30-1)
	if closeErr := film.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	const slowestRead = 20 << 20
	_, err = NewReader(stallingRunner{&watches}, slowestRead).Read(t.Context(), video,
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}}, &nodeproto.VideoTimelineAsk{}, 0, nil)
	stalled, isStalled := errors.AsType[*execx.StalledError](err)
	if !isStalled || stalled.Cause != execx.StallStoodStill {
		t.Errorf("a stalled read: %v, want the stall", err)
	}
	if len(watches) != 1 || watches[0].StartGrace != 2*time.Minute || watches[0].StallWindow != 3*time.Minute ||
		watches[0].Deadline != execx.ReadingDeadline(video, slowestRead) {
		t.Errorf("a read was watched %+v; want two minutes to start, three standing still, and the "+
			"deadline of reading the file at the node's slowest read", watches)
	}
}

// A long film is allowed as long as decoding its sound at real time takes, when that is longer than
// reading it: measuring decodes every track, and a film read quickly is not measured quickly (the
// owner's first Watch Together test, 2026-10-05). Reading only its timeline decodes nothing.
func TestRead_ALongFilmIsAllowedAsLongAsDecodingItsSoundTakesOnlyWhenItsSoundIsMeasured(t *testing.T) {
	t.Parallel()
	var watches []execx.Watch
	video := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(video, []byte("a small file, read in no time"), 0o600); err != nil {
		t.Fatal(err)
	}
	const slowestRead = 20 << 20
	twoHours := 2 * time.Hour
	reader := NewReader(stallingRunner{&watches}, slowestRead)
	if _, err := reader.Read(t.Context(), video,
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 6}}, nil, twoHours, nil); err == nil {
		t.Fatal("the stalling runner's read succeeded")
	}
	if _, err := reader.Read(t.Context(), video, nil, &nodeproto.VideoTimelineAsk{}, twoHours, nil); err == nil {
		t.Fatal("the stalling runner's read succeeded")
	}
	if len(watches) != 2 || watches[0].Deadline != execx.DeadlineFor(twoHours) ||
		watches[0].Deadline <= execx.ReadingDeadline(video, slowestRead) {
		t.Errorf("a two-hour film's sound was watched %+v; want the deadline of decoding two hours, past "+
			"reading's", watches)
	}
	if len(watches) == 2 && watches[1].Deadline != execx.ReadingDeadline(video, slowestRead) {
		t.Errorf("a two-hour film's timeline alone was watched %+v; want reading's deadline", watches[1])
	}
}

// progressingRunner says one position, as FFmpeg's progress would, then stalls.
type progressingRunner struct{ at time.Duration }

func (runner progressingRunner) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	if run.Watch.OnProgress != nil {
		run.Watch.OnProgress(execx.Progress{Position: runner.at})
	}
	return execx.Ran{}, &execx.StalledError{Tool: execx.FFmpeg, Cause: execx.StallStoodStill}
}

// A read tells its caller how far into the film FFmpeg says it is (D324 §6).
func TestRead_SaysHowFarIntoTheFilmItIs(t *testing.T) {
	t.Parallel()
	var said []int64
	_, err := NewReader(progressingRunner{at: 12 * time.Second}, 20<<20).Read(t.Context(), "/media/film.mkv",
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}}, nil, time.Minute,
		func(readThroughMs int64) { said = append(said, readThroughMs) })
	if err == nil || len(said) != 1 || said[0] != 12_000 {
		t.Errorf("the read said %v (%v); want 12000 ms, as FFmpeg's progress said", said, err)
	}
}

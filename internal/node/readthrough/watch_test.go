// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/fdproxy"
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
	// A large film: its deadline is longer than the shortest any run gets.
	video := execx.Film{Path: "/media/film.mkv", Size: 60 << 30}
	const slowestRead = 20 << 20
	_, err := NewReader(stallingRunner{&watches}, slowestRead).Read(t.Context(), video,
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}}, &nodeproto.VideoTimelineAsk{}, 0, nil)
	stalled, isStalled := errors.AsType[*execx.StalledError](err)
	if !isStalled || stalled.Cause != execx.StallStoodStill {
		t.Errorf("a stalled read: %v, want the stall", err)
	}
	if len(watches) != 1 || watches[0].StartGrace != 2*time.Minute || watches[0].StallWindow != 3*time.Minute ||
		watches[0].Deadline != video.ReadingDeadline(slowestRead) {
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
	// A small file, read in no time.
	video := execx.Film{Path: "/media/film.mkv", Size: 29}
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
		watches[0].Deadline <= video.ReadingDeadline(slowestRead) {
		t.Errorf("a two-hour film's sound was watched %+v; want the deadline of decoding two hours, past "+
			"reading's", watches)
	}
	if len(watches) == 2 && watches[1].Deadline != video.ReadingDeadline(slowestRead) {
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
	reader := NewReader(progressingRunner{at: 12 * time.Second}, 20<<20)
	_, err := reader.Read(t.Context(), execx.Film{Path: "/media/film.mkv"},
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}}, nil, time.Minute,
		func(readThroughMs int64) { said = append(said, readThroughMs) })
	if err == nil || len(said) != 1 || said[0] != 12_000 {
		t.Errorf("the read said %v (%v); want 12000 ms, as FFmpeg's progress said", said, err)
	}
}

// recordingRunner keeps each run it is given, and answers as a run that read nothing.
type recordingRunner struct{ runs *[]execx.WatchedRun }

func (runner recordingRunner) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	*runner.runs = append(*runner.runs, run)
	return execx.Ran{}, nil
}

// A read-through reads its film as the node gives it: streamed through the node, granted no path to
// it, or by its path, granted that path alone (D330).
func TestRead_ReadsAStreamedFilmThroughTheNodeAndAnyOtherByItsPathAlone(t *testing.T) {
	t.Parallel()
	var runs []execx.WatchedRun
	reader := NewReader(recordingRunner{&runs}, 20<<20)
	timeline := &nodeproto.VideoTimelineAsk{}
	byPath := execx.Film{Path: "/media/film.mkv", Size: 10}
	streamed := execx.Film{Path: "/media/film.mkv", Size: 10, Stream: &fdproxy.Config{Run: 1}, CacheBlocks: 9}
	for _, film := range []execx.Film{byPath, streamed} {
		_, _ = reader.Read(t.Context(), film, nil, timeline, 0, nil) //nolint:errcheck // the run is what is checked
	}
	if len(runs) != 2 {
		t.Fatalf("%d runs; want one for each film", len(runs))
	}
	if run := runs[0]; run.Film != nil || !slices.Equal(run.Access.Read, []string{"/media/film.mkv"}) ||
		!slices.Contains(run.Arguments, "file:/media/film.mkv") {
		t.Errorf("a film by path was read as %+v; want its path granted and named, and no stream", run)
	}
	if run := runs[1]; run.Film != streamed.Stream || len(run.Access.Read) != 0 ||
		!slices.Contains(run.Arguments, "fdproxy:") || slices.Contains(run.Arguments, "file:/media/film.mkv") {
		t.Errorf("a streamed film was read as %+v; want it served through the node, no path granted or named", run)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitle

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// filmInput is how every test here names the film FFmpeg reads.
const filmInput = "file:/media/film.mkv"

// mapsOf are the streams a run's arguments map, in order.
func mapsOf(arguments []string) []string {
	var maps []string
	for index, argument := range arguments[:len(arguments)-1] {
		if argument == "-map" {
			maps = append(maps, arguments[index+1])
		}
	}
	return maps
}

// A film with N text tracks is read once, not N times (the audit of 2026-10-25, N3): one FFmpeg
// writes every track into one Matroska, and each is taken out of it from memory (D340).
func TestExtract_AVideosTracksComeFromOneReadOfIt(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "the text"})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	tracks, err := NewExtractor(runner, 20<<20).ExtractTracks(t.Context(), execx.Film{Path: "/media/film.mkv"},
		[]int{2, 3, 5})
	if err != nil || len(tracks) != 3 {
		t.Fatalf("ExtractTracks = %+v, %v; want three tracks", tracks, err)
	}
	for place, want := range []int{2, 3, 5} {
		track := tracks[place]
		if track.StreamIndex != want || track.Err != nil || string(track.Text.ASS) != "the text" {
			t.Errorf("track %d = %+v; want stream %d's text", place, track, want)
		}
	}
	filmReads, takes := classifyRuns(t, fake.Calls(t))
	if len(filmReads) != 1 || !slices.Equal(mapsOf(filmReads[0]), []string{"0:2", "0:3", "0:5"}) {
		t.Fatalf("the film was read by %q; want one run mapping streams 2, 3 and 5", filmReads)
	}
	if !slices.EqualFunc(takes, [][]string{{"0:0"}, {"0:1"}, {"0:2"}}, slices.Equal) {
		t.Errorf("the tracks were taken out by %q; want each of the Matroska's three in turn", takes)
	}
}

// classifyRuns are the arguments of the runs that read the film, and the maps of those that took a
// track out of the one read's Matroska; it fails the test for a run that read anything else.
func classifyRuns(t *testing.T, calls []testutil.FakeCall) (filmReads, takes [][]string) {
	t.Helper()
	for _, call := range calls {
		switch {
		case readsOnly(call.Arguments, "file", filmInput):
			filmReads = append(filmReads, call.Arguments)
		case readsOnly(call.Arguments, "pipe", "pipe:0") && slices.Contains(call.Arguments, formatMatroska):
			if string(call.Stdin) != "the text" {
				t.Errorf("a take was fed %q, want the one read's Matroska", call.Stdin)
			}
			takes = append(takes, mapsOf(call.Arguments))
		case !readsOnly(call.Arguments, "pipe", "pipe:0"):
			t.Errorf("a run read %q, neither the film by file: nor a pipe", call.Arguments)
		}
	}
	return filmReads, takes
}

// scriptedRunner answers each run as answer says, and counts the runs that read the film.
type scriptedRunner struct {
	answer    func(run execx.WatchedRun) (execx.Ran, error)
	filmReads *[][]string
}

func (runner scriptedRunner) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	if slices.Contains(run.Arguments, filmInput) {
		*runner.filmReads = append(*runner.filmReads, mapsOf(run.Arguments))
	}
	return runner.answer(run)
}

// errCannotConvert is FFmpeg failing on a track's bytes.
var errCannotConvert = &execx.RunError{Tool: execx.FFmpeg, ExitStatus: 1, Err: errors.New("exit status 1")}

// extractScripted extracts streams 2, 3 and 5 of the film with a runner that answers as answer
// says, and returns the tracks, the maps of each read of the film and the error.
func extractScripted(
	t *testing.T, answer func(run execx.WatchedRun) (execx.Ran, error),
) ([]TrackText, [][]string, error) {
	t.Helper()
	var filmReads [][]string
	extractor := NewExtractor(scriptedRunner{answer: answer, filmReads: &filmReads}, 20<<20)
	tracks, err := extractor.ExtractTracks(t.Context(), execx.Film{Path: "/media/film.mkv"}, []int{2, 3, 5})
	return tracks, filmReads, err
}

// failedStreams are the stream indexes of the tracks that failed.
func failedStreams(tracks []TrackText) []int {
	var failed []int
	for _, track := range tracks {
		if track.Err != nil {
			failed = append(failed, track.StreamIndex)
		}
	}
	return failed
}

func TestExtract_ATrackThatCannotBeMadeFailsAloneAndAStallFailsThemAll(t *testing.T) {
	t.Parallel()
	// The one read is made, and taking the second track out of it fails.
	tracks, reads, err := extractScripted(t, func(run execx.WatchedRun) (execx.Ran, error) {
		if slices.Contains(run.Arguments, formatMatroska) && slices.Equal(mapsOf(run.Arguments), []string{"0:1"}) {
			return execx.Ran{}, errCannotConvert
		}
		return execx.Ran{Stdout: []byte("text")}, nil
	})
	if err != nil || len(reads) != 1 || !slices.Equal(failedStreams(tracks), []int{3}) {
		t.Errorf("a track not taken out: %+v, %v, reads %q; want stream 3 failed alone, the film read once",
			tracks, err, reads)
	}
	// FFmpeg cannot make stream 5, which fails the one read: each is read on its own.
	tracks, reads, err = extractScripted(t, func(run execx.WatchedRun) (execx.Ran, error) {
		if slices.Contains(mapsOf(run.Arguments), "0:5") {
			return execx.Ran{}, errCannotConvert
		}
		return execx.Ran{Stdout: []byte("text")}, nil
	})
	wantReads := [][]string{{"0:2", "0:3", "0:5"}, {"0:2"}, {"0:3"}, {"0:5"}}
	if err != nil || !slices.EqualFunc(reads, wantReads, slices.Equal) ||
		!slices.Equal(failedStreams(tracks), []int{5}) {
		t.Errorf("a track that fails the one read: %+v, %v, reads %q; want each read alone and stream 5 failed",
			tracks, err, reads)
	}
	// A stall says nothing of the bytes: the whole extraction ends, and nothing is read again.
	tracks, reads, err = extractScripted(t, func(execx.WatchedRun) (execx.Ran, error) {
		return execx.Ran{}, &execx.StalledError{Tool: execx.FFmpeg, Cause: execx.StallStoodStill}
	})
	if !errors.As(err, new(*execx.StalledError)) || tracks != nil || len(reads) != 1 ||
		!strings.Contains(err.Error(), "[2 3 5]") {
		t.Errorf("a stalled read: %+v, %v, reads %q; want the stall, the film read once", tracks, err, reads)
	}
}

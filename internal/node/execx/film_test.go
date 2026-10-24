// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/ffstreamer"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// protocolsListing is the start of ffmpeg -protocols from the patched build, then its output
// protocols.
const protocolsListing = "Supported file protocols:\nInput:\n  async\n  bluray\n  fd\n  ffstreamer\n  file\n" +
	"Output:\n  file\n  pipe\n"

// A node streams its films only to an FFmpeg that lists ffstreamer among the protocols it reads: the
// stock build, which does not, reads each film by its path (D330), and so does one that names it only
// as an output.
func TestHasFFstreamer_IsTheProtocolAmongTheInputsFFmpegLists(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		name, listing string
		want          bool
	}{
		{"the patched build", protocolsListing, true},
		{"the stock build", "Supported file protocols:\nInput:\n  async\n  file\nOutput:\n  file\n", false},
		{"an output only", "Supported file protocols:\nInput:\n  file\nOutput:\n  ffstreamer\n", false},
	} {
		runner := newRunner(t, testutil.InstallFakeFFmpeg(t, testutil.FakeReply{FFmpegStdout: example.listing}))
		got, err := runner.HasFFstreamer(t.Context())
		if err != nil || got != example.want {
			t.Errorf("%s: %v, %v; want %v", example.name, got, err, example.want)
		}
	}
}

// A runner that streams films serves each from the file the node opened, through its disk's gate,
// numbering each run and bounding it by what its kind reads: a probe far less than a whole read. One
// that does not reads each by its path (D330).
func TestFilmOf_StreamsTheOpenedFileOnTheTermsOfWhatTheRunReads(t *testing.T) {
	t.Parallel()
	path, opened := openedTestFilm(t)
	runner, err := NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	film := runner.FilmOf(path, opened, ReadsWholeFilm)
	if film.Stream != nil || film.Path != path || film.Size != 1000 {
		t.Errorf("a runner that does not stream made %+v; want the film by its path", film)
	}
	runner.StreamFilms()
	whole, probe := runner.FilmOf(path, opened, ReadsWholeFilm), runner.FilmOf(path, opened, ReadsIndex)
	if whole.Stream == nil || probe.Stream == nil {
		t.Fatalf("a runner that streams made %+v and %+v; want both streamed", whole, probe)
	}
	if whole.Stream.Run < 1 || probe.Stream.Run == whole.Stream.Run {
		t.Errorf("runs numbered %d and %d; want each its own, from 1", whole.Stream.Run, probe.Stream.Run)
	}
	wantWhole, wantProbe := int64(1250+filmBudgetFloorBytes), int64(1000/16+filmBudgetFloorBytes)
	if whole.Stream.BudgetBytes != wantWhole || probe.Stream.BudgetBytes != wantProbe {
		t.Errorf("budgets %d for a whole read and %d for a probe; want %d, the film and a quarter, and %d, "+
			"a sixteenth, over the floor", whole.Stream.BudgetBytes, probe.Stream.BudgetBytes, wantWhole, wantProbe)
	}
	if whole.CacheBlocks != filmCacheBlocks || whole.Stream.MostOutstanding != 2 ||
		whole.Stream.DataPipeBytes != 1<<20 {
		t.Errorf("terms %+v, %d blocks kept; want two READs, a 1 MiB pipe and %d blocks", *whole.Stream,
			whole.CacheBlocks, filmCacheBlocks)
	}
	if err := whole.Stream.Source.ReadAt(t.Context(), make([]byte, 10), 990); err != nil {
		t.Errorf("the stream could not read the opened file: %v", err)
	}
}

// openedTestFilm is a film of 1,000 bytes opened as the node opens one, and its path.
func openedTestFilm(t *testing.T) (string, OpenedFilm) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(path, make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() }) //nolint:errcheck // the test's own file
	facts, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return path, OpenedFilm{
		File: file, Facts: facts, Disks: diskwatch.NewGate(clock.System{}), Root: "/", Window: time.Minute,
	}
}

// A streamed run passed only having exited 0, said DONE for its input, that DONE OK and for its own
// run, and given the node no reason to fail it (ffstreamer's specification, Judging a Run): a run that
// read part of its film never passes as complete.
func TestJudgeFilm_PassesOnlyARunThatExited0WithDoneOKForItsOwnRun(t *testing.T) {
	t.Parallel()
	exitedNonZero := &RunError{Tool: FFmpeg, ExitStatus: 1, Err: errors.New("exit status 1")}
	doneOK := &ffstreamer.Done{Run: 7, IsOK: true}
	for _, example := range []struct {
		name    string
		account ffstreamer.Account
		ended   error
		// says is something the verdict's error must say; empty for a pass.
		says string
		want error
	}{
		{name: "DONE OK for its run", account: ffstreamer.Account{Done: doneOK}},
		{name: "no DONE", account: ffstreamer.Account{}, says: "no DONE"},
		{
			name: "DONE ERR, though it exited 0",
			account: ffstreamer.Account{Done: &ffstreamer.Done{
				Run: 7, Kind: ffstreamer.KindChannel, Code: 5, Message: "Input/output error",
			}},
			says: "Input/output error",
		},
		{
			name: "DONE OK for another run", account: ffstreamer.Account{Done: &ffstreamer.Done{Run: 8, IsOK: true}},
			says: "names run 8", want: ffstreamer.ErrViolation,
		},
		{
			name: "the node's own reason, its DONE OK",
			account: ffstreamer.Account{
				Done: doneOK, Failure: ffstreamer.ErrOverBudget,
			},
			want: ffstreamer.ErrOverBudget,
		},
		{
			name: "a nonzero exit, its DONE OK", account: ffstreamer.Account{Done: doneOK},
			ended: exitedNonZero, want: exitedNonZero,
		},
	} {
		err := judgeFilm(FFmpeg, example.account, 7, &tailBuffer{}, example.ended)
		isPass := example.says == "" && example.want == nil
		switch {
		case isPass && err != nil:
			t.Errorf("%s: %v; want a pass", example.name, err)
		case !isPass && err == nil:
			t.Errorf("%s: passed; want a failure", example.name)
		case example.want != nil && !errors.Is(err, example.want):
			t.Errorf("%s: %v; want %v", example.name, err, example.want)
		case example.says != "" && !strings.Contains(err.Error(), example.says):
			t.Errorf("%s: %v; want it to say %q", example.name, err, example.says)
		}
	}
}

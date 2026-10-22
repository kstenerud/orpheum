// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/ffstreamer"
	"github.com/kstenerud/orpheum/internal/node/ffstreamer/ffstreamertest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// filmMode is the argument that tells the helper, standing in for the patched FFmpeg, what to do.
const filmMode = "-test_film_mode"

// TestMain runs the package's tests, or, when this test binary is run by the name ffmpeg (filmRunner
// links it so), is the patched FFmpeg of these tests: it reads its input through the node over
// ffstreamer as filmMode says, and prints the SHA-256 of what it read.
func TestMain(m *testing.M) {
	words := os.Args //nolint:forbidigo // the stand-in FFmpeg's own command line
	if filepath.Base(words[0]) == string(FFmpeg) {
		os.Exit(actAsFilmReader(words[1:], valueAfter(words, filmMode))) //nolint:forbidigo // its exit is its answer
	}
	os.Exit(m.Run()) //nolint:forbidigo // the tests' own exit
}

// actAsFilmReader reads the input as mode says and returns the exit status: 0, or which step failed.
func actAsFilmReader(words []string, mode string) int {
	readFD, _ := strconv.Atoi(valueAfter(words, "-ffstreamer_read_fd"))   //nolint:errcheck // the node's
	writeFD, _ := strconv.Atoi(valueAfter(words, "-ffstreamer_write_fd")) //nolint:errcheck // as above
	client := ffstreamertest.NewClient(readFD, writeFD)
	if err := client.Open(64); err != nil {
		return 2
	}
	switch mode {
	case "read-all", "no-done":
		film, err := client.ReadAll()
		if err != nil {
			return 3
		}
		if mode == "read-all" && client.Done() != nil {
			return 4
		}
		sum := sha256.Sum256(film)
		_, _ = fmt.Println(hex.EncodeToString(sum[:])) //nolint:forbidigo // the helper's answer
	case "breach":
		_ = client.Read(-1, 10) //nolint:errcheck // the node kills it for this
		time.Sleep(time.Minute) // waits for the kill
	}
	return 0
}

func valueAfter(words []string, name string) string {
	if index := slices.Index(words, name); index >= 0 && index+1 < len(words) {
		return words[index+1]
	}
	return ""
}

// memoryFilm is a film in memory that counts its reads and can fail them.
type memoryFilm struct {
	contents []byte
	reads    atomic.Int32
	isBroken bool
}

func (film *memoryFilm) Size() int64 { return int64(len(film.contents)) }

func (film *memoryFilm) ReadAt(_ context.Context, buffer []byte, offset int64) error {
	film.reads.Add(1)
	if film.isBroken {
		return fmt.Errorf("read the film: %w", syscall.EIO)
	}
	copy(buffer, film.contents[offset:])
	return nil
}

// filmRunner is a runner whose ffmpeg is this test binary, linked as ffmpeg so TestMain acts as the
// patched FFmpeg, watched on a clock that never ticks, so only the channel ends a run. A link, never a
// script written here: writing an executable while a parallel test forks lets the child inherit it
// open, and running it then fails with "text file busy" (testutil's fakeScript).
func filmRunner(t *testing.T) *Runner {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err = os.Symlink(self, filepath.Join(directory, string(FFmpeg))); err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(directory)
	if err != nil {
		t.Fatal(err)
	}
	runner.clock = clocktest.NewFake(time.Unix(0, 0))
	runner.watchTicks = func() (<-chan time.Time, func()) { return make(chan time.Time), func() {} }
	return runner
}

func runFilm(t *testing.T, ctx context.Context, film *memoryFilm, mode string) (Ran, error) { //nolint:revive // t first
	t.Helper()
	return filmRunner(t).RunWatched(ctx, WatchedRun{
		Arguments:        append([]string{filmMode, mode}, StreamedInput(4)...),
		StdoutLimitBytes: 1 << 10,
		Watch:            Watch{StartGrace: time.Hour, StallWindow: time.Hour, Deadline: time.Hour},
		Film:             &ffstreamer.Config{Run: 3, Source: film, MostOutstanding: 2},
	})
}

func newMemoryFilm() *memoryFilm {
	contents := make([]byte, 1000)
	for index := range contents {
		contents[index] = byte(index)
	}
	return &memoryFilm{contents: contents}
}

// A run whose input is a streamed film reads it through the node, and passes only having said
// DONE OK for its run (D330).
func TestRunWatched_AStreamedRunReadsItsFilmThroughTheNodeAndPassesOnItsDone(t *testing.T) {
	t.Parallel()
	film := newMemoryFilm()
	ran, err := runFilm(t, t.Context(), film, "read-all")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(film.contents)
	if got := string(ran.Stdout); got != hex.EncodeToString(sum[:])+"\n" {
		t.Errorf("the run read %q; want the film's SHA-256", got)
	}
	if film.reads.Load() == 0 {
		t.Error("the film was never read through the node")
	}
	if _, err := runFilm(t, t.Context(), newMemoryFilm(), "no-done"); err == nil {
		t.Error("a run that exited 0 without DONE passed; it may have read part of its film")
	}
}

// A run that breaks the protocol is killed, and fails as the node's verdict, not FFmpeg's (§9.2).
func TestRunWatched_AStreamedRunThatBreaksTheProtocolIsKilledAndFails(t *testing.T) {
	t.Parallel()
	started := time.Now() //nolint:forbidigo // bounds how long the kill took, in real time
	_, err := runFilm(t, t.Context(), newMemoryFilm(), "breach")
	runError, isRunError := errors.AsType[*RunError](err)
	if !isRunError || !errors.Is(err, ffstreamer.ErrViolation) {
		t.Fatalf("the run ended with %v; want a RunError for the violation", err)
	}
	if runError.ExitStatus != -1 || time.Since(started) > 30*time.Second { //nolint:forbidigo // as above
		took := time.Since(started) //nolint:forbidigo // as above
		t.Errorf("the run's status %d after %v; want killed at once", runError.ExitStatus, took)
	}
}

// A film whose storage fails a read is its disk's failure: the run stalled on its disk, as one
// reading a file whose share is gone does (D233 §10).
func TestRunWatched_AStreamedRunWhoseFilmCannotBeReadStalledOnItsDisk(t *testing.T) {
	t.Parallel()
	film := newMemoryFilm()
	film.isBroken = true
	_, err := runFilm(t, t.Context(), film, "read-all")
	stalled, isStalled := errors.AsType[*StalledError](err)
	if !isStalled || stalled.Cause != StallDiskFailed {
		t.Fatalf("the run ended with %v; want stalled on its disk", err)
	}
}

// A paused streamed run reads nothing until it is continued: its READs are held as its process is
// stopped (ffstreamer's PROTOCOL.md §2, D240 §6).
func TestRunWatched_APausedStreamedRunReadsNothingUntilContinued(t *testing.T) {
	t.Parallel()
	pausing := &Pausing{}
	pausing.Pause()
	film := newMemoryFilm()
	ended := make(chan error, 1)
	go func() {
		_, err := runFilm(t, WithPausing(t.Context(), pausing), film, "read-all")
		ended <- err
	}()
	time.Sleep(500 * time.Millisecond) // gives a run that ignored its pause time to read
	if got := film.reads.Load(); got != 0 {
		t.Fatalf("a paused run's film was read %d times; want none", got)
	}
	pausing.Continue()
	if err := testutil.Receive(t, ended, "the continued run to end"); err != nil {
		t.Fatalf("the continued run: %v", err)
	}
	if film.reads.Load() == 0 {
		t.Error("the continued run read nothing")
	}
}

// A run that is not watched, as ffprobe's is, also reads a streamed film through the node: its
// descriptors are where the input names them, though it has no progress pipe before them (D330).
func TestRunOnFilm_AnUnwatchedRunReadsItsStreamedFilmThroughTheNode(t *testing.T) {
	t.Parallel()
	contents := newMemoryFilm()
	film := Film{
		Path: "/films/a.mkv", Size: contents.Size(),
		Stream: &ffstreamer.Config{Run: 5, Source: contents, MostOutstanding: 2}, CacheBlocks: 4,
	}
	stdout, err := filmRunner(t).RunOnFilm(t.Context(), FFmpeg, film,
		append([]string{filmMode, "read-all"}, film.Input()...), 1<<10)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(contents.contents)
	if got := string(stdout); got != hex.EncodeToString(sum[:])+"\n" {
		t.Errorf("the run read %q; want the film's SHA-256", got)
	}
}

// A streamed film's run is granted no path to it and names the protocol as its input; a film read by
// path is granted its path and named by it, with the file protocol alone (cut-a §7).
func TestFilm_AStreamedFilmGrantsNoPathAndAFilmByPathGrantsOnlyItsOwn(t *testing.T) {
	t.Parallel()
	byPath := Film{Path: "/films/a.mkv", Size: 10}
	if got := byPath.Readable(); !slices.Equal(got, []string{"/films/a.mkv"}) {
		t.Errorf("a film by path grants %q; want its path", got)
	}
	if got := byPath.Input(); !slices.Equal(got, FileInput("/films/a.mkv")) {
		t.Errorf("a film by path is named %q; want FileInput", got)
	}
	streamed := Film{Path: "/films/a.mkv", Size: 10, Stream: &ffstreamer.Config{Run: 1}, CacheBlocks: 12}
	if got := streamed.Readable(); len(got) != 0 {
		t.Errorf("a streamed film grants %q; want nothing", got)
	}
	if got := streamed.Input(); !slices.Equal(got, StreamedInput(12)) {
		t.Errorf("a streamed film is named %q; want StreamedInput keeping its 12 blocks", got)
	}
}

// The node says once, not at every run, that its films' pipes are at their default size, since every
// run after the first finds the same (ffstreamer's PROTOCOL.md §4.3).
func TestRunner_SaysOnceThatAStreamedFilmsPipeKeptItsDefaultSize(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	runner := filmRunner(t).LoggingTo(slog.New(slog.NewTextHandler(&logged, nil)))
	runner.tellPipeSizeRefused(t.Context(), nil)
	if logged.Len() != 0 {
		t.Fatalf("a pipe that took its size was logged: %s", logged.String())
	}
	for range 3 {
		runner.tellPipeSizeRefused(t.Context(), syscall.EPERM)
	}
	if got := strings.Count(logged.String(), "fs.pipe-user-pages-soft"); got != 1 {
		t.Errorf("logged %d times; want once:\n%s", got, logged.String())
	}
}

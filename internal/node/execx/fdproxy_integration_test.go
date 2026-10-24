// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration && linux

package execx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/fdproxy"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// These run the integration tier's FFmpeg, which must have the fdproxy protocol (D330), on films the
// node serves it: what the stand-in FFmpeg of film_linux_test.go shows of the node's side, shown
// against the real child.

// streamedReadWindow is how long a read of a streamed film may take before its disk is judged stuck.
const streamedReadWindow = time.Second

// streamingRunner is a runner on the integration tier's FFmpeg that streams films, failing the test
// when that FFmpeg cannot read one through the node.
func streamingRunner(t *testing.T) *Runner {
	t.Helper()
	runner, err := NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	hasIt, err := runner.HasFDProxy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIt {
		t.Fatalf("the FFmpeg in %s has no %s protocol: the tier tests the build Orpheum pins (D330)",
			testutil.FFmpegDirectory(t), fdproxy.ProtocolName)
	}
	runner.StreamFilms()
	return runner
}

// makeStreamedFilm writes a film of a few MiB, video and audio interleaved, so a whole read of it
// takes several of the node's blocks.
func makeStreamedFilm(t *testing.T, runner *Runner) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "film.mkv")
	if _, err := runner.Run(t.Context(), FFmpeg, confine.Access{}, []string{
		"-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25:duration=6",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=6:sample_rate=48000",
		"-c:v", "libx264", "-preset", "ultrafast", "-b:v", "3M", "-g", "25",
		"-c:a", "flac", "file:" + path,
	}, 1<<20); err != nil {
		t.Fatal(err)
	}
	return path
}

// streamedFilm is the film at path as the node serves it to a run that reads it as reading, through
// a gate of its own on the real clock, as a node's reads are.
func streamedFilm(t *testing.T, runner *Runner, path string, reading FilmReading) Film {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() }) //nolint:errcheck // read only
	facts, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	film := runner.FilmOf(path, OpenedFilm{
		File: file, Facts: facts, Disks: diskwatch.NewGate(clock.System{}), Root: filepath.Dir(path),
		Window: streamedReadWindow,
	}, reading)
	if film.Stream == nil {
		t.Fatal("a streaming runner gave a film by path")
	}
	return film
}

// failingFrom is a film's source whose reads fail as a dead disk's do from offset on.
type failingFrom struct {
	fdproxy.Source

	offset int64
}

func (source failingFrom) ReadAt(ctx context.Context, buffer []byte, offset int64) error {
	if offset+int64(len(buffer)) > source.offset {
		return fmt.Errorf("read the film: %w", syscall.EIO)
	}
	if err := source.Source.ReadAt(ctx, buffer, offset); err != nil {
		return fmt.Errorf("read the film: %w", err)
	}
	return nil
}

// stuckFrom is a film's source whose reads from offset on never return, as a hung share's do, through
// a gate that judges such a read after its window.
type stuckFrom struct {
	fdproxy.Source

	offset int64
	gate   *diskwatch.Gate
	stuck  <-chan struct{}
}

func (source stuckFrom) ReadAt(ctx context.Context, buffer []byte, offset int64) error {
	operation := diskwatch.Operation{Root: "/films", Path: "/films/film.mkv", Window: streamedReadWindow}
	_, err := diskwatch.Run(ctx, source.gate, operation, func() (struct{}, error) {
		if offset+int64(len(buffer)) > source.offset {
			<-source.stuck
			return struct{}{}, fmt.Errorf("read the film: %w", syscall.EIO)
		}
		return struct{}{}, source.Source.ReadAt(ctx, buffer, offset)
	}, func(struct{}) {})
	if err != nil {
		return fmt.Errorf("read the film: %w", err)
	}
	return nil
}

// framesOf is the per-packet checksums of the film's every stream as run reads it: the same for two
// runs only when both read the same bytes.
func framesOf(t *testing.T, runner *Runner, film Film) string {
	t.Helper()
	ran, err := runner.RunWatched(t.Context(), WatchedRun{
		Arguments: append(append([]string{"-nostats"}, film.Input()...),
			"-map", "0", "-c", "copy", "-f", "framemd5", "-"),
		StdoutLimitBytes: 4 << 20,
		LogLimitBytes:    4096,
		Watch:            Watch{StartGrace: time.Minute, StallWindow: time.Minute, Deadline: 2 * time.Minute},
		Film:             film.Stream,
	})
	if err != nil {
		t.Fatalf("read %s: %v", film.Path, err)
	}
	return string(ran.Stdout)
}

// The real FFmpeg reads a film the node streams as it reads the film by its path: every packet of
// every stream the same, its input accounted for with DONE OK (D330).
func TestFDProxy_TheRealFFmpegReadsAStreamedFilmAsItReadsItByPath(t *testing.T) {
	t.Parallel()
	runner := streamingRunner(t)
	path := makeStreamedFilm(t, runner)
	byPath := framesOf(t, runner, Film{Path: path})
	streamed := framesOf(t, runner, streamedFilm(t, runner, path, ReadsWholeFilm))
	if streamed != byPath {
		t.Errorf("the streamed film read differently from the film by path:\nstreamed %d bytes\nby path %d bytes",
			len(streamed), len(byPath))
	}
	if lines := strings.Count(byPath, "\n"); lines < 150 {
		t.Errorf("the film read as %d lines; want a line for each of its packets, 150 of video alone", lines)
	}
}

// The real ffprobe, unwatched as a probe is, reads a streamed film's streams as it reads them by path.
func TestFDProxy_TheRealFFprobeProbesAStreamedFilmAsItProbesItByPath(t *testing.T) {
	t.Parallel()
	runner := streamingRunner(t)
	path := makeStreamedFilm(t, runner)
	probe := func(film Film) string {
		stdout, err := runner.RunOnFilm(t.Context(), FFprobe, film,
			append(append([]string{"-v", "error"}, film.Input()...), "-show_streams", "-of", "json"), 1<<20)
		if err != nil {
			t.Fatalf("probe %s: %v", film.Path, err)
		}
		return string(stdout)
	}
	byPath := probe(Film{Path: path})
	if streamed := probe(streamedFilm(t, runner, path, ReadsIndex)); streamed != byPath {
		t.Errorf("ffprobe saw the streamed film as\n%s\nand the film by path as\n%s", streamed, byPath)
	}
}

// A streamed film whose disk fails part way through a whole read is the disk's failure, never a run
// that passes on what it read (D233 §10, D330).
func TestFDProxy_TheRealFFmpegWhoseFilmFailsPartWayStalledOnItsDisk(t *testing.T) {
	t.Parallel()
	runner := streamingRunner(t)
	film := streamedFilm(t, runner, makeStreamedFilm(t, runner), ReadsWholeFilm)
	film.Stream.Source = failingFrom{Source: film.Stream.Source, offset: film.Size / 2}
	_, err := runner.RunWatched(t.Context(), WatchedRun{
		Arguments:     append(append([]string{"-nostats"}, film.Input()...), "-map", "0", "-f", "null", "-"),
		LogLimitBytes: 4096,
		Watch:         Watch{StartGrace: time.Minute, StallWindow: time.Minute, Deadline: 2 * time.Minute},
		Film:          film.Stream,
	})
	if stalled, isStalled := errors.AsType[*StalledError](err); !isStalled || stalled.Cause != StallDiskFailed {
		t.Errorf("a run whose film failed part way ended %v; want stalled on its disk", err)
	}
}

// ffprobe, whose film's disk fails while it probes, has stalled on its disk too: a probe never
// passes on part of an index.
func TestFDProxy_TheRealFFprobeWhoseFilmFailsWhileProbingStalledOnItsDisk(t *testing.T) {
	t.Parallel()
	runner := streamingRunner(t)
	film := streamedFilm(t, runner, makeStreamedFilm(t, runner), ReadsIndex)
	film.Stream.Source = failingFrom{Source: film.Stream.Source, offset: 0}
	_, err := runner.RunOnFilm(t.Context(), FFprobe, film,
		append(append([]string{"-v", "error"}, film.Input()...), "-show_streams", "-of", "json"), 1<<20)
	if stalled, isStalled := errors.AsType[*StalledError](err); !isStalled || stalled.Cause != StallDiskFailed {
		t.Errorf("a probe whose film failed ended %v; want stalled on its disk", err)
	}
}

// A streamed film whose disk stops answering part way is judged stuck after the read's window, and
// the run is killed then: it neither waits on the hung read nor runs to its watchdog's deadline.
func TestFDProxy_TheRealFFmpegWhoseFilmStopsAnsweringIsKilledAfterTheReadsWindow(t *testing.T) {
	t.Parallel()
	runner := streamingRunner(t)
	film := streamedFilm(t, runner, makeStreamedFilm(t, runner), ReadsWholeFilm)
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	film.Stream.Source = stuckFrom{
		Source: film.Stream.Source, offset: film.Size / 2, gate: diskwatch.NewGate(clock.System{}), stuck: stuck,
	}
	started := time.Now() //nolint:forbidigo // bounds how long the kill took, in real time
	_, err := runner.RunWatched(t.Context(), WatchedRun{
		Arguments:     append(append([]string{"-nostats"}, film.Input()...), "-map", "0", "-f", "null", "-"),
		LogLimitBytes: 4096,
		Watch:         Watch{StartGrace: time.Hour, StallWindow: time.Hour, Deadline: time.Hour},
		Film:          film.Stream,
	})
	took := time.Since(started) //nolint:forbidigo // as above
	if stalled, isStalled := errors.AsType[*StalledError](err); !isStalled || stalled.Cause != StallDiskFailed {
		t.Errorf("a run whose film stopped answering ended %v; want stalled on its disk", err)
	}
	if took > 30*time.Second {
		t.Errorf("the run ended %v after it started; want soon after the read's %v window", took, streamedReadWindow)
	}
}

// A run that asks for more of its film than its budget is killed, and fails for it (Budget).
func TestFDProxy_TheRealFFmpegPastItsBudgetIsKilled(t *testing.T) {
	t.Parallel()
	runner := streamingRunner(t)
	film := streamedFilm(t, runner, makeStreamedFilm(t, runner), ReadsWholeFilm)
	film.Stream.BudgetBytes = film.Size / 4
	_, err := runner.RunWatched(t.Context(), WatchedRun{
		Arguments:     append(append([]string{"-nostats"}, film.Input()...), "-map", "0", "-f", "null", "-"),
		LogLimitBytes: 4096,
		Watch:         Watch{StartGrace: time.Minute, StallWindow: time.Minute, Deadline: 2 * time.Minute},
		Film:          film.Stream,
	})
	if runError, isRunError := errors.AsType[*RunError](err); !isRunError || !errors.Is(err, fdproxy.ErrOverBudget) ||
		runError.ExitStatus != -1 {
		t.Errorf("a run past its budget ended %v; want killed for its budget", err)
	}
}

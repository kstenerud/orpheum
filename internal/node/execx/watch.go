// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// progressFD is the descriptor a watched FFmpeg writes its -progress lines to: its own pipe, so
// a run's stdout stays its output and its stderr its log.
const progressFD = 3

// watchCheckEvery is how often the watchdog judges a run: a few seconds is nothing against stall
// windows of minutes.
const watchCheckEvery = 5 * time.Second

// Graces of a stopped stalled run: FFmpeg first gets SIGINT, its graceful quit, then SIGTERM,
// then its whole process group SIGKILL. Together they stay within waitDelay, so the group is
// killed before Wait stops waiting for its pipes.
const (
	defaultQuitGrace = 3 * time.Second
	defaultTermGrace = 3 * time.Second
)

// deadlineFactor is how many times its expected length a watched run may take before the watchdog
// stops it however it moves: room for a slow disk, a busy processor, or a guess that was wrong.
const deadlineFactor = 3

// shortestDeadline is the least any watched run is allowed, so a quick run whose expected length
// is a few seconds is not stopped by a disk that pauses for a minute.
const shortestDeadline = 10 * time.Minute

// unknownLengthDeadline bounds a run whose expected length nothing tells: a day, far beyond any
// film, so it only ends a run that would otherwise never end.
const unknownLengthDeadline = 24 * time.Hour

// DeadlineFor is the deadline of a run expected to take expected: deadlineFactor times it, and at
// least shortestDeadline. An expected length of zero or less is unknown, and gets a day.
func DeadlineFor(expected time.Duration) time.Duration {
	if expected <= 0 {
		return unknownLengthDeadline
	}
	return max(deadlineFactor*expected, shortestDeadline)
}

// ReadingDeadline is the deadline of a run that reads the whole file at path, an absolute path:
// DeadlineFor the time reading it takes at slowestReadBytesPerSecond, the slowest the node's
// config says it reads (nodeproto.Capacity), or for an unknown length when its size or the rate
// is not known.
func ReadingDeadline(path string, slowestReadBytesPerSecond int64) time.Duration {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= 0 || slowestReadBytesPerSecond <= 0 {
		return DeadlineFor(0)
	}
	return DeadlineFor(time.Duration(info.Size()/slowestReadBytesPerSecond+1) * time.Second)
}

// Progress is where a watched run is, as FFmpeg's -progress last said.
type Progress struct {
	// Position is how far into its output FFmpeg has written (out_time).
	Position time.Duration
	// FrameCount is how many pictures it has written; zero for a run without one.
	FrameCount int64
	// RealtimeFactor is how many seconds of media it made per second of running, as it last said;
	// zero until it says.
	RealtimeFactor float64
}

// Watch is how a run is watched: FFmpeg reports its progress on its own pipe, and the watchdog
// stops the run when its position does not move. Lines that arrive with the position where it
// was are not movement.
type Watch struct {
	// StartGrace is how long the position may take to first move: opening the input, probing it,
	// starting an encoder.
	StartGrace time.Duration
	// StallWindow is how long the position may then stand still.
	StallWindow time.Duration
	// Deadline bounds the whole run, however it moves (DeadlineFor).
	Deadline time.Duration
	// OnProgress, when set, is told each progress FFmpeg reports, from another goroutine, and never
	// after the run has returned.
	OnProgress func(Progress)
}

// StallCause is why the watchdog stopped a run.
type StallCause string

// The causes of a stall.
const (
	// StallNeverMoved is a run whose position did not move within its start grace.
	StallNeverMoved StallCause = "never_moved"
	// StallStoodStill is a run whose position stood still for its stall window.
	StallStoodStill StallCause = "stood_still"
	// StallPastDeadline is a run that moved but did not end by its deadline.
	StallPastDeadline StallCause = "past_deadline"
)

// StalledError is a run the watchdog stopped: FFmpeg hung, or crawled past its deadline. It says
// nothing of the input's bytes, apart from a RunError: the tool, not the file, is what was seen.
type StalledError struct {
	Tool  Tool
	Cause StallCause
	// LastPosition is the furthest position FFmpeg reported.
	LastPosition time.Duration
	// WithoutAdvance is how long the position had not moved when the run was stopped, counted from
	// the start when it never moved.
	WithoutAdvance time.Duration
	StderrTail     string
}

func (stalled *StalledError) Error() string {
	return fmt.Sprintf("%s stalled (%s): stopped at %s of its output after %s without moving; stderr ends: %s",
		stalled.Tool, stalled.Cause, stalled.LastPosition, stalled.WithoutAdvance.Round(time.Second),
		stalled.StderrTail)
}

// watchdog judges a run by its progress: it records where the position last moved, and says when
// the run has stalled.
type watchdog struct {
	clock     clock.Clock
	watch     Watch
	startedAt time.Time

	mutex         sync.Mutex
	furthest      Progress  // guarded by mutex: the furthest position and frame count seen
	hasMoved      bool      // guarded by mutex
	lastAdvanceAt time.Time // guarded by mutex: when the position or frame count last grew
	pausedAt      time.Time // guarded by mutex: when the run was paused; zero while it is not
}

func newWatchdog(clockSource clock.Clock, watch Watch) *watchdog {
	now := clockSource.Now()
	return &watchdog{clock: clockSource, watch: watch, startedAt: now, lastAdvanceAt: now}
}

// observe records one progress report: movement when the position or the frame count grew past
// the furthest seen.
func (dog *watchdog) observe(progress Progress) {
	dog.mutex.Lock()
	defer dog.mutex.Unlock()
	if progress.Position > dog.furthest.Position || progress.FrameCount > dog.furthest.FrameCount {
		dog.furthest.Position = max(dog.furthest.Position, progress.Position)
		dog.furthest.FrameCount = max(dog.furthest.FrameCount, progress.FrameCount)
		dog.hasMoved = true
		dog.lastAdvanceAt = dog.clock.Now()
	}
}

// verdict is the stall the run is in now, or nil while it is fine.
func (dog *watchdog) verdict(tool Tool) *StalledError {
	now := dog.clock.Now()
	dog.mutex.Lock()
	defer dog.mutex.Unlock()
	if !dog.pausedAt.IsZero() {
		return nil
	}
	stalled := &StalledError{
		Tool: tool, LastPosition: dog.furthest.Position, WithoutAdvance: now.Sub(dog.lastAdvanceAt),
	}
	switch {
	case now.Sub(dog.startedAt) >= dog.watch.Deadline:
		stalled.Cause = StallPastDeadline
	case !dog.hasMoved && stalled.WithoutAdvance >= dog.watch.StartGrace:
		stalled.Cause = StallNeverMoved
	case dog.hasMoved && stalled.WithoutAdvance >= dog.watch.StallWindow:
		stalled.Cause = StallStoodStill
	default:
		return nil
	}
	return stalled
}

// readProgress reads FFmpeg's -progress blocks, each a run of key=value lines ending with
// progress=, and hands each to the watchdog and the watch's OnProgress, until the pipe closes.
func (dog *watchdog) readProgress(reader io.Reader) {
	var block Progress
	lines := bufio.NewScanner(reader)
	for lines.Scan() {
		key, value, _ := strings.Cut(lines.Text(), "=")
		value = strings.TrimSpace(value)
		switch key {
		case "out_time_us":
			// "N/A" until FFmpeg has written something.
			if microseconds, err := strconv.ParseInt(value, 10, 64); err == nil && microseconds >= 0 {
				block.Position = time.Duration(microseconds) * time.Microsecond
			}
		case "frame":
			if frames, err := strconv.ParseInt(value, 10, 64); err == nil && frames >= 0 {
				block.FrameCount = frames
			}
		case "speed":
			// "N/A" until FFmpeg has timed something, and "1.2e+03x" when fast; a speed it cannot
			// say leaves the last one.
			if said, err := strconv.ParseFloat(strings.TrimSuffix(value, "x"), 64); err == nil && said > 0 {
				block.RealtimeFactor = said
			}
		case "progress":
			dog.observe(block)
			if dog.watch.OnProgress != nil {
				dog.watch.OnProgress(block)
			}
		}
	}
	// A reader that ends early leaves FFmpeg's pipe unread; drain it so the run is never blocked.
	_, _ = io.Copy(io.Discard, reader) //nolint:errcheck // draining
}

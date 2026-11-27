// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Runner runs FFmpeg watched, as *execx.Runner does.
type Runner interface {
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// makeStartGrace is how long a run's position may take to first move: opening the video on a
// network mount, and on a resume seeking into it.
const makeStartGrace = 2 * time.Minute

// makeStallWindow is how long a run's position may stand still. Each keyframe decoded moves it, and
// a sheet waiting to be taken by the core holds FFmpeg on its pipe; three minutes outlasts an NFS
// client's retries, as a read-through's window does.
const makeStallWindow = 3 * time.Minute

// Maker makes a video's trickplay sheets with the bundled FFmpeg.
type Maker struct {
	runner                    Runner
	slowestReadBytesPerSecond int64
}

// NewMaker is a Maker running FFmpeg by runner, its deadline a whole read of the video at
// slowestReadBytesPerSecond, as a read-through's is.
func NewMaker(runner Runner, slowestReadBytesPerSecond int64) *Maker {
	return &Maker{runner: runner, slowestReadBytesPerSecond: slowestReadBytesPerSecond}
}

// SheetMade takes a finished sheet: its index and its WebP. An error stops the run, and Make returns
// it.
type SheetMade func(index int, webp []byte) error

// Make makes the sheets of film the command asks for, from the first sheet not already made to the
// video's end, giving each to sheetMade as FFmpeg finishes it, in order, and none of those already
// made (trickplay.md §3). It is one FFmpeg run, sought only to resume, in the idle scheduling class.
// A run of FFmpeg that fails is an *execx.RunError, a stall an *execx.StalledError.
func (maker *Maker) Make(
	ctx context.Context, film execx.Film, command nodeproto.MakeTrickplay, sheetMade SheetMade,
) error {
	if err := check(command); err != nil {
		return err
	}
	first := firstNotMade(command.SheetsMade)
	runCtx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	sheets := &sheetSplitter{next: first, stop: stop, made: func(index int, webp []byte) error {
		if slices.Contains(command.SheetsMade, index) {
			return nil
		}
		return sheetMade(index, webp)
	}}
	_, err := maker.runner.RunWatched(runCtx, execx.WatchedRun{
		Access:    confine.Access{Read: film.Readable()},
		Arguments: makeArguments(film.Input(), command, first),
		StdoutTo:  sheets,
		Watch: execx.Watch{
			StartGrace: makeStartGrace, StallWindow: makeStallWindow,
			Deadline: film.ReadingDeadline(maker.slowestReadBytesPerSecond),
		},
		Film:   film.Stream,
		IsIdle: true,
	})
	// A sheet the splitter or its taker refused stopped the run, which says only that it was
	// cancelled: the refusal is the answer.
	if cause := context.Cause(runCtx); ctx.Err() == nil && cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	if err != nil {
		return fmt.Errorf("make the trickplay sheets: %w", err)
	}
	if err := sheets.finish(); err != nil {
		return err
	}
	return nil
}

// check refuses sizes no sheet can be made of.
func check(command nodeproto.MakeTrickplay) error {
	if command.IntervalMs <= 0 || command.PictureWidth <= 0 || command.SheetColumns <= 0 ||
		command.SheetRows <= 0 || command.WebPQuality < 0 || command.WebPQuality > 100 {
		return fmt.Errorf("trickplay of %d ms, %d px, %d×%d sheets at quality %d cannot be made",
			command.IntervalMs, command.PictureWidth, command.SheetColumns, command.SheetRows, command.WebPQuality)
	}
	return nil
}

// firstNotMade is the lowest sheet index not among made.
func firstNotMade(made []int) int {
	first := 0
	for slices.Contains(made, first) {
		first++
	}
	return first
}

// squarePixels makes a picture of non-square pixels square at its height, as a frame still does.
const squarePixels = "scale='trunc(iw*sar/2)*2':ih,setsar=1"

// makeArguments are the FFmpeg arguments of a run from sheet first. Only keyframes are decoded, and
// fps, its marks from the video's start (start_time=0) and rounding up, takes at each mark the last
// keyframe at or before it. A resume seeks to its first mark without decoding there
// (-noaccurate_seek), to the keyframe at or before it, and its times start there less the mark, so
// fps counts its marks from the mark and picks the same keyframes a run from the start would. The
// tile filter lays them in sheets, flushing the last, short one at the end, and each sheet is a WebP of its own
// on stdout.
func makeArguments(input []string, command nodeproto.MakeTrickplay, first int) []string {
	arguments := []string{"-v", "error", "-skip_frame", "nokey"}
	interval := time.Duration(command.IntervalMs) * time.Millisecond
	if from := time.Duration(first*command.SheetColumns*command.SheetRows) * interval; from > 0 {
		arguments = append(arguments, "-ss", strconv.FormatFloat(from.Seconds(), 'f', 3, 64), "-noaccurate_seek")
	}
	filters := squarePixels +
		",fps=1/" + strconv.FormatFloat(interval.Seconds(), 'f', 3, 64) + ":start_time=0:round=up" +
		",scale=" + strconv.Itoa(command.PictureWidth) + ":-2" +
		",tile=" + strconv.Itoa(command.SheetColumns) + "x" + strconv.Itoa(command.SheetRows)
	arguments = append(arguments, input...)
	return append(arguments,
		"-map", "0:"+strconv.Itoa(command.VideoStreamIndex), "-an", "-sn", "-dn",
		"-vf", filters,
		"-map_metadata", "-1", "-map_chapters", "-1",
		"-c:v", "libwebp", "-lossless", "0", "-quality", strconv.Itoa(command.WebPQuality),
		"-f", "image2pipe", "-flush_packets", "1", "pipe:1")
}

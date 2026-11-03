// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// spanToleranceMs is how far a raw file's span, as the muxer lists it, may be from its chunk's: the
// list times a picture by its shown times, some frames off its keyframes' and a cut 50 ms early,
// where a cut a keyframe off, or a file FFmpeg was stopped in, is off by a stretch between
// keyframes or more (idiosyncrasies.md).
const spanToleranceMs = 500

// errNotHeld is a raw file whose span is not its chunk's: it is never made a chunk.
var errNotHeld = errors.New("FFmpeg cut it elsewhere or was stopped in it")

// listed is one line of the muxer's list: a raw file it finished, the chunk it is (number, once
// taken: its name's number less rawNumberShift) and the span it holds, in
// milliseconds from the run's first frame, or, for a sound copied, in the film (copiedSoundTimes);
// and its end in seconds as listed, to the microsecond, by which a sound copied is placed.
type listed struct {
	name           string
	number         int
	startMs, endMs int64
	endSeconds     float64
}

// parseListed reads a list line: "<name>,<start seconds>,<end seconds>".
func parseListed(line string) (listed, error) {
	fields := strings.Split(line, ",")
	if len(fields) != 3 {
		return listed{}, fmt.Errorf("the run's list says %q", line)
	}
	number, err := strconv.Atoi(strings.TrimSuffix(fields[0], ".mp4"))
	if err != nil {
		return listed{}, fmt.Errorf("the run's list names %q", fields[0])
	}
	var times [2]float64
	for index, field := range fields[1:] {
		seconds, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return listed{}, fmt.Errorf("the run's list times %q at %q", fields[0], field)
		}
		times[index] = seconds
	}
	return listed{
		name: fields[0], number: number, endSeconds: times[1],
		startMs: int64(math.Round(times[0] * 1000)), endMs: int64(math.Round(times[1] * 1000)),
	}, nil
}

// chunkSpan is the span raw file number of the run should hold, in milliseconds from the run's
// start: its chunk's, or, for a lead-in, from the run's start to its first chunk's. A sound
// copied is listed at its times in the film, its first packet at the film's start
// (copiedSoundTimes): its chunks' spans are their own, and its lead-in's from the film's start.
func chunkSpan(command nodeproto.MakeChunks, number int) (fromMs, toMs int64) {
	origin := runStartMs(command)
	if command.Stream.Kind == nodeproto.StreamSoundCopied {
		origin = 0
	}
	if number < command.FirstChunk {
		return 0, command.BoundsMs[0] - origin
	}
	index := number - command.FirstChunk
	return command.BoundsMs[index] - origin, command.BoundsMs[index+1] - origin
}

// checkSpan fails a raw file whose listed span is not its chunk's. A file closed by FFmpeg's end
// and not by a cut, the run's last chunk when FFmpeg ended cleanly, may end before its chunk does,
// as a film's last picture may. The list times a run from its first frame, so a whole run begun a
// keyframe early passes when its chunks are as long as the stretches between keyframes: a copied
// picture's seek past its first keyframe is what keeps that from happening.
func checkSpan(command nodeproto.MakeChunks, raw listed, isClosedByTheEnd bool) error {
	fromMs, toMs := chunkSpan(command, raw.number)
	isStartOff := math.Abs(float64(raw.startMs-fromMs)) > spanToleranceMs
	isEndOff := raw.endMs > toMs+spanToleranceMs || (!isClosedByTheEnd && raw.endMs < toMs-spanToleranceMs)
	if isStartOff || isEndOff {
		return fmt.Errorf("raw %s holds %d to %d ms of the run, not its chunk %d's %d to %d ms "+
			"(the run read from %d ms, its first cut at %s s): %w",
			raw.name, raw.startMs, raw.endMs, raw.number, fromMs, toMs, seekMs(command), firstCut(command), errNotHeld)
	}
	return nil
}

// firstCut is the first time the run's muxer was told to cut at, in its seconds: what a raw file
// held wrongly is first checked against.
func firstCut(command nodeproto.MakeChunks) string {
	first, _, _ := strings.Cut(cutTimes(command), ",")
	return first
}

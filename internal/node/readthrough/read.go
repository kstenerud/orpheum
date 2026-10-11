// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Runner runs FFmpeg, watched for a stall, and returns the end of its log: execx.Runner.
type Runner interface {
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// readStartGrace is how long a read's position may take to first move (execx.Watch): opening the
// video on a network mount and probing it.
const readStartGrace = 2 * time.Minute

// readStallWindow is how long a read's position may stand still. Every frame decoded or packet
// copied moves it, so it stands still only while FFmpeg reads nothing; three minutes outlasts an
// NFS client's retries (a 60-second timeout, retransmitted twice by default).
const readStallWindow = 3 * time.Minute

// Reader reads videos through with the bundled FFmpeg.
type Reader struct {
	runner Runner
	// slowestReadBytesPerSecond is the slowest the node reads a whole file (nodeproto.Capacity).
	slowestReadBytesPerSecond int64
}

// NewReader returns the reader, whose watchdog allows a read as long as reading its file at
// slowestReadBytesPerSecond takes.
func NewReader(runner Runner, slowestReadBytesPerSecond int64) *Reader {
	return &Reader{runner: runner, slowestReadBytesPerSecond: slowestReadBytesPerSecond}
}

// Findings are what a read learned of a video: each part it was asked for.
type Findings struct {
	// Loudness is each asked track's measurements; empty when none was asked for.
	Loudness []nodeproto.TrackLoudness
	// VideoTimeline is the picture's keyframes and bytes in each second; nil when not asked for.
	VideoTimeline *nodeproto.VideoTimeline
}

// Read reads the video at videoPath, an absolute path the caller has checked, through once, and
// learns what the command asks: the loudness of each of its loudnessTracks, a track with more than
// two channels as it is and as the standard stereo mix a prepare makes of it, any other as stereo
// (D226 §1); and, when timeline is set, its picture's timeline (D247 §10). The film's length,
// duration, is 0 when unknown: decoding its sound takes as long as the film plays on the slowest
// machine, so when loudness is asked the watchdog allows the longer of that and reading the file.
func (reader *Reader) Read(
	ctx context.Context, videoPath string, loudnessTracks []nodeproto.LoudnessTrack,
	timeline *nodeproto.VideoTimelineAsk, duration time.Duration,
) (Findings, error) {
	if len(loudnessTracks) == 0 && timeline == nil {
		return Findings{}, errors.New("a read-through asked for nothing")
	}
	arguments := append([]string{"-nostats"}, execx.FileInput(videoPath)...)
	arguments = append(arguments, loudnessOutput(loudnessTracks)...)
	run := execx.WatchedRun{
		Access: confine.Access{Read: []string{videoPath}}, LogLimitBytes: maxLogBytes,
		Watch: execx.Watch{
			StartGrace: readStartGrace, StallWindow: readStallWindow,
			Deadline: reader.deadline(videoPath, len(loudnessTracks) > 0, duration),
		},
	}
	var packets *packetLines
	if timeline != nil {
		packets = &packetLines{}
		run.StdoutTo = packets
		arguments = append(arguments, timelineOutput(timeline.VideoStreamIndex)...)
	}
	run.Arguments = arguments
	ran, err := reader.runner.RunWatched(ctx, run)
	if err != nil {
		return Findings{}, fmt.Errorf("read the video through: %w", err)
	}
	var findings Findings
	if len(loudnessTracks) > 0 {
		if findings.Loudness, err = loudnessOf(ran.Log, loudnessTracks); err != nil {
			return Findings{}, err
		}
	}
	if packets != nil {
		videoTimeline, timelineErr := packets.timeline()
		if timelineErr != nil {
			return Findings{}, timelineErr
		}
		findings.VideoTimeline = &videoTimeline
	}
	return findings, nil
}

// deadline is how long a read may run however it moves: the deadline of reading the whole file,
// or, when it measures loudness, of decoding its sound at real time, the slowest a machine decodes
// it, whichever is longer. A two-hour film with four tracks over a network mount took sixteen
// minutes at 7.9 times real time, past the ten its reading allowed, and was stopped near its end
// (the owner's first Watch Together test, 2026-10-05). Copying the picture's packets for its
// timeline decodes nothing, so it takes as long as reading.
func (reader *Reader) deadline(videoPath string, isMeasuringLoudness bool, duration time.Duration) time.Duration {
	reading := execx.ReadingDeadline(videoPath, reader.slowestReadBytesPerSecond)
	if !isMeasuringLoudness || duration <= 0 {
		return reading
	}
	return max(reading, execx.DeadlineFor(duration))
}

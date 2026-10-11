// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitle

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
)

// MaxTextBytes bounds one subtitle in either format. A film's dialogue is well under a
// megabyte; a heavily typeset ASS track can reach a few.
const MaxTextBytes = 16 << 20

// Runner runs FFmpeg, watched for a stall, and returns its stdout: execx.Runner.
type Runner interface {
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// How an extraction is watched (execx.Watch). A text track's position moves only when a subtitle
// is written, and a film can go many minutes without one (a forced track, for most of its
// length), so a track's run is given long enough to read through a large file's longest quiet
// stretch on a slow disk. A conversion reads a few megabytes of text from memory, in well under a
// second.
const (
	trackStartGrace     = 10 * time.Minute
	trackStallWindow    = 10 * time.Minute
	convertStartGrace   = time.Minute
	convertStallWindow  = time.Minute
	convertTextDeadline = 5 * time.Minute
)

// Extractor makes ASS and WebVTT with the bundled FFmpeg.
type Extractor struct {
	runner Runner
	// slowestReadBytesPerSecond is the slowest the node reads a whole file (nodeproto.Capacity).
	slowestReadBytesPerSecond int64
}

// NewExtractor returns the extractor, whose watchdog allows an extraction as long as reading its
// file at slowestReadBytesPerSecond takes.
func NewExtractor(runner Runner, slowestReadBytesPerSecond int64) *Extractor {
	return &Extractor{runner: runner, slowestReadBytesPerSecond: slowestReadBytesPerSecond}
}

// Text is one subtitle in both formats.
type Text struct {
	ASS []byte
	VTT []byte
}

// ExtractTrack extracts the text track at streamIndex of the video at videoPath, an absolute
// path the caller has checked.
func (extractor *Extractor) ExtractTrack(ctx context.Context, videoPath string, streamIndex int) (Text, error) {
	ran, err := extractor.runner.RunWatched(ctx, execx.WatchedRun{
		Access:           confine.Access{Read: []string{videoPath}},
		Arguments:        arguments(execx.FileInput(videoPath), "0:"+strconv.Itoa(streamIndex), codecASS),
		StdoutLimitBytes: MaxTextBytes,
		Watch: execx.Watch{
			StartGrace: trackStartGrace, StallWindow: trackStallWindow,
			Deadline: execx.ReadingDeadline(videoPath, extractor.slowestReadBytesPerSecond),
		},
	})
	if err != nil {
		return Text{}, fmt.Errorf("extract stream %d as ASS: %w", streamIndex, err)
	}
	return extractor.withVTT(ctx, ran.Stdout)
}

// ConvertText converts a subtitle's text — a sidecar's bytes or a fetched subtitle's — in any
// text format FFmpeg recognises.
func (extractor *Extractor) ConvertText(ctx context.Context, source []byte) (Text, error) {
	ass, err := extractor.convert(ctx, arguments(execx.PipeInput(), "0:s:0", codecASS), source)
	if err != nil {
		return Text{}, fmt.Errorf("convert the file to ASS: %w", err)
	}
	return extractor.withVTT(ctx, ass)
}

// withVTT converts the ASS to WebVTT, so the video is read only once.
func (extractor *Extractor) withVTT(ctx context.Context, ass []byte) (Text, error) {
	vtt, err := extractor.convert(ctx, arguments(execx.PipeInput(), "0:s:0", codecWebVTT, "-f", codecASS), ass)
	if err != nil {
		return Text{}, fmt.Errorf("convert the ASS to WebVTT: %w", err)
	}
	return Text{ASS: ass, VTT: vtt}, nil
}

// convert runs one conversion of text fed on a pipe, and returns what it wrote.
func (extractor *Extractor) convert(ctx context.Context, arguments []string, text []byte) ([]byte, error) {
	ran, err := extractor.runner.RunWatched(ctx, execx.WatchedRun{
		Arguments: arguments, Input: text, StdoutLimitBytes: MaxTextBytes,
		Watch: execx.Watch{
			StartGrace: convertStartGrace, StallWindow: convertStallWindow, Deadline: convertTextDeadline,
		},
	})
	if err != nil {
		return nil, err
	}
	return ran.Stdout, nil
}

// The codecs written, each in the format of the same name.
const (
	codecASS    = "ass"
	codecWebVTT = "webvtt"
)

// arguments read input, execx.FileInput or execx.PipeInput, and write the stream mapping names, as
// codec, to stdout. When inputFormatArguments are given, they name the input's format rather than
// letting FFmpeg guess.
func arguments(input []string, mapping, codec string, inputFormatArguments ...string) []string {
	read := append(append([]string{"-v", "error"}, inputFormatArguments...), input...)
	return append(read, "-map", mapping, "-c:s", codec, "-f", codec, "pipe:1")
}

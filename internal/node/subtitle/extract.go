// SPDX-License-Identifier: GPL-3.0-or-later

package subtitle

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
)

// MaxTextBytes bounds one subtitle in either format. A film's dialogue is well under a
// megabyte; a heavily typeset ASS track can reach a few.
const MaxTextBytes = 16 << 20

// Runner runs FFmpeg and returns its stdout: execx.Runner.
type Runner interface {
	Run(
		ctx context.Context, tool execx.Tool, access confine.Access, arguments []string, stdoutLimitBytes int,
	) ([]byte, error)
	RunInput(
		ctx context.Context, tool execx.Tool, access confine.Access, arguments []string, input []byte,
		stdoutLimitBytes int,
	) ([]byte, error)
}

// Extractor makes ASS and WebVTT with the bundled FFmpeg.
type Extractor struct {
	runner Runner
}

// NewExtractor returns the extractor.
func NewExtractor(runner Runner) *Extractor {
	return &Extractor{runner: runner}
}

// Text is one subtitle in both formats.
type Text struct {
	ASS []byte
	VTT []byte
}

// ExtractTrack extracts the text track at streamIndex of the video at videoPath, an absolute
// path the caller has checked.
func (extractor *Extractor) ExtractTrack(ctx context.Context, videoPath string, streamIndex int) (Text, error) {
	ass, err := extractor.runner.Run(ctx, execx.FFmpeg, confine.Access{Read: []string{videoPath}},
		arguments(execx.FileInput(videoPath), "0:"+strconv.Itoa(streamIndex), codecASS), MaxTextBytes)
	if err != nil {
		return Text{}, fmt.Errorf("extract stream %d as ASS: %w", streamIndex, err)
	}
	return extractor.withVTT(ctx, ass)
}

// ConvertText converts a subtitle's text — a sidecar's bytes or a fetched subtitle's — in any
// text format FFmpeg recognises.
func (extractor *Extractor) ConvertText(ctx context.Context, source []byte) (Text, error) {
	ass, err := extractor.runner.RunInput(ctx, execx.FFmpeg, confine.Access{},
		arguments(execx.PipeInput(), "0:s:0", codecASS), source, MaxTextBytes)
	if err != nil {
		return Text{}, fmt.Errorf("convert the file to ASS: %w", err)
	}
	return extractor.withVTT(ctx, ass)
}

// withVTT converts the ASS to WebVTT, so the video is read only once.
func (extractor *Extractor) withVTT(ctx context.Context, ass []byte) (Text, error) {
	vtt, err := extractor.runner.RunInput(ctx, execx.FFmpeg, confine.Access{},
		arguments(execx.PipeInput(), "0:s:0", codecWebVTT, "-f", codecASS), ass, MaxTextBytes)
	if err != nil {
		return Text{}, fmt.Errorf("convert the ASS to WebVTT: %w", err)
	}
	return Text{ASS: ass, VTT: vtt}, nil
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

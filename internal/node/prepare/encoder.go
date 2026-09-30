// SPDX-License-Identifier: GPL-3.0-or-later

package prepare

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
)

// Encoder is the H.264 encoder an encode uses: Quick Sync, else NVENC, else x264 (cut-a §6).
type Encoder string

// The encoders, in the order a node tries them. Their names are the hello's capabilities (cut-a
// §4): what the node found it can use.
const (
	EncoderQuickSync Encoder = "h264_qsv"
	EncoderNVENC     Encoder = "h264_nvenc"
	EncoderX264      Encoder = "x264"
)

// hardwareEncoders are tried in order; x264 needs no trial.
func hardwareEncoders() []Encoder { return []Encoder{EncoderQuickSync, EncoderNVENC} }

// UsesGPU is whether an encode with it opens the GPU, which its confinement must grant.
func (encoder Encoder) UsesGPU() bool { return encoder != EncoderX264 }

// encodeArguments are the encoder's video arguments: 8-bit H.264 at about 8 Mb/s, the high profile,
// and a keyframe every two seconds, however it is made (cut-a §6).
func (encoder Encoder) encodeArguments() []string {
	codec, preset, pixelFormat, extra := "libx264", "veryfast", "yuv420p", []string(nil)
	switch encoder {
	case EncoderQuickSync:
		codec, pixelFormat = "h264_qsv", "nv12"
	case EncoderNVENC:
		codec, preset, extra = "h264_nvenc", "p4", []string{"-forced-idr", "1"}
	case EncoderX264:
	}
	return append([]string{
		"-c:v", codec, "-preset", preset, "-profile:v", "high", "-pix_fmt", pixelFormat,
		"-b:v", "8M", "-maxrate", "10M", "-bufsize", "16M", "-force_key_frames", "expr:gte(t,n_forced*2)",
	}, extra...)
}

// trialTimeout bounds one encoder's trial: a GPU that hangs is not one to encode with.
const trialTimeout = 30 * time.Second

// Trial is one hardware encoder's trial on this machine: it worked, or Failure says why, in
// FFmpeg's words (D225 §3).
type Trial struct {
	Encoder Encoder
	Failure string
}

// Choice is the encoder a node prepares with, and the trials that chose it.
type Choice struct {
	Chosen Encoder
	Trials []Trial
}

// Usable is every encoder the node can use: the hardware ones whose trial worked, and x264.
func (choice Choice) Usable() []Encoder {
	var usable []Encoder
	for _, trial := range choice.Trials {
		if trial.Failure == "" {
			usable = append(usable, trial.Encoder)
		}
	}
	return append(usable, EncoderX264)
}

// TrialRunner runs an encoder's trial to its end, streaming its stdout: execx.Runner.
type TrialRunner interface {
	RunStreaming(
		ctx context.Context, tool execx.Tool, access confine.Access, arguments []string, stdout io.Writer,
	) error
}

// ChooseEncoder tries each hardware encoder, encoding a second of test picture on this machine,
// confined as a prepare is, and chooses the first that works, or x264 when none does.
func ChooseEncoder(ctx context.Context, runner TrialRunner) Choice {
	choice := Choice{Chosen: EncoderX264}
	for _, encoder := range hardwareEncoders() {
		trial := Trial{Encoder: encoder}
		if err := tryEncoder(ctx, runner, encoder); err != nil {
			trial.Failure = err.Error()
		} else if choice.Chosen == EncoderX264 {
			choice.Chosen = encoder
		}
		choice.Trials = append(choice.Trials, trial)
	}
	return choice
}

// tryEncoder encodes a second of generated test picture with encoder, to nothing.
func tryEncoder(ctx context.Context, runner TrialRunner, encoder Encoder) error {
	ctx, cancel := context.WithTimeout(ctx, trialTimeout)
	defer cancel()
	arguments := append([]string{
		"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25:duration=1",
	}, encoder.encodeArguments()...)
	arguments = append(arguments, "-f", "null", "-")
	err := runner.RunStreaming(ctx, execx.FFmpeg, confine.Access{UsesGPU: true}, arguments, io.Discard)
	if err != nil {
		return fmt.Errorf("try %s: %w", encoder, err)
	}
	return nil
}

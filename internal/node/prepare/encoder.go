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

// arguments are the encoder's video arguments: 8-bit H.264 at about 8 Mb/s, the high profile,
// and a keyframe every two seconds, however it is made (cut-a §6).
func (encoder Encoder) arguments() []string {
	codec, preset, pixels, extra := "libx264", "veryfast", "yuv420p", []string(nil)
	switch encoder {
	case EncoderQuickSync:
		codec, pixels = "h264_qsv", "nv12"
	case EncoderNVENC:
		codec, preset, extra = "h264_nvenc", "p4", []string{"-forced-idr", "1"}
	case EncoderX264:
	}
	return append([]string{
		"-c:v", codec, "-preset", preset, "-profile:v", "high", "-pix_fmt", pixels,
		"-b:v", "8M", "-maxrate", "10M", "-bufsize", "16M", "-force_key_frames", "expr:gte(t,n_forced*2)",
	}, extra...)
}

// trialTimeout bounds one encoder's trial: a GPU that hangs is not one to encode with.
const trialTimeout = 30 * time.Second

// ChooseEncoder is the first hardware encoder that encodes a second of test picture on this
// machine, confined as a prepare is, or x264 when none does. Every encoder the node can use is
// in usable, x264 always.
func ChooseEncoder(ctx context.Context, runner Runner) (chosen Encoder, usable []Encoder) {
	chosen = EncoderX264
	for _, encoder := range hardwareEncoders() {
		if tryEncoder(ctx, runner, encoder) == nil {
			usable = append(usable, encoder)
			if chosen == EncoderX264 {
				chosen = encoder
			}
		}
	}
	return chosen, append(usable, EncoderX264)
}

// tryEncoder encodes a second of generated test picture with encoder, to nothing.
func tryEncoder(ctx context.Context, runner Runner, encoder Encoder) error {
	ctx, cancel := context.WithTimeout(ctx, trialTimeout)
	defer cancel()
	arguments := append([]string{
		"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25:duration=1",
	}, encoder.arguments()...)
	arguments = append(arguments, "-f", "null", "-")
	err := runner.RunStreaming(ctx, execx.FFmpeg, confine.Access{Devices: true}, arguments, io.Discard)
	if err != nil {
		return fmt.Errorf("try %s: %w", encoder, err)
	}
	return nil
}

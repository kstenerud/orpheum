// SPDX-License-Identifier: AGPL-3.0-or-later

package encode

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Encoder is the H.264 encoder an encode uses: Quick Sync, else NVENC, else VAAPI, else x264 (cut-a §6,
// D332).
type Encoder string

// The encoders, in the order a node tries them. Their names are the hello's capabilities (cut-a
// §4): what the node found it can use.
const (
	EncoderQuickSync Encoder = nodeproto.EncoderQuickSync
	EncoderNVENC     Encoder = nodeproto.EncoderNVENC
	EncoderVAAPI     Encoder = nodeproto.EncoderVAAPI
	EncoderX264      Encoder = nodeproto.EncoderX264
)

// hardwareEncoders are tried in order; x264 needs no trial.
func hardwareEncoders() []Encoder { return []Encoder{EncoderQuickSync, EncoderNVENC, EncoderVAAPI} }

// vaapiDevice opens FFmpeg's own VAAPI device for the run, the first render node, and makes it the
// one filters upload to (D332).
func vaapiDevice() []string {
	return []string{"-init_hw_device", "vaapi=va", "-filter_hw_device", "va"}
}

// vaapiUpload converts a picture to NV12 and uploads it to the card, which h264_vaapi encodes from.
const vaapiUpload = "format=nv12,hwupload"

// UsesGPU is whether an encode with it opens the GPU, which its confinement must grant.
func (encoder Encoder) UsesGPU() bool { return encoder != EncoderX264 }

// QualityArguments are the encoder's video arguments for a chunk's quality (D247 §6): 8-bit H.264
// at kbps, its peak at most 1.5 times that, scaled to height with the source's shape, and keyframes
// at keyframesAt, a list of seconds, and nowhere else, so its chunks cut where the copied
// picture's do (slow-links.md §10).
func (encoder Encoder) QualityArguments(kbps, height int, keyframesAt string) []string {
	rate := func(kbps int) string { return strconv.Itoa(kbps) + "k" }
	scale := "scale=-2:" + strconv.Itoa(height)
	rates := []string{
		"-profile:v", "high", "-b:v", rate(kbps), "-maxrate", rate(kbps * 3 / 2), "-bufsize", rate(2 * kbps),
		"-g", "100000", "-force_key_frames", keyframesAt,
	}
	if encoder == EncoderVAAPI {
		// It has no preset, and the scaled picture reaches it uploaded rather than in a pixel format.
		return slices.Concat(vaapiDevice(), []string{codecFlag, "h264_vaapi", "-vf", scale + "," + vaapiUpload}, rates)
	}
	codec, preset, pixelFormat, extra := "libx264", "veryfast", "yuv420p", []string{"-sc_threshold", "0"}
	switch encoder {
	case EncoderQuickSync:
		// Without it, Quick Sync makes a forced keyframe an I picture but not an IDR, which no chunk can
		// start on (measured on an i3-N305, 2026-10-23).
		codec, pixelFormat, extra = "h264_qsv", "nv12", []string{"-forced_idr", "1"}
	case EncoderNVENC:
		codec, preset, extra = "h264_nvenc", "p4", []string{"-forced-idr", "1"}
	case EncoderVAAPI, EncoderX264:
	}
	return slices.Concat(
		[]string{codecFlag, codec, "-preset", preset, "-vf", scale, "-pix_fmt", pixelFormat}, rates, extra,
	)
}

// codecFlag names a stream's codec on FFmpeg's command line.
const codecFlag = "-c:v"

// trialTimeout bounds one encoder's trial: a GPU that hangs is not one to encode with.

// A trial encodes its test picture as a chunk of a low quality is encoded.
const (
	trialKbps   = 1000
	trialHeight = 360
)
const trialTimeout = 30 * time.Second

// Trial is one hardware encoder's trial on this machine: it worked, or Failure says why, in
// FFmpeg's words (D225 §3).
type Trial struct {
	Encoder Encoder
	Failure string
}

// Choice is the encoder a node encodes chunks with, and the trials that chose it.
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
// confined as a run of chunks is, and chooses the first that works, or x264 when none does.
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
	}, encoder.QualityArguments(trialKbps, trialHeight, "0")...)
	arguments = append(arguments, "-f", "null", "-")
	err := runner.RunStreaming(ctx, execx.FFmpeg, confine.Access{UsesGPU: true}, arguments, io.Discard)
	if err != nil {
		return fmt.Errorf("try %s: %w", encoder, err)
	}
	return nil
}

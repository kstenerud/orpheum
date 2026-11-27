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

// startAtZero is the filter that starts an encode's times at its first picture, as a copied
// picture's start at its first keyframe, so its forced keyframes and the segment muxer's cuts count
// from the same picture. FFmpeg forces a keyframe at the first picture shown at or after each time
// of -force_key_frames on its own clock, from the file's start, while the segment muxer counts its
// cuts from its first packet (idiosyncrasies.md): a picture starting after the file does, as an
// MP4's empty edit delays one for a sound starting before it, had every keyframe forced that much
// earlier than its cut, which missed it, so an encode from the film's start never made chunk 0
// (file 1282's picture 105 ms after its sound, staging, 2026-11-26). A run sought into the film
// starts within a picture of zero, and is moved that little.
const startAtZero = "setpts=PTS-STARTPTS"

// vaapiUpload converts a picture to NV12 and uploads it to the card, which h264_vaapi encodes from.
const vaapiUpload = "format=nv12,hwupload"

// Decoding is where an encode's picture is decoded and scaled: on the CPU, as every encode's was, or
// on the GPU, the picture kept on the card from the decoder to the encoder.
type Decoding string

// The places an encode's picture is decoded, as a run's log line names them (decodedOn).
const (
	DecodedOnCPU Decoding = "cpu"
	DecodedOnGPU Decoding = "gpu"
)

// vaapiDecodes is whether codec, by the name FFmpeg's probe gives it, is one the pinned FFmpeg (8.1)
// has a VAAPI decoder for (its libavcodec/hwaccels.h); a card decodes those of them its driver
// offers, at the profiles it offers.
func vaapiDecodes(codec string) bool {
	switch codec {
	case "av1", "h263", "h264", "hevc", "mjpeg", "mpeg2video", "mpeg4", "vc1", "vp8", "vp9", "vvc", "wmv3":
		return true
	}
	return false
}

// DecodesOnGPU is whether an encode of a picture in codec is first tried decoded and scaled on the
// GPU it is encoded on: on VAAPI, of a codec FFmpeg decodes through it. Kept on the card, an HEVC
// Main 10 picture at 1080p encoded at 5.94 times real time on production's Celeron J3455, not 2.65
// decoded and scaled on its four cores, and at 432p at 11.4, not 3.09, on a tenth of the CPU (the
// owner, 2026-11-27: "We should be using the hardware available to us for as much as we can").
// Quick Sync's, NVENC's and x264's encodes decode on the CPU as before.
func (encoder Encoder) DecodesOnGPU(codec string) bool {
	return encoder == EncoderVAAPI && vaapiDecodes(codec)
}

// DecodeArguments are an encode's input options, before its film: decoded on the GPU, by VAAPI on
// the run's device (vaapiDevice), each picture left on the card; none on the CPU. Where the card
// cannot open the decoder for the picture's codec or profile, FFmpeg decodes on the CPU by itself
// and hands the filter a picture in memory, which scale_vaapi cannot take: the run fails at its first
// picture, and the maker runs it again decoded on the CPU (internal/node/chunks, idiosyncrasies.md).
func (encoder Encoder) DecodeArguments(decoding Decoding) []string {
	if encoder != EncoderVAAPI || decoding != DecodedOnGPU {
		return nil
	}
	return []string{"-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi"}
}

// vaapiScale scales a picture on the card to height lines with its shape and makes it NV12, 8-bit,
// as h264_vaapi takes it, reckoning a -2 width and the sample aspect ratio as the CPU's scale does
// (FFmpeg's ff_scale_eval_dimensions and ff_scale_adjust_dimensions), so a picture decoded on the GPU
// comes out the size and shape of one decoded on the CPU (idiosyncrasies.md).
func vaapiScale(height int) string {
	return "scale_vaapi=w=-2:h=" + strconv.Itoa(height) + ":format=nv12"
}

// UsesGPU is whether an encode with it opens the GPU, which its confinement must grant.
func (encoder Encoder) UsesGPU() bool { return encoder != EncoderX264 }

// QualityArguments are the encoder's video arguments for a chunk's quality (D247 §6): 8-bit H.264
// at kbps, its peak at most 1.5 times that, scaled to height with the source's shape, and keyframes
// at keyframesAt, a list of seconds from its first picture, and nowhere else, so its chunks cut where
// the copied picture's do (slow-links.md §10). Its times start at its first picture (startAtZero).
// On VAAPI a picture decoded on the GPU (DecodeArguments) is scaled there by vaapiScale, and one
// decoded on the CPU is scaled on the CPU and uploaded; the other encoders take one decoded on the CPU.
func (encoder Encoder) QualityArguments(kbps, height int, keyframesAt string, decoding Decoding) []string {
	rate := func(kbps int) string { return strconv.Itoa(kbps) + "k" }
	scale := startAtZero + ",scale=-2:" + strconv.Itoa(height)
	rates := []string{
		"-profile:v", "high", "-b:v", rate(kbps), "-maxrate", rate(kbps * 3 / 2), "-bufsize", rate(2 * kbps),
		"-g", "100000", "-force_key_frames", keyframesAt,
	}
	if encoder == EncoderVAAPI {
		// It has no preset, and the scaled picture reaches it on the card rather than in a pixel format.
		filter := scale + "," + vaapiUpload
		if decoding == DecodedOnGPU {
			filter = startAtZero + "," + vaapiScale(height)
		}
		return slices.Concat(vaapiDevice(), []string{codecFlag, "h264_vaapi", "-vf", filter}, rates)
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
	}, encoder.QualityArguments(trialKbps, trialHeight, "0", DecodedOnCPU)...)
	arguments = append(arguments, "-f", "null", "-")
	err := runner.RunStreaming(ctx, execx.FFmpeg, confine.Access{UsesGPU: true}, arguments, io.Discard)
	if err != nil {
		return fmt.Errorf("try %s: %w", encoder, err)
	}
	return nil
}

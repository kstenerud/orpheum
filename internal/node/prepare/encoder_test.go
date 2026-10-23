// SPDX-License-Identifier: AGPL-3.0-or-later

package prepare_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeTrialRunner is a machine whose encoders work or not, as works says, recording each trial's access.
type fakeTrialRunner struct {
	worksByEncoder map[string]bool
	accesses       []confine.Access
}

func (machine *fakeTrialRunner) RunStreaming(
	_ context.Context, _ execx.Tool, access confine.Access, arguments []string, _ io.Writer,
) error {
	machine.accesses = append(machine.accesses, access)
	if machine.worksByEncoder[after(arguments, "-c:v")] {
		return nil
	}
	return errors.New("no such device")
}

func TestChooseEncoder_TakesTheFirstHardwareEncoderThatWorksConfinedOrX264(t *testing.T) {
	t.Parallel()
	nvidia := &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_nvenc": true}}
	choice := prepare.ChooseEncoder(t.Context(), nvidia)
	wantUsable := []prepare.Encoder{prepare.EncoderNVENC, prepare.EncoderX264}
	if choice.Chosen != prepare.EncoderNVENC || !slices.Equal(choice.Usable(), wantUsable) {
		t.Errorf("with NVENC only: %+v; want NVENC, and x264 usable too", choice)
	}
	for _, access := range nvidia.accesses {
		if !access.UsesGPU {
			t.Errorf("a trial ran without the GPU granted: %+v; it would fail confined, as a real encode would", access)
		}
	}
	none := prepare.ChooseEncoder(t.Context(), &fakeTrialRunner{worksByEncoder: map[string]bool{}})
	if none.Chosen != prepare.EncoderX264 || !slices.Equal(none.Usable(), []prepare.Encoder{prepare.EncoderX264}) {
		t.Errorf("with no GPU: %+v; want x264 (cut-a §6)", none)
	}
	both := &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_qsv": true, "h264_nvenc": true}}
	if chosen := prepare.ChooseEncoder(t.Context(), both).Chosen; chosen != prepare.EncoderQuickSync {
		t.Errorf("with Quick Sync and NVENC: %s, want Quick Sync first (cut-a §6)", chosen)
	}
}

func TestChooseEncoder_EachTrialSaysWhyItFailedInFFmpegsWords(t *testing.T) {
	t.Parallel()
	choice := prepare.ChooseEncoder(t.Context(), &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_nvenc": true}})
	if len(choice.Trials) != 3 {
		t.Fatalf("trials %+v; want Quick Sync's, NVENC's and VAAPI's (D225 §3, D332)", choice.Trials)
	}
	quickSync, nvenc, vaapi := choice.Trials[0], choice.Trials[1], choice.Trials[2]
	if quickSync.Encoder != prepare.EncoderQuickSync || !strings.Contains(quickSync.Failure, "no such device") {
		t.Errorf("Quick Sync's trial is %+v; want its failure in the words the run gave", quickSync)
	}
	if nvenc.Encoder != prepare.EncoderNVENC || nvenc.Failure != "" {
		t.Errorf("NVENC's trial is %+v; want it worked", nvenc)
	}
	if vaapi.Encoder != prepare.EncoderVAAPI || !strings.Contains(vaapi.Failure, "no such device") {
		t.Errorf("VAAPI's trial is %+v; want its failure in the words the run gave", vaapi)
	}
}

// A node with only an AMD card encodes on it through VAAPI, which comes after Quick Sync and NVENC,
// and before x264 (D332).
func TestChooseEncoder_AnAMDCardEncodesThroughVAAPIAfterQuickSyncAndNVENC(t *testing.T) {
	t.Parallel()
	amd := &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_vaapi": true}}
	choice := prepare.ChooseEncoder(t.Context(), amd)
	if want := []prepare.Encoder{prepare.EncoderVAAPI, prepare.EncoderX264}; choice.Chosen != prepare.EncoderVAAPI ||
		!slices.Equal(choice.Usable(), want) {
		t.Errorf("with an AMD card: %+v; want VAAPI, and x264 usable too", choice)
	}
	for _, access := range amd.accesses {
		if !access.UsesGPU {
			t.Errorf("a trial ran without the GPU granted: %+v", access)
		}
	}
	nvidiaAndAMD := &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_nvenc": true, "h264_vaapi": true}}
	if chosen := prepare.ChooseEncoder(t.Context(), nvidiaAndAMD).Chosen; chosen != prepare.EncoderNVENC {
		t.Errorf("with NVENC and VAAPI: %s, want NVENC first (D332)", chosen)
	}
}

// A VAAPI encode opens its device, uploads the picture to the card, and takes the rate, profile and
// keyframes every encoder does, with no preset or pixel format, which h264_vaapi has none of (D332).
func TestArguments_AVAAPIEncodeUploadsToTheCardWithTheSameRateAndKeyframes(t *testing.T) {
	t.Parallel()
	encode := nodeproto.Prepare{Profile: nodeproto.ProfileH264, SegmentLengthSeconds: 4, VideoKbps: 3000}
	vaapi := prepare.Arguments(execx.Film{Path: "/a"}, "/b", encode, prepare.EncoderVAAPI)
	x264 := prepare.Arguments(execx.Film{Path: "/a"}, "/b", encode, prepare.EncoderX264)
	for _, check := range []struct{ flag, want string }{
		{"-init_hw_device", "vaapi=va"},
		{"-filter_hw_device", "va"},
		{"-filter:v", "format=nv12,hwupload"},
		{"-c:v", "h264_vaapi"},
		{"-profile:v", "high"},
	} {
		if got := after(vaapi, check.flag); got != check.want {
			t.Errorf("a VAAPI encode's %s is %q, want %q", check.flag, got, check.want)
		}
	}
	for _, flag := range []string{"-b:v", "-maxrate", "-bufsize", "-force_key_frames"} {
		if after(vaapi, flag) != after(x264, flag) {
			t.Errorf("a VAAPI encode's %s is %q, x264's %q; want the same", flag, after(vaapi, flag), after(x264, flag))
		}
	}
	if slices.Contains(vaapi, "-preset") || slices.Contains(vaapi, "-pix_fmt") {
		t.Errorf("a VAAPI encode names a preset or pixel format, which h264_vaapi refuses: %q", vaapi)
	}
}

// A chunk's quality on VAAPI scales the picture and then uploads it, with keyframes at the chunk's
// times and nowhere else, as every encoder's (D332, D247 §6).
func TestQualityArguments_OnVAAPIScaleThenUploadWithTheChunksKeyframes(t *testing.T) {
	t.Parallel()
	vaapi := prepare.EncoderVAAPI.QualityArguments(1500, 480, "0,1.5,4")
	x264 := prepare.EncoderX264.QualityArguments(1500, 480, "0,1.5,4")
	if got := after(vaapi, "-vf"); got != "scale=-2:480,format=nv12,hwupload" {
		t.Errorf("a VAAPI quality's filter is %q, want the scale and then the upload", got)
	}
	if after(vaapi, "-init_hw_device") != "vaapi=va" || after(vaapi, "-c:v") != "h264_vaapi" {
		t.Errorf("a VAAPI quality: %q; want its device and h264_vaapi", vaapi)
	}
	for _, flag := range []string{"-b:v", "-maxrate", "-bufsize", "-g", "-force_key_frames", "-profile:v"} {
		if after(vaapi, flag) != after(x264, flag) {
			t.Errorf("a VAAPI quality's %s is %q, x264's %q; want the same", flag, after(vaapi, flag),
				after(x264, flag))
		}
	}
	if slices.Contains(vaapi, "-preset") || slices.Contains(vaapi, "-pix_fmt") {
		t.Errorf("a VAAPI quality names a preset or pixel format: %q", vaapi)
	}
}

func TestArguments_AnEncodeUsesTheNodesEncoderAndARemuxCopies(t *testing.T) {
	t.Parallel()
	encode := nodeproto.Prepare{Profile: nodeproto.ProfileH264}
	arguments := prepare.Arguments(execx.Film{Path: "/a"}, "/b", encode, prepare.EncoderQuickSync)
	if codec := after(arguments, "-c:v"); codec != "h264_qsv" {
		t.Errorf("an encode on a Quick Sync node uses %s, want h264_qsv", codec)
	}
	remux := prepare.Arguments(execx.Film{Path: "/a"}, "/b", nodeproto.Prepare{Profile: nodeproto.ProfileRemux},
		prepare.EncoderQuickSync)
	if codec := after(remux, "-c:v"); codec != "copy" {
		t.Errorf("a remux uses %s, want copy whatever the encoder", codec)
	}
}

// A prepare's encode makes its forced keyframes IDR pictures as a chunk's does, on every encoder: an
// encoder that needs a flag for it gets it in both, or a prepare's segments, which can start only on
// an IDR, come out longer than asked (Quick Sync's lacked it, 2026-10-23).
func TestArguments_APreparesKeyframesAreIDRsAsAChunksAreOnEveryEncoder(t *testing.T) {
	t.Parallel()
	encode := nodeproto.Prepare{Profile: nodeproto.ProfileH264}
	for _, encoder := range []prepare.Encoder{
		prepare.EncoderQuickSync, prepare.EncoderNVENC, prepare.EncoderVAAPI, prepare.EncoderX264,
	} {
		prepared := prepare.Arguments(execx.Film{Path: "/a"}, "/b", encode, encoder)
		chunk := encoder.QualityArguments(1500, 480, "0,4")
		for _, flag := range []string{"-forced_idr", "-forced-idr"} {
			if after(prepared, flag) != after(chunk, flag) {
				t.Errorf("%s: a prepare's %s is %q, a chunk's %q; want the same", encoder, flag,
					after(prepared, flag), after(chunk, flag))
			}
		}
	}
}

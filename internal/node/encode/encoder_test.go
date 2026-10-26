// SPDX-License-Identifier: AGPL-3.0-or-later

package encode_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
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
	choice := encode.ChooseEncoder(t.Context(), nvidia)
	wantUsable := []encode.Encoder{encode.EncoderNVENC, encode.EncoderX264}
	if choice.Chosen != encode.EncoderNVENC || !slices.Equal(choice.Usable(), wantUsable) {
		t.Errorf("with NVENC only: %+v; want NVENC, and x264 usable too", choice)
	}
	for _, access := range nvidia.accesses {
		if !access.UsesGPU {
			t.Errorf("a trial ran without the GPU granted: %+v; it would fail confined, as a real encode would", access)
		}
	}
	none := encode.ChooseEncoder(t.Context(), &fakeTrialRunner{worksByEncoder: map[string]bool{}})
	if none.Chosen != encode.EncoderX264 || !slices.Equal(none.Usable(), []encode.Encoder{encode.EncoderX264}) {
		t.Errorf("with no GPU: %+v; want x264 (cut-a §6)", none)
	}
	both := &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_qsv": true, "h264_nvenc": true}}
	if chosen := encode.ChooseEncoder(t.Context(), both).Chosen; chosen != encode.EncoderQuickSync {
		t.Errorf("with Quick Sync and NVENC: %s, want Quick Sync first (cut-a §6)", chosen)
	}
}

func TestChooseEncoder_EachTrialSaysWhyItFailedInFFmpegsWords(t *testing.T) {
	t.Parallel()
	choice := encode.ChooseEncoder(t.Context(), &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_nvenc": true}})
	if len(choice.Trials) != 3 {
		t.Fatalf("trials %+v; want Quick Sync's, NVENC's and VAAPI's (D225 §3, D332)", choice.Trials)
	}
	quickSync, nvenc, vaapi := choice.Trials[0], choice.Trials[1], choice.Trials[2]
	if quickSync.Encoder != encode.EncoderQuickSync || !strings.Contains(quickSync.Failure, "no such device") {
		t.Errorf("Quick Sync's trial is %+v; want its failure in the words the run gave", quickSync)
	}
	if nvenc.Encoder != encode.EncoderNVENC || nvenc.Failure != "" {
		t.Errorf("NVENC's trial is %+v; want it worked", nvenc)
	}
	if vaapi.Encoder != encode.EncoderVAAPI || !strings.Contains(vaapi.Failure, "no such device") {
		t.Errorf("VAAPI's trial is %+v; want its failure in the words the run gave", vaapi)
	}
}

// A node with only an AMD card encodes on it through VAAPI, which comes after Quick Sync and NVENC,
// and before x264 (D332).
func TestChooseEncoder_AnAMDCardEncodesThroughVAAPIAfterQuickSyncAndNVENC(t *testing.T) {
	t.Parallel()
	amd := &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_vaapi": true}}
	choice := encode.ChooseEncoder(t.Context(), amd)
	if want := []encode.Encoder{encode.EncoderVAAPI, encode.EncoderX264}; choice.Chosen != encode.EncoderVAAPI ||
		!slices.Equal(choice.Usable(), want) {
		t.Errorf("with an AMD card: %+v; want VAAPI, and x264 usable too", choice)
	}
	for _, access := range amd.accesses {
		if !access.UsesGPU {
			t.Errorf("a trial ran without the GPU granted: %+v", access)
		}
	}
	nvidiaAndAMD := &fakeTrialRunner{worksByEncoder: map[string]bool{"h264_nvenc": true, "h264_vaapi": true}}
	if chosen := encode.ChooseEncoder(t.Context(), nvidiaAndAMD).Chosen; chosen != encode.EncoderNVENC {
		t.Errorf("with NVENC and VAAPI: %s, want NVENC first (D332)", chosen)
	}
}

// A chunk's quality on VAAPI scales the picture and then uploads it, with keyframes at the chunk's
// times and nowhere else, as every encoder's (D332, D247 §6).
func TestQualityArguments_OnVAAPIScaleThenUploadWithTheChunksKeyframes(t *testing.T) {
	t.Parallel()
	vaapi := encode.EncoderVAAPI.QualityArguments(1500, 480, "0,1.5,4")
	x264 := encode.EncoderX264.QualityArguments(1500, 480, "0,1.5,4")
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

// A chunk's forced keyframes are IDR pictures on every encoder that needs a flag for it, as a chunk
// can start only on one (Quick Sync's lacked it, 2026-10-23).
func TestQualityArguments_ForcedKeyframesAreIDRsOnQuickSyncAndNVENC(t *testing.T) {
	t.Parallel()
	if got := after(encode.EncoderQuickSync.QualityArguments(1500, 480, "0,4"), "-forced_idr"); got != "1" {
		t.Errorf("Quick Sync's -forced_idr is %q, want 1", got)
	}
	if got := after(encode.EncoderNVENC.QualityArguments(1500, 480, "0,4"), "-forced-idr"); got != "1" {
		t.Errorf("NVENC's -forced-idr is %q, want 1", got)
	}
}

// after is what follows flag in arguments, or "" when flag is absent.
func after(arguments []string, flag string) string {
	if at := slices.Index(arguments, flag); at >= 0 && at+1 < len(arguments) {
		return arguments[at+1]
	}
	return ""
}

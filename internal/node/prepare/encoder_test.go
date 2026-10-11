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
	if len(choice.Trials) != 2 {
		t.Fatalf("trials %+v; want Quick Sync's and NVENC's (D225 §3)", choice.Trials)
	}
	quickSync, nvenc := choice.Trials[0], choice.Trials[1]
	if quickSync.Encoder != prepare.EncoderQuickSync || !strings.Contains(quickSync.Failure, "no such device") {
		t.Errorf("Quick Sync's trial is %+v; want its failure in the words the run gave", quickSync)
	}
	if nvenc.Encoder != prepare.EncoderNVENC || nvenc.Failure != "" {
		t.Errorf("NVENC's trial is %+v; want it worked", nvenc)
	}
}

func TestArguments_AnEncodeUsesTheNodesEncoderAndARemuxCopies(t *testing.T) {
	t.Parallel()
	encode := nodeproto.Prepare{Profile: nodeproto.ProfileH264}
	arguments := prepare.Arguments("/a", "/b", encode, prepare.EncoderQuickSync)
	if codec := after(arguments, "-c:v"); codec != "h264_qsv" {
		t.Errorf("an encode on a Quick Sync node uses %s, want h264_qsv", codec)
	}
	remux := prepare.Arguments("/a", "/b", nodeproto.Prepare{Profile: nodeproto.ProfileRemux}, prepare.EncoderQuickSync)
	if codec := after(remux, "-c:v"); codec != "copy" {
		t.Errorf("a remux uses %s, want copy whatever the encoder", codec)
	}
}

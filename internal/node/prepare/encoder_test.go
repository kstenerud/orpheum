// SPDX-License-Identifier: GPL-3.0-or-later

package prepare_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// trials is a machine whose encoders work or not, as works says, recording each trial's access.
type trials struct {
	works    map[string]bool
	accesses []confine.Access
}

func (machine *trials) RunStreaming(
	_ context.Context, _ execx.Tool, access confine.Access, arguments []string, _ io.Writer,
) error {
	machine.accesses = append(machine.accesses, access)
	if machine.works[after(arguments, "-c:v")] {
		return nil
	}
	return errors.New("no such device")
}

func TestChooseEncoder_TakesTheFirstHardwareEncoderThatWorksConfinedOrX264(t *testing.T) {
	t.Parallel()
	nvidia := &trials{works: map[string]bool{"h264_nvenc": true}}
	chosen, usable := prepare.ChooseEncoder(t.Context(), nvidia)
	wantUsable := []prepare.Encoder{prepare.EncoderNVENC, prepare.EncoderX264}
	if chosen != prepare.EncoderNVENC || !slices.Equal(usable, wantUsable) {
		t.Errorf("with NVENC only: %s, %v; want NVENC, and x264 usable too", chosen, usable)
	}
	for _, access := range nvidia.accesses {
		if !access.Devices {
			t.Errorf("a trial ran without the GPU granted: %+v; it would fail confined, as a real encode would", access)
		}
	}
	chosen, usable = prepare.ChooseEncoder(t.Context(), &trials{works: map[string]bool{}})
	if chosen != prepare.EncoderX264 || !slices.Equal(usable, []prepare.Encoder{prepare.EncoderX264}) {
		t.Errorf("with no GPU: %s, %v; want x264 (cut-a §6)", chosen, usable)
	}
	both := &trials{works: map[string]bool{"h264_qsv": true, "h264_nvenc": true}}
	if chosen, _ = prepare.ChooseEncoder(t.Context(), both); chosen != prepare.EncoderQuickSync {
		t.Errorf("with Quick Sync and NVENC: %s, want Quick Sync first (cut-a §6)", chosen)
	}
}

func TestArguments_AnEncodeUsesTheNodesEncoderAndARemuxCopies(t *testing.T) {
	t.Parallel()
	encode := nodeproto.Prepare{Profile: nodeproto.ProfileH264At8M}
	arguments := prepare.Arguments("/a", "/b", encode, prepare.EncoderQuickSync)
	if codec := after(arguments, "-c:v"); codec != "h264_qsv" {
		t.Errorf("an encode on a Quick Sync node uses %s, want h264_qsv", codec)
	}
	remux := prepare.Arguments("/a", "/b", nodeproto.Prepare{Profile: nodeproto.ProfileRemux}, prepare.EncoderQuickSync)
	if codec := after(remux, "-c:v"); codec != "copy" {
		t.Errorf("a remux uses %s, want copy whatever the encoder", codec)
	}
}

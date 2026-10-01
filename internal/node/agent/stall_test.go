// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// errStalled is an FFmpeg the watchdog stopped: it stood at 4 s for three minutes.
var errStalled = &execx.StalledError{
	Tool: execx.FFmpeg, Cause: execx.StallStoodStill, LastPosition: 4 * time.Second,
	WithoutAdvance: 3 * time.Minute, StderrTail: "frame=100",
}

// stallingStreamIndex is the stream index whose measurement or extraction stalls.
const stallingStreamIndex = 97

// wantStall fails the test unless stall is errStalled as the core is told it.
func wantStall(t *testing.T, what string, stall *nodeproto.Stall) {
	t.Helper()
	if stall == nil || stall.Cause != nodeproto.StallStoodStill || stall.LastPositionMs != 4000 ||
		stall.WithoutAdvanceMs != 180_000 || !strings.Contains(stall.Detail, "stalled") {
		t.Errorf("%s told the core %+v; want stood still at 4000 ms for 180000 ms", what, stall)
	}
}

func TestAgent_AStalledPrepareIsAnsweredStalledNotFailedAndFreesItsSlot(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	source := preparableFilm(t, root)
	// More stalls than a remux has slots: each frees its slot, or the last prepare never runs.
	for job := range int64(testCapacity.RemuxSlotCount + 1) {
		core.sendPrepare(job+1, source, "3/remux/stalled")
		stalled := finalPrepareReport(t, core)
		if stalled.State != nodeproto.PrepareStalled || stalled.Failure != "" || stalled.Refusal != "" {
			t.Fatalf("a stalled prepare ended %+v; want stalled, neither failed nor refused", stalled)
		}
		wantStall(t, "a stalled prepare", stalled.Stall)
	}
	core.sendPrepare(10, source, "3/remux/0123456789abcdef")
	if done := finalPrepareReport(t, core); done.State != nodeproto.PrepareDone {
		t.Errorf("a prepare after the stalls ended %+v; want done, in a slot the stalls freed", done)
	}
}

func TestAgent_AStalledMeasurementOrExtractionIsAnsweredStalledNotFailed(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	command := nodeproto.MeasureLoudness{
		Video: video, Tracks: []nodeproto.LoudnessTrack{{StreamIndex: stallingStreamIndex, ChannelCount: 2}},
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 1, Type: nodeproto.CommandMeasureLoudness, JobID: 1, MeasureLoudness: &command,
	}
	measured := receiveLoudness(t, core)
	if measured.Failure != "" || measured.Refusal != "" {
		t.Errorf("a stalled measurement answered %+v; want a stall, not the file's failure", measured)
	}
	wantStall(t, "a stalled measurement", measured.Stall)

	core.queueExtraction(2, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: stallingStreamIndex})
	extracted := receiveSubtitle(t, core)
	if extracted.Failure != "" || extracted.Refusal != "" {
		t.Errorf("a stalled extraction answered %+v; want a stall, not the file's failure", extracted)
	}
	wantStall(t, "a stalled extraction", extracted.Stall)
}

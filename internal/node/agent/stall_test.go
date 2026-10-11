// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// errStalled is an FFmpeg the watchdog stopped: it stood at 4 s for three minutes, waiting on the
// disk, and outlived its kill.
var errStalled = &execx.StalledError{
	Tool: execx.FFmpeg, Cause: execx.StallStoodStill, Condition: execx.ConditionWaitingOnDisk, IsLeftRunning: true,
	LastPosition: 4 * time.Second, WithoutAdvance: 3 * time.Minute, StderrTail: "frame=100",
}

// stallingStreamIndex is the stream index whose measurement or extraction stalls.
const stallingStreamIndex = 97

// wantStall fails the test unless stall is errStalled as the core is told it.
func wantStall(t *testing.T, what string, stall *nodeproto.Stall) {
	t.Helper()
	if stall == nil || stall.Cause != nodeproto.StallStoodStill ||
		stall.Condition != nodeproto.ConditionWaitingOnDisk || !stall.IsLeftRunning || stall.LastPositionMs != 4000 ||
		stall.WithoutAdvanceMs != 180_000 || !strings.Contains(stall.Detail, "stalled") {
		t.Errorf("%s told the core %+v; want stood still at 4000 ms for 180000 ms, waiting on the disk and left "+
			"running", what, stall)
	}
}

// The status lists the processes the runner left running, waiting on a device (D233 §10).
func TestStatus_ListsTheProcessesLeftRunning(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	left := []execx.StuckProcess{{PID: 42, Tool: execx.FFprobe, Reading: []string{"/media/films/a.mkv"}, Since: since}}
	node := New("/nowhere", nodeproto.Hello{Name: "local"}, nil,
		Workers{Stuck: func() []nodeproto.Stuck { return StuckOf(left) }},
		clocktest.NewFake(since), slog.New(slog.DiscardHandler))
	stuck := node.status().Stuck
	want := nodeproto.Stuck{Tool: "ffprobe", PID: 42, Paths: []string{"/media/films/a.mkv"}, Since: since.UnixMilli()}
	if len(stuck) != 1 || stuck[0].Tool != want.Tool || stuck[0].PID != want.PID || stuck[0].Since != want.Since ||
		len(stuck[0].Paths) != 1 || stuck[0].Paths[0] != want.Paths[0] {
		t.Errorf("the status's stuck = %+v, want %+v", stuck, want)
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

func TestAgent_AStalledReadThroughOrExtractionIsAnsweredStalledNotFailed(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	core.sendReadThrough(1, nodeproto.ReadThrough{
		Video: video, LoudnessTracks: []nodeproto.LoudnessTrack{{StreamIndex: stallingStreamIndex, ChannelCount: 2}},
	})
	measured := receiveReadThrough(t, core)
	if measured.Failure != "" || measured.Refusal != "" {
		t.Errorf("a stalled read-through answered %+v; want a stall, not the file's failure", measured)
	}
	wantStall(t, "a stalled read-through", measured.Stall)

	core.queueExtraction(2, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: stallingStreamIndex})
	extracted := receiveSubtitle(t, core)
	if extracted.Failure != "" || extracted.Refusal != "" {
		t.Errorf("a stalled extraction answered %+v; want a stall, not the file's failure", extracted)
	}
	wantStall(t, "a stalled extraction", extracted.Stall)
}

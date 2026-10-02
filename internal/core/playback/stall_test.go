// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"crypto/rand"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// preparesOnAFakeNode is a prepare runner whose node is node, whose clock is clockSource, and
// which never gives up for silence within a test.
func preparesOnAFakeNode(t *testing.T, node fakeNodes, clockSource *clocktest.Fake) *prepares {
	t.Helper()
	all := &prepares{
		cache: storetest.OpenCache(t), queue: &progressQueue{progress: map[int64]float64{}}, nodes: node,
		// The silence limit outlasts the slot cap, which these tests move the clock past at once.
		clock: clockSource, maxSilence: 2 * maxSlotWait, maxSlotWait: maxSlotWait,
		exchanges: nodes.NewExchanges(node, clockSource, rand.Reader, slog.New(slog.DiscardHandler)),
		logger:    slog.New(slog.DiscardHandler),
	}
	all.exchanges.Record(nodeproto.CommandPrepare, all.recordPrepare)
	return all
}

// insertRendition records a rendition being made, and returns its id.
func insertRendition(t *testing.T, all *prepares) int64 {
	t.Helper()
	var rendition int64
	err := all.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		var insertErr error
		rendition, insertErr = queries.InsertRendition(t.Context(), cachegen.InsertRenditionParams{
			SourceFile: 3, SourcePayloadHash: []byte{1}, Profile: "remux", NodeID: 1, SegmentLengthSeconds: 2,
			RunID: "0123456789abcdef", CreatedAt: 1,
		})
		return insertErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return rendition
}

func TestHandle_AStalledPrepareIsTriedAgainAndTwoInARowAreTheRenditionsFailure(t *testing.T) {
	t.Parallel()
	node := fakeNodes{sent: make(chan nodeproto.Command, 1)}
	all := preparesOnAFakeNode(t, node, clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)))
	rendition := insertRendition(t, all)
	stall := &nodeproto.Stall{Cause: nodeproto.StallStoodStill, LastPositionMs: 4000, WithoutAdvanceMs: 180_000}
	prepareAfter := func(previousAttemptStalled bool) error {
		job := jobs.Job{
			ID: 7, Target: jobs.Target{RenditionID: rendition}, PreviousAttemptToolFailed: previousAttemptStalled,
		}
		ended := make(chan error, 1)
		go func() { ended <- all.handle(t.Context(), job, testOrder) }()
		command := testutil.Receive(t, node.sent, "the prepare command")
		all.reported(t, command, nodeproto.PrepareReport{State: nodeproto.PrepareStalled, Stall: stall})
		return testutil.Receive(t, ended, "the prepare job to end")
	}
	if err := prepareAfter(false); err == nil || !strings.HasPrefix(err.Error(), "stalled: ") ||
		!strings.Contains(err.Error(), "stood_still at 4000 ms") {
		t.Errorf("a first stall ended %v; want it tried again as a stall, saying what the node saw", err)
	}
	if row := renditionRow(t, all, rendition); row.State == renditionFailed {
		t.Errorf("after one stall the rendition is %+v; want it not failed, the hang not its source's fault", row)
	}
	if err := prepareAfter(true); err == nil || !strings.Contains(err.Error(), stalledTwice) {
		t.Errorf("a second stall in a row ended %v; want the rendition's failure", err)
	}
	if row := renditionRow(t, all, rendition); row.State != renditionFailed || row.Failure == nil ||
		!strings.HasPrefix(*row.Failure, stalledTwice) {
		t.Errorf("after two stalls the rendition is %+v; want failed, saying FFmpeg stalled twice", row)
	}
}

// A node that started again lost the prepare it held (D238 §3): the tool's failure, as a stall
// is, tried again at no cost the first time and the rendition's failure on the second in a row.
func TestHandle_APrepareItsNodeLostByStartingAgainIsTriedAgainAndTwoInARowAreTheRenditionsFailure(t *testing.T) {
	t.Parallel()
	node := fakeNodes{sent: make(chan nodeproto.Command, 1)}
	all := preparesOnAFakeNode(t, node, clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)))
	rendition := insertRendition(t, all)
	prepareAfter := func(previousAttemptToolFailed bool) error {
		job := jobs.Job{
			ID: 7, Target: jobs.Target{RenditionID: rendition}, PreviousAttemptToolFailed: previousAttemptToolFailed,
		}
		ended := make(chan error, 1)
		go func() { ended <- all.handle(t.Context(), job, testOrder) }()
		testutil.Receive(t, node.sent, "the prepare command")
		all.exchanges.Interrupt(t.Context(), 1)
		return testutil.Receive(t, ended, "the prepare job to end")
	}
	if err := prepareAfter(false); err == nil || !strings.HasPrefix(err.Error(), "interrupted: ") {
		t.Errorf("a first restart of the node ended %v; want it tried again as the tool's failure", err)
	}
	if row := renditionRow(t, all, rendition); row.State == renditionFailed {
		t.Errorf("after one restart the rendition is %+v; want it not failed", row)
	}
	if err := prepareAfter(true); err == nil || !strings.Contains(err.Error(), jobs.ToolFailedTwice) {
		t.Errorf("a second tool failure in a row ended %v; want the rendition's failure", err)
	}
	if row := renditionRow(t, all, rendition); row.State != renditionFailed || row.Failure == nil ||
		!strings.HasPrefix(*row.Failure, jobs.ToolFailedTwice) {
		t.Errorf("after two tool failures the rendition is %+v; want failed, saying so", row)
	}
}

func TestPrepareCommand_CarriesTheSourcesLengthForTheNodesDeadline(t *testing.T) {
	t.Parallel()
	probe := nodeproto.Probe{Format: nodeproto.ProbeFormat{DurationMs: 7_200_000}}
	command := prepareCommand(nodeproto.BytesFile{}, 3, nodeproto.ProfileRemux, "0123456789abcdef", probe)
	if command.SourceDurationMs != 7_200_000 {
		t.Errorf("the prepare says the source lasts %d ms, want the probe's 7200000", command.SourceDurationMs)
	}
}

func renditionRow(t *testing.T, all *prepares, rendition int64) cachegen.PreparedRendition {
	t.Helper()
	row, err := all.cache.Read().GetRenditionByID(t.Context(), rendition)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestHandle_APrepareHeldWaitingForASlotTooLongIsGivenUpAndTheNodeToldToStopIt(t *testing.T) {
	t.Parallel()
	node := fakeNodes{sent: make(chan nodeproto.Command, 2)}
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	all := preparesOnAFakeNode(t, node, clockSource)
	job := jobs.Job{ID: 7, Target: jobs.Target{RenditionID: insertRendition(t, all)}}
	ended := make(chan error, 1)
	go func() { ended <- all.handle(t.Context(), job, testOrder) }()
	command := testutil.Receive(t, node.sent, "the prepare command")
	// The node says it is waiting at each tick; the clock moves on by the cap between them.
	for range 10 {
		all.progressed(t.Context(), command, nodeproto.ProgressWaiting)
		select {
		case err := <-ended:
			if err == nil || !strings.Contains(err.Error(), "waiting for a slot") ||
				strings.HasPrefix(err.Error(), "stalled: ") {
				t.Errorf("a prepare held waiting past the cap ended %v; want given up as an ordinary retry", err)
			}
			cancel := testutil.Receive(t, node.sent, "the cancel of job 7")
			if cancel.Type != nodeproto.CommandCancel || cancel.JobID != 7 {
				t.Errorf("the node was sent %+v; want it told to stop job 7", cancel)
			}
			return
		case <-time.After(20 * time.Millisecond):
		}
		clockSource.Advance(maxSlotWait)
	}
	t.Fatal("a prepare the node held waiting for ten times the cap was never given up")
}

// A node's word that a making is done is recorded by its run as it arrives, with no job waiting for
// it any more (S81), and nothing heard after undoes it; a making the rendition no longer is, made
// again since under another run, is not recorded over it (G13).
func TestRecordPrepare_RecordsAMakingByItsRunWhetherOrNotItsJobWaits(t *testing.T) {
	t.Parallel()
	node := fakeNodes{sent: make(chan nodeproto.Command, 1)}
	all := preparesOnAFakeNode(t, node, clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)))
	rendition := insertRendition(t, all)
	doneBy := func(run renditionRun) {
		t.Helper()
		command := nodeproto.Command{
			JobID:        7,
			AttemptToken: "an attempt no job waits for",
			Work: nodeproto.Work{
				Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{RenditionDir: run.dir()},
			},
		}
		all.reported(t, command, nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 10})
	}
	earlier := renditionRun{fileID: testRun.fileID, profile: testRun.profile, runID: "fedcba9876543210"}
	doneBy(earlier)
	if row := renditionRow(t, all, rendition); row.State == "complete" {
		t.Errorf("an earlier making's end completed the rendition: %+v; want it left to its own run", row)
	}
	doneBy(testRun)
	if row := renditionRow(t, all, rendition); row.State != "complete" || row.SizeBytes == nil || *row.SizeBytes != 10 {
		t.Errorf("the making's end, with no job waiting, left the rendition %+v; want it complete, 10 bytes", row)
	}
	// Progress heard after the end, and a job that gave up on the making, do not undo it.
	if err := all.recordRunning(t.Context(), testRun, 5000); err != nil {
		t.Fatal(err)
	}
	if err := all.failMaking(t.Context(), testRun, "cancelled"); err != nil {
		t.Fatal(err)
	}
	if row := renditionRow(t, all, rendition); row.State != "complete" {
		t.Errorf("after late progress and a job giving up, the rendition is %+v; want it still complete", row)
	}
}

// A refusal says nothing of the source, and is tried again; one for a file changed since the scan
// says so, and that the next scan of its root ends the job (S74).
func TestRefusalError_SaysAFileChangedSinceTheScanApart(t *testing.T) {
	t.Parallel()
	changed := refusalError(1, "prepare 3/remux/0123456789abcdef", "not the one named", nodeproto.RefusedChanged)
	if !strings.Contains(changed.Error(), "changed since the scan") || !strings.Contains(changed.Error(), "next scan") {
		t.Errorf("a changed file's refusal says %q, want it said apart, with what ends the job", changed)
	}
	other := refusalError(1, "prepare 3/remux/0123456789abcdef", "this node does not prepare", "")
	if strings.Contains(other.Error(), "changed since the scan") {
		t.Errorf("another refusal says %q, want it not taken for a changed file", other)
	}
}

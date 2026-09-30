// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
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
)

// preparesOnAFakeNode is a prepare runner whose node is node, whose clock is clockSource, and
// which never gives up for silence within a test.
func preparesOnAFakeNode(t *testing.T, node fakeNodes, clockSource *clocktest.Fake) *prepares {
	t.Helper()
	return &prepares{
		cache: storetest.OpenCache(t), queue: &progressQueue{progress: map[int64]float64{}}, nodes: node,
		clock: clockSource, maxSilence: time.Hour, maxSlotWait: maxSlotWait,
		reportAnswers: nodes.NewAnswerRouter[nodeproto.PrepareReport](slog.New(slog.DiscardHandler),
			"a prepare report"),
	}
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
		go func() { ended <- all.handle(t.Context(), job, prepareOrder{nodeID: 1}) }()
		<-node.sent
		all.reported(t.Context(), 1, nodeproto.PrepareReport{JobID: 7, State: nodeproto.PrepareStalled, Stall: stall})
		return <-ended
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
	go func() { ended <- all.handle(t.Context(), job, prepareOrder{nodeID: 1}) }()
	<-node.sent
	// The node says it is waiting at each tick; the clock moves on by the cap between them.
	for range 10 {
		all.reported(t.Context(), 1, nodeproto.PrepareReport{JobID: 7, State: nodeproto.PrepareWaiting})
		select {
		case err := <-ended:
			if err == nil || !strings.Contains(err.Error(), "waiting for a slot") ||
				strings.HasPrefix(err.Error(), "stalled: ") {
				t.Errorf("a prepare held waiting past the cap ended %v; want given up as an ordinary retry", err)
			}
			if cancel := <-node.sent; cancel.Type != nodeproto.CommandCancel || cancel.Cancel.JobID != 7 {
				t.Errorf("the node was sent %+v; want it told to stop job 7", cancel)
			}
			return
		case <-time.After(20 * time.Millisecond):
		}
		clockSource.Advance(maxSlotWait)
	}
	t.Fatal("a prepare the node held waiting for ten times the cap was never given up")
}

// SPDX-License-Identifier: GPL-3.0-or-later

package renditions

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/nodes/nodestest"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// preparesOnAFakeNode is a prepare runner whose node is node, whose clock is clockSource, and
// which gives up for silence after maxSilence; and start, which runs a prepare of testOrder and
// returns where the answer its node's word resolves it with arrives.
func preparesOnAFakeNode(
	t *testing.T, node fakeNodes, clockSource *clocktest.Fake, maxSilence time.Duration,
) (service *Service, start func(job jobs.Job) <-chan error) {
	t.Helper()
	cache := storetest.OpenCache(t)
	service = &Service{
		cache: cache, queue: &progressQueue{progress: map[int64]float64{}}, nodes: node,
		clock: clockSource,
		exchanges: nodes.NewExchanges(nodes.ExchangeParts{
			Sender: node, Rooms: node, Clock: clockSource, Random: rand.Reader, Logger: slog.New(slog.DiscardHandler),
			Cache: cache, Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)), MaxSilence: maxSilence,
		}),
		logger: slog.New(slog.DiscardHandler),
	}
	service.exchanges.Record(nodeproto.CommandPrepare, service.recordPrepare)
	resolver := nodestest.NewResolver()
	service.exchanges.Handle(jobs.KindPrepare, service.Assignee(resolver.Of(nil)))
	return service, func(job jobs.Job) <-chan error {
		job.Kind = jobs.KindPrepare
		return resolver.Start(t, func(ctx context.Context, job jobs.Job) error {
			return service.handle(ctx, job, testOrder)
		}, job)
	}
}

// neverSilent is a silence limit no test reaches.
const neverSilent = 12 * time.Hour

// insertRendition records a rendition being made, and returns its id.
func insertRendition(t *testing.T, service *Service) int64 {
	t.Helper()
	var rendition int64
	err := service.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
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
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	service, start := preparesOnAFakeNode(t, node, clockSource, neverSilent)
	rendition := insertRendition(t, service)
	stall := &nodeproto.Stall{
		Cause: nodeproto.StallStoodStill, Condition: nodeproto.ConditionSpinning, LastPositionMs: 4000,
		WithoutAdvanceMs: 180_000,
	}
	prepareAfter := func(previousAttemptStalled bool) error {
		job := jobs.Job{
			ID: 7, Target: jobs.Target{RenditionID: rendition}, PreviousAttemptToolFailed: previousAttemptStalled,
		}
		ended := start(job)
		command := testutil.Receive(t, node.sent, "the prepare command")
		service.reported(t, command, nodeproto.PrepareReport{State: nodeproto.PrepareStalled, Stall: stall})
		return testutil.Receive(t, ended, "the prepare job to end")
	}
	if err := prepareAfter(false); err == nil || !strings.HasPrefix(err.Error(), "stalled: ") ||
		!strings.Contains(err.Error(), "stood_still at 4000 ms") || !strings.Contains(err.Error(), "spinning") {
		t.Errorf("a first stall ended %v; want it tried again as a stall, saying what the node saw", err)
	}
	if row := renditionRow(t, service, rendition); row.State == renditionFailed {
		t.Errorf("after one stall the rendition is %+v; want it not failed, the hang not its source's fault", row)
	}
	if err := prepareAfter(true); err == nil || !strings.Contains(err.Error(), nodes.FFmpegStalledTwice) {
		t.Errorf("a second stall in a row ended %v; want the rendition's failure", err)
	}
	if row := renditionRow(t, service, rendition); row.State != renditionFailed || row.Failure == nil ||
		!strings.HasPrefix(*row.Failure, nodes.FFmpegStalledTwice) {
		t.Errorf("after two stalls the rendition is %+v; want failed, saying FFmpeg stalled twice", row)
	}
}

// A node that started again lost the prepare it held: an attempt like any other's failure, tried
// again, and never the rendition's, however many in a row (D248).
func TestHandle_APrepareItsNodeLostByStartingAgainIsTriedAgainAndNeverTheRenditionsFailure(t *testing.T) {
	t.Parallel()
	node := fakeNodes{sent: make(chan nodeproto.Command, 1)}
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	service, start := preparesOnAFakeNode(t, node, clockSource, neverSilent)
	rendition := insertRendition(t, service)
	prepareAfter := func(previousAttemptToolFailed bool) error {
		job := jobs.Job{
			ID: 7, Target: jobs.Target{RenditionID: rendition}, PreviousAttemptToolFailed: previousAttemptToolFailed,
		}
		ended := start(job)
		testutil.Receive(t, node.sent, "the prepare command")
		service.exchanges.Interrupt(t.Context(), 1, "started again")
		return testutil.Receive(t, ended, "the prepare job to end")
	}
	for _, previousAttemptToolFailed := range []bool{false, true} {
		if err := prepareAfter(previousAttemptToolFailed); !errors.Is(err, nodes.ErrNodeRestarted) {
			t.Errorf("a restart of the node ended %v; want it tried again, the node's work lost", err)
		}
		if row := renditionRow(t, service, rendition); row.State == renditionFailed {
			t.Errorf("after a restart the rendition is %+v; want it never failed for work its node lost", row)
		}
	}
}

// A prepare its node's status no longer lists was lost otherwise than by a restart: that too is an
// attempt like any other's failure, tried again (D248).
func TestHandle_APrepareTheNodesStatusNoLongerHoldsIsTriedAgainAsTheToolsFailure(t *testing.T) {
	t.Parallel()
	node := fakeNodes{sent: make(chan nodeproto.Command, 1)}
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	service, start := preparesOnAFakeNode(t, node, clockSource, neverSilent)
	ended := start(jobs.Job{ID: 7, Target: jobs.Target{RenditionID: insertRendition(t, service)}})
	testutil.Receive(t, node.sent, "the prepare command")
	service.exchanges.Reconcile(t.Context(), 1, nil, nil)
	err := testutil.Receive(t, ended, "the prepare job to end")
	if !errors.Is(err, nodes.ErrWorkLost) || errors.Is(err, nodes.ErrNodeRestarted) {
		t.Errorf("a prepare the node's status no longer holds ended %v; want it tried again, the work lost", err)
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

// An encode is told its picture's rate, by the source's height; a remux, which copies the picture,
// none (D256).
func TestPrepareCommand_AnEncodeCarriesTheRateForItsSourcesHeight(t *testing.T) {
	t.Parallel()
	probe := nodeproto.Probe{Streams: []nodeproto.ProbeStream{
		{Kind: "video", StreamIndex: 0, Codec: "mpeg4", Video: nodeproto.ProbeVideo{Width: 640, Height: 272}},
	}}
	encode := prepareCommand(nodeproto.BytesFile{}, 3, nodeproto.ProfileH264, "0123456789abcdef", probe)
	if encode.VideoKbps != 1000 {
		t.Errorf("an encode of a 640×272 picture is told %d kb/s, want 1000", encode.VideoKbps)
	}
	remux := prepareCommand(nodeproto.BytesFile{}, 3, nodeproto.ProfileRemux, "0123456789abcdef", probe)
	if remux.VideoKbps != 0 {
		t.Errorf("a remux is told %d kb/s, want none: it copies the picture", remux.VideoKbps)
	}
}

func renditionRow(t *testing.T, service *Service, rendition int64) cachegen.PreparedRendition {
	t.Helper()
	row, err := service.cache.Read().GetRenditionByID(t.Context(), rendition)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

// A prepare that waits, for its node or for room on it, has not begun its making: its rendition is
// recorded running only once the node is given it.
func TestHandle_APrepareThatWaitsHasNotBegunItsMaking(t *testing.T) {
	t.Parallel()
	away := jobs.Park(jobs.Parking{Destination: jobs.NodeDestination(1), Reason: "not connected"},
		errors.New("node 1 is not connected"))
	node := fakeNodes{sent: make(chan nodeproto.Command, 1), notReady: away}
	clockSource := clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	service, _ := preparesOnAFakeNode(t, node, clockSource, neverSilent)
	rendition := insertRendition(t, service)
	answer := service.handle(t.Context(), jobs.Job{ID: 7, Target: jobs.Target{RenditionID: rendition}}, testOrder)
	if _, isParked := jobqueue.ParkingOf(answer); !isParked {
		t.Fatalf("a prepare for a node away was answered %v, want it parked", answer)
	}
	if row := renditionRow(t, service, rendition); row.State != renditionQueued {
		t.Errorf("the rendition of a prepare that waits is %s, want still queued", row.State)
	}
}

// A node's word that a making is done is recorded by its run as it arrives, with no job waiting for
// it any more (S81), and nothing heard after undoes it; a making the rendition no longer is, made
// again since under another run, is not recorded over it (G13).
func TestRecordPrepare_RecordsAMakingByItsRunWhetherOrNotItsJobWaits(t *testing.T) {
	t.Parallel()
	node := fakeNodes{sent: make(chan nodeproto.Command, 1)}
	clockSource := clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	service, _ := preparesOnAFakeNode(t, node, clockSource, neverSilent)
	rendition := insertRendition(t, service)
	doneBy := func(run renditionRun) {
		t.Helper()
		command := nodeproto.Command{
			JobID:        7,
			AttemptToken: "an attempt no job waits for",
			Work: nodeproto.Work{
				Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{RenditionDir: run.dir()},
			},
		}
		service.reported(t, command, nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 10})
	}
	earlier := renditionRun{fileID: testRun.fileID, profile: testRun.profile, runID: "fedcba9876543210"}
	doneBy(earlier)
	if row := renditionRow(t, service, rendition); row.State == "complete" {
		t.Errorf("an earlier making's end completed the rendition: %+v; want it left to its own run", row)
	}
	doneBy(testRun)
	row := renditionRow(t, service, rendition)
	if row.State != "complete" || row.SizeBytes == nil || *row.SizeBytes != 10 {
		t.Errorf("the making's end, with no job waiting, left the rendition %+v; want it complete, 10 bytes", row)
	}
	// Progress heard after the end, and a job that gave up on the making, do not undo it.
	if err := service.recordRunning(t.Context(), testRun, 5000); err != nil {
		t.Fatal(err)
	}
	if err := service.failMaking(t.Context(), testRun, "cancelled"); err != nil {
		t.Fatal(err)
	}
	if row := renditionRow(t, service, rendition); row.State != "complete" {
		t.Errorf("after late progress and a job giving up, the rendition is %+v; want it still complete", row)
	}
}

// A refusal says nothing of the source, and is tried again; one for a file changed since the scan
// says so, and that the next scan of its root ends the job (S74).
func TestRefusalError_SaysAFileChangedSinceTheScanApart(t *testing.T) {
	t.Parallel()
	changed := nodes.RefusalError(1, "prepare 3/remux/0123456789abcdef", "not the one named", nodeproto.RefusedChanged)
	if !strings.Contains(changed.Error(), "changed since the scan") || !strings.Contains(changed.Error(), "next scan") {
		t.Errorf("a changed file's refusal says %q, want it said apart, with what ends the job", changed)
	}
	other := nodes.RefusalError(1, "prepare 3/remux/0123456789abcdef", "this node does not prepare", "")
	if strings.Contains(other.Error(), "changed since the scan") {
		t.Errorf("another refusal says %q, want it not taken for a changed file", other)
	}
}

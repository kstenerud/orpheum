// SPDX-License-Identifier: GPL-3.0-or-later

package renditions

import (
	"context"
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

// progressQueue keeps each job's progress as the job view would.
type progressQueue struct {
	progress map[int64]float64
	// released are the jobs said to be no longer wanted.
	released []jobs.Target
}

func (queue *progressQueue) Enqueue(context.Context, jobs.Request) (bool, error) { return true, nil }

func (*progressQueue) KeepWanted(context.Context, jobs.Kind, jobs.Target) error { return nil }

func (queue *progressQueue) ReleaseWanted(_ context.Context, _ jobs.Kind, target jobs.Target) error {
	queue.released = append(queue.released, target)
	return nil
}

func (queue *progressQueue) SetProgress(_ context.Context, id int64, fraction float64) error {
	queue.progress[id] = fraction
	return nil
}

func TestRecordProgress_PutsTheShareOfTheSourceMadeOnTheJob(t *testing.T) {
	t.Parallel()
	queue := &progressQueue{progress: map[int64]float64{}}
	service := &Service{cache: storetest.OpenCache(t), queue: queue}
	for _, step := range []struct {
		job                           int64
		preparedThroughMs, durationMs int64
		want                          float64
		isKnown                       bool
	}{
		{job: 1, preparedThroughMs: 25_000, durationMs: 100_000, want: 0.25, isKnown: true},
		// Never past the end.
		{job: 2, preparedThroughMs: 120_000, durationMs: 100_000, want: 1, isKnown: true},
		// A length the probe did not say.
		{job: 3, preparedThroughMs: 1_000, durationMs: 0},
	} {
		job := jobs.Job{ID: step.job, Target: jobs.Target{RenditionID: 9}}
		err := service.recordProgress(t.Context(), job.ID, testRun, step.preparedThroughMs, step.durationMs)
		if err != nil {
			t.Fatal(err)
		}
		got, isKnown := queue.progress[step.job]
		if isKnown != step.isKnown || got != step.want {
			t.Errorf("job %d at %d of %d ms: %v (%v), want %v (%v)",
				step.job, step.preparedThroughMs, step.durationMs, got, isKnown, step.want, step.isKnown)
		}
	}
}

// fakeNodes is a node that takes commands and does nothing with them.
type fakeNodes struct {
	sent chan nodeproto.Command
	// notReady is what Ready answers: nil for a node work can be sent to, a parking for one away.
	notReady error
}

func (node fakeNodes) Enqueue(_ int64, command nodeproto.Command) { node.sent <- command }

func (node fakeNodes) ParkUnlessConnected(context.Context, int64) error { return node.notReady }

func (fakeNodes) Withdraw(int64, string) bool { return false }

func (fakeNodes) StartOf(int64) string { return "" }

func (node fakeNodes) Send(_ context.Context, nodeID int64, command nodeproto.Command) error {
	node.Enqueue(nodeID, command)
	return nil
}

func (fakeNodes) IsConnected(context.Context, int64) bool { return true }

func (fakeNodes) NodeConnection(context.Context, int64) (nodes.Connection, time.Time) {
	return nodes.Connected, time.Time{}
}

func (fakeNodes) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (fakeNodes) Unpark(context.Context, jobs.Destination) {}

func (fakeNodes) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	return nodes.Resources{}, false, nil
}

func (fakeNodes) SlowestReadBytesPerSecond(int64) int64 { return 20 << 20 }

// progressed hands node 1's word on where the attempt command made is to the job waiting for it, as
// the node channel does.
func (service *Service) progressed(ctx context.Context, command nodeproto.Command, state nodeproto.ProgressState) {
	service.exchanges.DeliverProgress(ctx, 1, nodeproto.Progress{
		JobID: command.JobID, AttemptToken: command.AttemptToken, State: state,
	})
}

// reported hands node 1's report on how the attempt command made ended to its recorder and the job
// waiting for it, as the node channel does, and fails the test if it is not taken.
func (service *Service) reported(t *testing.T, command nodeproto.Command, report nodeproto.PrepareReport) {
	t.Helper()
	err := service.exchanges.Deliver(t.Context(), 1, nodeproto.Result{
		JobID: command.JobID, AttemptToken: command.AttemptToken, Work: &command.Work, Prepare: &report,
	})
	if err != nil {
		t.Errorf("the node's report was not taken: %v", err)
	}
}

// testRun is the making insertRendition records, and testOrder a prepare of it on node 1.
var (
	testRun   = renditionRun{fileID: 3, profile: "remux", runID: "0123456789abcdef"}
	testOrder = prepareOrder{run: testRun, command: nodeproto.Prepare{RenditionDir: testRun.dir()}, nodeID: 1}
)

func TestHandle_APrepareTheNodeHoldsWaitingForASlotOutlivesTheSilenceLimit(t *testing.T) {
	t.Parallel()
	const silence = time.Minute
	node := fakeNodes{sent: make(chan nodeproto.Command, 2)}
	clockSource := clocktest.NewFake(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC))
	service, start := preparesOnAFakeNode(t, node, clockSource, silence)
	job := jobs.Job{ID: 7, Target: jobs.Target{RenditionID: insertRendition(t, service)}}
	ended := start(job)
	command := testutil.Receive(t, node.sent, "the prepare command")
	// The exchanges look for silence at each tick, as the server's do.
	tick := make(chan time.Time)
	go service.exchanges.Watch(t.Context(), tick)
	// The node holds the prepare behind another for three silence limits, saying so each time.
	for range 6 {
		clockSource.Advance(silence / 2)
		service.progressed(t.Context(), command, nodeproto.ProgressWaiting)
		select {
		case tick <- clockSource.Now():
		case <-t.Context().Done():
		}
	}
	service.reported(t, command, nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 10})
	if err := testutil.Receive(t, ended, "the prepare job to end"); err != nil {
		t.Errorf("a prepare waiting for a slot, said so: %v, want it to wait and end done", err)
	}
}

// A player that leaves lets go of the prepare of its file's rendition in its profile (D258 §2); a
// profile the file has no rendition in has nothing to let go of.
func TestReleaseWanted_LetsGoOfThePrepareOfTheFilesRenditionInTheProfile(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	queue := &progressQueue{progress: map[int64]float64{}}
	service := &Service{cache: cache, queue: queue}
	var rendition int64
	err := cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		var insertErr error
		rendition, insertErr = queries.InsertRendition(t.Context(), cachegen.InsertRenditionParams{
			SourceFile: 3, SourcePayloadHash: []byte{1}, Profile: "h264-8m", NodeID: 1, SegmentLengthSeconds: 6,
			RunID: "0123456789abcdef", CreatedAt: 1,
		})
		return insertErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ReleaseWanted(t.Context(), 3, nodeproto.ProfileRemux); err != nil {
		t.Fatal(err)
	}
	if len(queue.released) != 0 {
		t.Errorf("leaving a profile with no rendition let go of %v", queue.released)
	}
	if err := service.ReleaseWanted(t.Context(), 3, nodeproto.ProfileH264At8M); err != nil {
		t.Fatal(err)
	}
	if len(queue.released) != 1 || queue.released[0] != (jobs.Target{RenditionID: rendition}) {
		t.Errorf("the prepares let go of = %v; want rendition %d's", queue.released, rendition)
	}
}

func TestFinish_RecordsWhatMadeTheVideoAndHowFast(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	service := &Service{cache: cache, clock: clocktest.NewFake(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC))}
	var rendition int64
	err := cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		var insertErr error
		rendition, insertErr = queries.InsertRendition(t.Context(), cachegen.InsertRenditionParams{
			SourceFile: 3, SourcePayloadHash: []byte{1}, Profile: "h264-8m", NodeID: 1, SegmentLengthSeconds: 6,
			RunID: "0123456789abcdef", CreatedAt: 1,
		})
		return insertErr
	})
	if err != nil {
		t.Fatal(err)
	}
	done := nodeproto.PrepareReport{
		State: nodeproto.PrepareDone, SizeBytes: 10, VideoEncoder: "h264_qsv",
		RealtimeFactor: 6.5,
	}
	run := renditionRun{fileID: 3, profile: "h264-8m", runID: "0123456789abcdef"}
	if isCurrent, finishErr := service.finish(t.Context(), run, &done, ""); finishErr != nil || !isCurrent {
		t.Fatalf("finishing the making = %v, %v; want it recorded", isCurrent, finishErr)
	}
	row, err := cache.Read().GetRenditionByID(t.Context(), rendition)
	if err != nil {
		t.Fatal(err)
	}
	if row.VideoEncoder == nil || *row.VideoEncoder != "h264_qsv" || row.RealtimeFactor == nil ||
		*row.RealtimeFactor != 6.5 {
		t.Errorf("the rendition records encoder %v at speed %v; want h264_qsv at 6.5 (D225 §3)",
			row.VideoEncoder, row.RealtimeFactor)
	}
}

// A prepare its node holds waiting for a slot has no cap of its own: however long it waits, saying
// so, it is not given up here. Whether the work ahead of it moves is the core's to judge, by the
// node's status (internal/core/nodes, D233 §5).
func TestHandle_APrepareWaitingForASlotIsNotGivenUpForHowLongItWaits(t *testing.T) {
	t.Parallel()
	node := fakeNodes{sent: make(chan nodeproto.Command, 2)}
	clockSource := clocktest.NewFake(time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	service, start := preparesOnAFakeNode(t, node, clockSource, neverSilent)
	ended := start(jobs.Job{ID: 7, Target: jobs.Target{RenditionID: insertRendition(t, service)}})
	command := testutil.Receive(t, node.sent, "the prepare command")
	for range 24 {
		service.progressed(t.Context(), command, nodeproto.ProgressWaiting)
		clockSource.Advance(time.Hour)
	}
	service.progressed(t.Context(), command, nodeproto.ProgressWaiting)
	select {
	case err := <-ended:
		t.Errorf("a prepare waiting a day for a slot ended %v; want it held", err)
	default:
	}
	service.reported(t, command, nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 10})
	if err := testutil.Receive(t, ended, "the prepare job to end"); err != nil {
		t.Errorf("the prepare that waited a day ended %v, want done", err)
	}
}

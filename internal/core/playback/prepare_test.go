// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// progressQueue keeps each job's progress as the job view would.
type progressQueue struct {
	progress map[int64]float64
}

func (queue *progressQueue) Enqueue(context.Context, jobs.Request) (bool, error) { return true, nil }

func (queue *progressQueue) SetProgress(_ context.Context, id int64, fraction float64) error {
	queue.progress[id] = fraction
	return nil
}

func TestRecordProgress_PutsTheShareOfTheSourceMadeOnTheJob(t *testing.T) {
	t.Parallel()
	queue := &progressQueue{progress: map[int64]float64{}}
	all := &prepares{cache: storetest.OpenCache(t), queue: queue}
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
		err := all.recordProgress(t.Context(), job.ID, testRun, step.preparedThroughMs, step.durationMs)
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
type fakeNodes struct{ sent chan nodeproto.Command }

func (node fakeNodes) Enqueue(_ int64, command nodeproto.Command) { node.sent <- command }

func (fakeNodes) Ready(context.Context, int64) error { return nil }

func (fakeNodes) Withdraw(int64, string) bool { return false }

func (node fakeNodes) Send(_ context.Context, nodeID int64, command nodeproto.Command) error {
	node.Enqueue(nodeID, command)
	return nil
}

func (fakeNodes) IsConnected(context.Context, int64) bool { return true }

func (fakeNodes) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (fakeNodes) SlowestReadBytesPerSecond(int64) int64 { return 20 << 20 }

// progressed hands node 1's word on where the attempt command made is to the job waiting for it, as
// the node channel does.
func (all *prepares) progressed(ctx context.Context, command nodeproto.Command, state nodeproto.ProgressState) {
	all.exchanges.DeliverProgress(ctx, 1, nodeproto.Progress{
		JobID: command.JobID, AttemptToken: command.AttemptToken, State: state,
	})
}

// reported hands node 1's report on how the attempt command made ended to its recorder and the job
// waiting for it, as the node channel does, and fails the test if it is not taken.
func (all *prepares) reported(t *testing.T, command nodeproto.Command, report nodeproto.PrepareReport) {
	t.Helper()
	err := all.exchanges.Deliver(t.Context(), 1, nodeproto.Result{
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
	all, start := preparesOnAFakeNode(t, node, clockSource)
	all.maxSilence = silence
	job := jobs.Job{ID: 7, Target: jobs.Target{RenditionID: insertRendition(t, all)}}
	ended := start(job)
	command := testutil.Receive(t, node.sent, "the prepare command")
	// The node holds the prepare behind another for three silence limits, saying so each time.
	for range 6 {
		clockSource.Advance(silence / 2)
		all.progressed(t.Context(), command, nodeproto.ProgressWaiting)
	}
	all.reported(t, command, nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 10})
	if err := testutil.Receive(t, ended, "the prepare job to end"); err != nil {
		t.Errorf("a prepare waiting for a slot, said so: %v, want it to wait and end done", err)
	}
}

func TestFinish_RecordsWhatMadeTheVideoAndHowFast(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	all := &prepares{cache: cache, clock: clocktest.NewFake(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC))}
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
	if isCurrent, finishErr := all.finish(t.Context(), run, &done, ""); finishErr != nil || !isCurrent {
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

// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"log/slog"
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
		if err := all.recordProgress(t.Context(), job, step.preparedThroughMs, step.durationMs); err != nil {
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

func (node fakeNodes) Send(_ context.Context, nodeID int64, command nodeproto.Command) error {
	node.Enqueue(nodeID, command)
	return nil
}

func (fakeNodes) IsConnected(context.Context, int64) bool { return true }

func (fakeNodes) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (fakeNodes) SlowestReadBytesPerSecond(int64) int64 { return 20 << 20 }

// reported hands node 1's report to the job waiting for it, as the node channel does.
func (all *prepares) reported(ctx context.Context, report nodeproto.PrepareReport) {
	all.exchanges.Deliver(ctx, 1, nodeproto.Result{JobID: report.JobID, Prepare: &report})
}

func TestHandle_APrepareTheNodeHoldsWaitingForASlotOutlivesTheSilenceLimit(t *testing.T) {
	t.Parallel()
	const silence = time.Minute
	node := fakeNodes{sent: make(chan nodeproto.Command, 2)}
	clockSource := clocktest.NewFake(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC))
	all := &prepares{
		cache: storetest.OpenCache(t), queue: &progressQueue{progress: map[int64]float64{}}, nodes: node,
		clock: clockSource, maxSilence: silence, maxSlotWait: maxSlotWait,
		exchanges: nodes.NewExchanges(node, clockSource, slog.New(slog.DiscardHandler)),
	}
	job := jobs.Job{ID: 7, Target: jobs.Target{RenditionID: 1}}
	ended := make(chan error, 1)
	go func() { ended <- all.handle(t.Context(), job, prepareOrder{command: nodeproto.Prepare{}, nodeID: 1}) }()
	testutil.Receive(t, node.sent, "the prepare command")
	// The node holds the prepare behind another for three silence limits, saying so each time.
	for range 6 {
		clockSource.Advance(silence / 2)
		all.reported(t.Context(), nodeproto.PrepareReport{JobID: 7, State: nodeproto.PrepareWaiting})
	}
	all.reported(t.Context(), nodeproto.PrepareReport{JobID: 7, State: nodeproto.PrepareDone, SizeBytes: 10})
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
	if err = all.finish(t.Context(), rendition, &done, ""); err != nil {
		t.Fatal(err)
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

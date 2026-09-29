// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// progressQueue keeps each job's progress as the job view would.
type progressQueue struct {
	progress map[int64]float64
}

func (queue *progressQueue) Enqueue(context.Context, jobs.New) (bool, error) { return true, nil }

func (queue *progressQueue) SetProgress(_ context.Context, id int64, fraction float64) error {
	queue.progress[id] = fraction
	return nil
}

func TestProgressed_PutsTheShareOfTheSourceMadeOnTheJob(t *testing.T) {
	t.Parallel()
	queue := &progressQueue{progress: map[int64]float64{}}
	all := &prepares{cache: storetest.OpenCache(t), queue: queue}
	for _, step := range []struct {
		job                    int64
		preparedMs, durationMs int64
		want                   float64
		isKnown                bool
	}{
		{job: 1, preparedMs: 25_000, durationMs: 100_000, want: 0.25, isKnown: true},
		{job: 2, preparedMs: 120_000, durationMs: 100_000, want: 1, isKnown: true}, // never past the end
		{job: 3, preparedMs: 1_000, durationMs: 0},                                 // a length the probe did not say
	} {
		job := jobs.Job{ID: step.job, Target: jobs.Target{RenditionID: 9}}
		if err := all.progressed(t.Context(), job, step.preparedMs, step.durationMs); err != nil {
			t.Fatal(err)
		}
		got, isKnown := queue.progress[step.job]
		if isKnown != step.isKnown || got != step.want {
			t.Errorf("job %d at %d of %d ms: %v (%v), want %v (%v)",
				step.job, step.preparedMs, step.durationMs, got, isKnown, step.want, step.isKnown)
		}
	}
}

// commanded is a node that takes commands and does nothing with them.
type commanded struct{ sent chan nodeproto.Command }

func (nodes commanded) Enqueue(_ int64, command nodeproto.Command) { nodes.sent <- command }

func TestHandle_APrepareTheNodeHoldsWaitingForASlotOutlivesTheSilenceLimit(t *testing.T) {
	t.Parallel()
	const silence = 200 * time.Millisecond
	nodes := commanded{sent: make(chan nodeproto.Command, 1)}
	all := &prepares{
		cache: storetest.OpenCache(t), queue: &progressQueue{progress: map[int64]float64{}}, nodes: nodes,
		clock: clocktest.NewFake(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)), maxSilence: silence,
		waiting: newWaiting[nodeproto.PrepareReport](slog.New(slog.DiscardHandler), "a prepare report"),
	}
	job := jobs.Job{ID: 7, Target: jobs.Target{RenditionID: 1}}
	ended := make(chan error, 1)
	go func() { ended <- all.handle(t.Context(), job, making{command: nodeproto.Prepare{}, nodeID: 1}) }()
	<-nodes.sent
	// The node holds the prepare behind another for three silence limits, saying so each time.
	for range 6 {
		time.Sleep(silence / 2)
		all.reported(t.Context(), 1, nodeproto.PrepareReport{JobID: 7, State: nodeproto.PrepareWaiting})
	}
	all.reported(t.Context(), 1, nodeproto.PrepareReport{JobID: 7, State: nodeproto.PrepareDone, SizeBytes: 10})
	if err := <-ended; err != nil {
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

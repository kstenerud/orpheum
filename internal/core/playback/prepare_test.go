// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
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
		job := jobs.Job{ID: step.job, Subject: 9}
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

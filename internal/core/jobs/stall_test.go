// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRunDue_AStallCostsNoAttemptAndTheNextAttemptKnowsTheLastOneStalled(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	answers := []error{
		MarkStalled(errors.New("FFmpeg stood still")), MarkStalled(errors.New("FFmpeg stood still")),
		errors.New("the node said nothing"), nil,
	}
	var toldStalled []bool
	var attempts []int64
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(_ context.Context, job Job) error {
		toldStalled = append(toldStalled, job.PreviousAttemptToolFailed)
		attempts = append(attempts, job.AttemptNumber)
		answer := answers[0]
		answers = answers[1:]
		return answer
	})
	for range 4 {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		if job := jobs.job(t, 1); len(answers) == 3 &&
			(job.State != "queued" || job.AttemptCount != 0 || !strings.HasPrefix(*job.LastFailure, "stalled: ")) {
			t.Errorf("after a stall: %+v; want queued again, no attempt spent, the stall its last failure", job)
		}
		// A stall waits jobFirstRetryDelaySeconds, as the fixture sets it, however many there were.
		jobs.clock.Advance(10 * time.Second)
	}
	if want := []bool{false, true, true, false}; !slices.Equal(toldStalled, want) {
		t.Errorf("each attempt was told the last stalled %v, want %v", toldStalled, want)
	}
	if want := []int64{1, 1, 1, 2}; !slices.Equal(attempts, want) {
		t.Errorf("the attempts were numbered %v, want %v: a stall costs none", attempts, want)
	}
	if job := jobs.job(t, 1); job.State != stateDone {
		t.Errorf("the job ended %+v, want done", job)
	}
}

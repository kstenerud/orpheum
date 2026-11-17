// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
)

// errPassing is a failure the test's queue knows to pass.
var errPassing = errors.New("the host did not answer in time")

// toldFailures is a test's UnknownFailures: what the queue told it, by target.
type toldFailures struct {
	mutex  sync.Mutex
	failed map[target]string
	done   []target
}

func (told *toldFailures) Failed(_ context.Context, job jobqueue.Job[target], failure string) {
	told.mutex.Lock()
	defer told.mutex.Unlock()
	told.failed[job.Target] = failure
}

func (told *toldFailures) Done(_ context.Context, job jobqueue.Job[target]) {
	told.mutex.Lock()
	defer told.mutex.Unlock()
	told.done = append(told.done, job.Target)
}

// endsAsNamed is a job's work that ends as its target names: blocked for good, failing for a cause
// that passes, succeeding, or failing for a cause not known.
func endsAsNamed(_ context.Context, job jobqueue.Job[target]) error {
	switch job.Target {
	case "permanent":
		return jobqueue.Permanent(errors.New("no such work"))
	case "passing":
		return errPassing
	case "succeeds":
		return nil
	}
	return errors.New("the provider said what was not understood")
}

func TestRunDue_AFailureAtTheLimitIsKeptForGoodOnlyWhenKnownToBlockAndPassesOnlyWhenKnownToPass(t *testing.T) {
	t.Parallel()
	jobs := newFixtureWith(t, jobqueue.Config{
		Random: jobqueuetest.Unjittered(), IsPassingFailure: func(err error) bool { return errors.Is(err, errPassing) },
	})
	told := &toldFailures{failed: map[target]string{}}
	jobs.queue.TellUnknownFailures(told)
	for _, name := range []target{"unknown", "permanent", "passing", "succeeds"} {
		jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: name})
	}
	recorded := map[target]string{}
	keep := func(_ context.Context, job jobqueue.Job[target], failure string) error {
		recorded[job.Target] = failure
		return nil
	}
	runner := jobs.queue.NewRunner(kindFetch, endsAsNamed).WithFailureRecorder(keep)
	for range attemptLimit + 2 {
		jobs.runDue(t, runner)
		jobs.clock.Advance(25 * time.Hour)
	}
	if got := jobs.row(t, "unknown"); got.State != jobqueue.StateFailed ||
		told.failed["unknown"] != "the provider said what was not understood" {
		t.Errorf("unknown is %s, told %q; want it failed, and told as unknown with its error", got.State,
			told.failed["unknown"])
	}
	if _, isTold := told.failed["permanent"]; isTold || recorded["permanent"] != "no such work" {
		t.Errorf("permanent told %v, recorded %q; want it recorded for good and not told as unknown", told.failed,
			recorded["permanent"])
	}
	passing := jobs.row(t, "passing")
	if _, isTold := told.failed["passing"]; isTold || passing.State != jobqueue.StateQueued ||
		passing.AttemptCount >= attemptLimit {
		t.Errorf("passing is %s after %d attempts, told %v; want it queued again under the limit, never told",
			passing.State, passing.AttemptCount, told.failed)
	}
	if _, isRecorded := recorded["passing"]; isRecorded {
		t.Error("a failure known to pass was recorded as its item's own")
	}
	if want := jobs.clock.Now().Add(-25 * time.Hour).Add(24 * time.Hour); !passing.RunnableAt.Equal(want) {
		t.Errorf("passing runs again at %v; want the longest retry delay after its last attempt, %v",
			passing.RunnableAt, want)
	}
	if len(told.done) != 1 || told.done[0] != "succeeds" {
		t.Errorf("told done %v; want the job that succeeded", told.done)
	}
}

func TestRunDue_APanicOnTwoAttemptsInARowIsAFailureOfUnknownCause(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	told := &toldFailures{failed: map[target]string{}}
	jobs.queue.TellUnknownFailures(told)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "panics"})
	runner := jobs.queue.NewRunner(kindFetch, func(context.Context, jobqueue.Job[target]) error {
		panic("an index out of range")
	})
	for range 2 {
		jobs.runDue(t, runner)
		jobs.clock.Advance(time.Hour)
	}
	if _, isTold := told.failed["panics"]; !isTold || jobs.row(t, "panics").State != jobqueue.StateFailed {
		t.Errorf("a job that panicked twice is %s, told %v; want it failed and told as unknown",
			jobs.row(t, "panics").State, told.failed)
	}
}

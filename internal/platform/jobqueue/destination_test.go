// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
)

// host is the tests' destination: a host that goes down.
const host jobqueue.Destination = "host"

// errUnreachable is what the host answers while it is down.
var errUnreachable = errors.New("connection refused")

// downHost is a handler for jobs that need the host: it parks them while the host is down, and
// notes each job it ran and whether it ran as the host's probe.
type downHost struct {
	isDown bool
	ran    []target
	probes []target
}

func (fake *downHost) handle(ctx context.Context, job jobqueue.Job[target]) error {
	fake.ran = append(fake.ran, job.Target)
	if probed, isProbe := jobqueue.ProbeOf(ctx); isProbe && probed == host {
		fake.probes = append(fake.probes, job.Target)
	}
	if fake.isDown {
		return jobqueue.Park(jobqueue.Parking{Destination: host, Reason: "unreachable"}, errUnreachable)
	}
	return nil
}

func TestRunDue_AParkedDestinationHoldsItsJobsAtNoCostAndOtherWorkRunsOn(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a", Priority: 2, Destination: host})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "b", Priority: 1, Destination: host})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "local"})
	fake := &downHost{isDown: true}
	runner := jobs.queue.NewRunner(kindFetch, fake.handle)
	jobs.runDue(t, runner)
	if !slices.Equal(fake.ran, []target{"a", "local"}) {
		t.Errorf("ran %q; want the first job for the host, which parks it, then the job that needs no host", fake.ran)
	}
	for _, parked := range []target{"a", "b"} {
		row := jobs.row(t, parked)
		if row.State != jobqueue.StateQueued || row.AttemptCount != 0 || row.Destination != host {
			t.Errorf("%s: %s after %d attempts, kept with %q; want queued on the host with no attempt spent",
				parked, row.State, row.AttemptCount, row.Destination)
		}
	}
	state, isParked := jobs.queue.DestinationState(host)
	now := jobs.clock.Now()
	if !isParked || state.Reason != "unreachable" || !state.Since.Equal(now) ||
		!state.NextProbeAt.Equal(now.Add(jobqueue.FirstProbeDelay)) {
		t.Errorf("the host's state: %+v (parked %v); want parked as unreachable since now, probed in 30 s", state,
			isParked)
	}
	if listed := jobs.queue.ParkedDestinations(); len(listed) != 1 || listed[0].Destination != host {
		t.Errorf("parked destinations: %+v; want the host alone", listed)
	}
}

func TestRunDue_OneProbeOnABackoffFrom30sToAnHourAFailedOneCostingNothingThenAllResumeByPriority(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	for index, name := range []target{"low", "high", "middle"} {
		jobs.enqueue(t, jobqueue.Request[target]{
			Kind: kindFetch, Target: name, Priority: []int64{1, 3, 2}[index], Destination: host,
		})
	}
	fake := &downHost{isDown: true}
	runner := jobs.queue.NewRunner(kindFetch, fake.handle)
	jobs.runDue(t, runner)
	wantWaits := []time.Duration{
		30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute,
		32 * time.Minute, time.Hour, time.Hour,
	}
	for step, wait := range wantWaits {
		state, _ := jobs.queue.DestinationState(host)
		if got := state.NextProbeAt.Sub(jobs.clock.Now()); got != wait {
			t.Fatalf("probe %d is due in %v, want %v", step+1, got, wait)
		}
		jobs.clock.Set(state.NextProbeAt.Add(-time.Millisecond))
		ranBefore := len(fake.ran)
		jobs.runDue(t, runner)
		if len(fake.ran) != ranBefore {
			t.Fatalf("a job ran %v before its probe was due", fake.ran[ranBefore:])
		}
		jobs.clock.Set(state.NextProbeAt)
		jobs.runDue(t, runner)
		if len(fake.ran) != ranBefore+1 || fake.probes[len(fake.probes)-1] != "high" {
			t.Fatalf("at probe %d, ran %q; want one job, the highest, as the host's probe", step+1,
				fake.ran[ranBefore:])
		}
	}
	for _, name := range []target{"low", "high", "middle"} {
		if row := jobs.row(t, name); row.AttemptCount != 0 {
			t.Errorf("%s spent %d attempts while the host was down; a parked job and a failed probe spend none",
				name, row.AttemptCount)
		}
	}
	fake.isDown, fake.ran = false, nil
	state, _ := jobs.queue.DestinationState(host)
	jobs.clock.Set(state.NextProbeAt)
	jobs.runDue(t, runner)
	if want := []target{"high", "middle", "low"}; !slices.Equal(fake.ran, want) {
		t.Errorf("once the probe found the host, ran %q; want every job, in priority order: %q", fake.ran, want)
	}
	if _, isParked := jobs.queue.DestinationState(host); isParked {
		t.Error("the host is still parked after its probe succeeded")
	}
}

func TestRunDue_AProbeGoesAtTheTimeTheDestinationNamed(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a", Destination: host})
	after := jobs.clock.Now().Add(17 * time.Minute)
	runner := jobs.queue.NewRunner(kindFetch, func(context.Context, jobqueue.Job[target]) error {
		return jobqueue.Park(jobqueue.Parking{Destination: host, Reason: "asked us to wait", ProbeAt: after},
			errors.New("429"))
	})
	jobs.runDue(t, runner)
	if state, _ := jobs.queue.DestinationState(host); !state.NextProbeAt.Equal(after) {
		t.Errorf("next probe at %v, want the time the host named, %v", state.NextProbeAt, after)
	}
}

func TestEnqueue_APersonsRequestLooksAtAParkedDestinationAtOnce(t *testing.T) {
	t.Parallel()
	jobs := newFixtureWith(t, jobqueue.Config{Random: jobqueuetest.Unjittered(), PersonsRequestPriority: 20})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "background", Destination: host})
	fake := &downHost{isDown: true}
	runner := jobs.queue.NewRunner(kindFetch, fake.handle)
	jobs.runDue(t, runner)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "another", Priority: 5, Destination: host})
	jobs.runDue(t, runner)
	if len(fake.ran) != 1 {
		t.Fatalf("ran %q; a request below a person's priority must not probe", fake.ran)
	}
	// A sweep deriving a person's request again, at their priority, is no one waiting now.
	jobs.enqueue(t, jobqueue.Request[target]{
		Kind: kindFetch, Target: "derived", Priority: 20, IsDerived: true,
		Destination: host,
	})
	if state, _ := jobs.queue.DestinationState(host); !state.NextProbeAt.After(jobs.clock.Now()) {
		t.Fatalf("after a sweep's request, the next probe is at %v; want it left on its back-off", state.NextProbeAt)
	}
	fake.isDown = false
	// A person's repeat request for the parked job: it names no destination; its job's is used.
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "background", Priority: 20})
	if state, _ := jobs.queue.DestinationState(host); !state.NextProbeAt.Equal(jobs.clock.Now()) {
		t.Fatalf("after a person's request, the next probe is at %v; want now", state.NextProbeAt)
	}
	jobs.runDue(t, runner)
	if want := []target{"background", "background", "derived", "another"}; !slices.Equal(fake.ran, want) {
		t.Errorf("ran %q; want the person's job as the probe at once, then the rest", fake.ran)
	}
}

func TestRunDue_ADestinationThatAwaitsASignalIsNeverProbedAndResumesWhenUnparked(t *testing.T) {
	t.Parallel()
	jobs := newFixtureWith(t, jobqueue.Config{Random: jobqueuetest.Unjittered(), PersonsRequestPriority: 20})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a", Destination: "node"})
	var ran []target
	isConnected := false
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		ran = append(ran, job.Target)
		if !isConnected {
			return jobqueue.Park(jobqueue.Parking{Destination: "node", Reason: "not connected", AwaitsSignal: true},
				errors.New("not connected"))
		}
		return nil
	})
	jobs.runDue(t, runner)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a", Priority: 20})
	jobs.clock.Advance(2 * time.Hour)
	jobs.runDue(t, runner)
	if len(ran) != 1 {
		t.Fatalf("ran %q; a destination that only a signal brings back is never probed", ran)
	}
	isConnected = true
	jobs.queue.Unpark(t.Context(), "node")
	jobs.runDue(t, runner)
	if row := jobs.row(t, "a"); len(ran) != 2 || row.State != jobqueue.StateDone || row.AttemptCount != 1 {
		t.Errorf("after the signal: ran %q, the job %s after %d attempts; want it run once more and done after 1",
			ran, row.State, row.AttemptCount)
	}
}

// errFull is the tests' storage out of room.
var errFull = errors.New("database or disk is full")

func TestRunDue_AFullStorageStopsEveryClaimUntilAProbeFindsRoomAndNothingIsLeftRunning(t *testing.T) {
	t.Parallel()
	jobs := newFixtureWith(t, jobqueue.Config{
		Random: jobqueuetest.Unjittered(), StorageDestination: "storage",
		IsStorageFailure: func(err error) bool { return errors.Is(err, errFull) },
	})
	for _, name := range []target{"a", "b", "fills the disk", "cannot keep"} {
		jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: name})
	}
	var ran []target
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		ran = append(ran, job.Target)
		switch job.Target {
		case "fills the disk":
			jobs.storage.FailWrites(errFull) // its outcome cannot be written
		case "cannot keep":
			if len(ran) < 5 {
				return fmt.Errorf("keep what it made: %w", errFull)
			}
		}
		return nil
	})
	jobs.storage.FailWrites(errFull)
	jobs.runDue(t, runner)
	state, isParked := jobs.queue.DestinationState("storage")
	if !isParked || len(ran) != 0 {
		t.Fatalf("a claim the storage refused: ran %q, storage parked %v; want nothing run and it parked", ran,
			isParked)
	}
	jobs.clock.Set(state.NextProbeAt)
	jobs.runDue(t, runner)
	if state, _ = jobs.queue.DestinationState("storage"); state.NextProbeAt.Sub(jobs.clock.Now()) != time.Minute {
		t.Errorf("after a failed probe, the next is in %v; want the back-off's next step, a minute",
			state.NextProbeAt.Sub(jobs.clock.Now()))
	}
	jobs.storage.FailWrites(nil)
	jobs.clock.Set(state.NextProbeAt)
	jobs.runDue(t, runner)
	if want := []target{"a", "b", "fills the disk"}; !slices.Equal(ran, want) {
		t.Fatalf("once there was room, ran %q; want %q, then a stop: the disk filled again", ran, want)
	}
	if row := jobs.row(t, "fills the disk"); row.State != jobqueue.StateRunning {
		t.Fatalf("a job whose outcome could not be written is %s; the storage should still show it running",
			row.State)
	}
	jobs.storage.FailWrites(nil)
	state, _ = jobs.queue.DestinationState("storage")
	jobs.clock.Set(state.NextProbeAt)
	jobs.runDue(t, runner)
	if row := jobs.row(t, "fills the disk"); row.State != jobqueue.StateDone {
		t.Errorf("once there was room again, the job whose outcome waited is %s, want done", row.State)
	}
	if row := jobs.row(t, "cannot keep"); row.State != jobqueue.StateQueued || row.AttemptCount != 0 {
		t.Errorf("a job that could not keep its work for want of room: %s after %d attempts; want queued, free",
			row.State, row.AttemptCount)
	}
}

func TestPark_AFailureThatIsNotTheProbeChangesNothingOfHowAParkedDestinationComesBack(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	ctx := t.Context()
	jobs.queue.Park(ctx, jobqueue.Parking{Destination: host, Reason: "the key was refused", AwaitsSignal: true})
	jobs.queue.Park(ctx, jobqueue.Parking{Destination: host, Reason: "unreachable"})
	if state, _ := jobs.queue.DestinationState(host); !state.AwaitsSignal || state.Reason != "the key was refused" {
		t.Errorf("after a job that was not the probe failed: %+v; want it still waiting for the key", state)
	}
	jobs.queue.Unpark(ctx, host)
	jobs.queue.Park(ctx, jobqueue.Parking{Destination: host, Reason: "unreachable"})
	first, _ := jobs.queue.DestinationState(host)
	later := jobs.clock.Now().Add(time.Hour)
	jobs.queue.Park(ctx, jobqueue.Parking{Destination: host, Reason: "asked us to wait", ProbeAt: later})
	earlier := first.NextProbeAt.Add(-time.Second)
	jobs.queue.Park(ctx, jobqueue.Parking{Destination: host, Reason: "unreachable", ProbeAt: earlier})
	if state, _ := jobs.queue.DestinationState(host); !state.NextProbeAt.Equal(later) || state.Reason != "unreachable" {
		t.Errorf("after two failures that were not the probe: %+v; want its probe at the latest time one named, %v,"+
			" and why it went down kept", state, later)
	}
}

// probeBackoff is a probe policy that does not change.
type probeBackoff struct{ first, longest time.Duration }

func (backoff probeBackoff) FirstProbeDelay(context.Context) (time.Duration, error) {
	return backoff.first, nil
}

func (backoff probeBackoff) LongestProbeDelay(context.Context) (time.Duration, error) {
	return backoff.longest, nil
}

func TestPark_TheProbeBackoffIsThePolicys(t *testing.T) {
	t.Parallel()
	jobs := newFixtureWith(t, jobqueue.Config{
		Random: jobqueuetest.Unjittered(), Probes: probeBackoff{first: 10 * time.Second, longest: 25 * time.Second},
	})
	ctx := t.Context()
	for index, want := range []time.Duration{10 * time.Second, 20 * time.Second, 25 * time.Second, 25 * time.Second} {
		jobs.queue.Park(ctx, jobqueue.Parking{Destination: host, Reason: "unreachable", IsProbeFailure: index > 0})
		if state, _ := jobs.queue.DestinationState(host); state.NextProbeAt.Sub(jobs.clock.Now()) != want {
			t.Errorf("probe %d is due in %v, want %v: the policy's first wait, doubling to its longest", index+1,
				state.NextProbeAt.Sub(jobs.clock.Now()), want)
		}
	}
}

// A tool that failed in a way that parks its job, as a stall on a disk parks it on the disk's root,
// is still the tool's failure: the job spends no attempt, and its next attempt knows its last one's
// tool failed, so the same failure again is its own (D233 §6, §10).
func TestRunDue_AToolFailureThatParksItsJobIsStillATheToolsFailure(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a", Destination: host})
	var previous []bool
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		previous = append(previous, job.PreviousAttemptToolFailed)
		if len(previous) == 1 {
			return jobqueue.Park(jobqueue.Parking{Destination: host, Reason: "missing"},
				jobqueue.ToolFailed(errors.New("its disk did not answer")))
		}
		return nil
	})
	jobs.runDue(t, runner)
	row := jobs.row(t, "a")
	if row.AttemptCount != 0 || !row.LastAttemptToolFailed || row.Destination != host {
		t.Fatalf("after a tool failure that parks: %+v; want parked on the host, no attempt spent, its tool failed",
			row)
	}
	jobs.queue.Unpark(t.Context(), host)
	jobs.runDue(t, runner)
	if !slices.Equal(previous, []bool{false, true}) {
		t.Errorf("the attempts were told their last one's tool failed: %v; want false, then true", previous)
	}
}

// A probe whose job's tool failed says nothing of its destination: the destination stays parked,
// and the next probe waits the same step of the back-off again, from now, rather than the next.
func TestRunDue_AProbeWhoseToolFailedSaysNothingOfItsDestination(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a", Destination: host})
	isToolFailing := false
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, _ jobqueue.Job[target]) error {
		if isToolFailing {
			return jobqueue.ToolFailed(errors.New("FFmpeg crashed"))
		}
		return jobqueue.Park(jobqueue.Parking{Destination: host, Reason: "unreachable"}, errUnreachable)
	})
	jobs.runDue(t, runner)
	parked, _ := jobs.queue.DestinationState(host)
	isToolFailing = true
	jobs.clock.Set(parked.NextProbeAt)
	jobs.runDue(t, runner)
	state, isParked := jobs.queue.DestinationState(host)
	if !isParked || state.IsProbing || !state.NextProbeAt.Equal(jobs.clock.Now().Add(jobqueue.FirstProbeDelay)) {
		t.Errorf("after a probe whose tool failed, the host is %+v (parked %v); want parked, not probing, probed "+
			"again at the same first step, %v from now", state, isParked, jobqueue.FirstProbeDelay)
	}
}

// A destination with no room now, a node's full window, is parked and brought back as often as its
// work comes and goes, and logged at debug: a scan's reads logged a "down" reader twice a second
// (staging, 2026-10-16). A destination that failed is still a warning.
func TestPark_BackPressureIsNoOutageAndIsNotLoggedAsOne(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	now := clocktest.NewFake(time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC))
	queue := jobqueue.New(&jobqueuetest.Memory[target]{}, now,
		jobqueue.Config{
			Random: jobqueuetest.Unjittered(), Policy: fixedPolicy{attemptLimit: attemptLimit},
			Logger: slog.New(slog.NewTextHandler(&logged, nil)),
		})
	ctx := t.Context()
	for range 3 {
		queue.Park(ctx, jobqueue.Parking{Destination: host, Reason: "busy", AwaitsSignal: true, IsBackPressure: true})
		queue.Unpark(ctx, host)
	}
	if logged.Len() != 0 {
		t.Errorf("back-pressure logged %q; want nothing at info or above", logged.String())
	}
	queue.Park(ctx, jobqueue.Parking{Destination: host, Reason: "unreachable"})
	if line := logged.String(); !strings.Contains(line, "level=WARN") {
		t.Errorf("a failure logged %q; want a warning", line)
	}
}

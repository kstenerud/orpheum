// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// Destination names what a job needs that can be down: a host, a machine, a disk (D231 §5). The
// queue treats it as a name, kept beside a job's target; "" is a job that needs nothing that parks.
type Destination string

// Parking is a destination found down: the failure said nothing about the job's own item.
type Parking struct {
	Destination Destination
	// Reason is why, in a person's words: "unreachable", "the key was refused", "not connected".
	Reason string
	// Since is when it went down, when the failure knows better than now (a node last heard at
	// 14:02); zero is now.
	Since time.Time
	// ProbeAt is when to look again, when the destination named the time itself (a Retry-After);
	// zero is the next step of the probe back-off.
	ProbeAt time.Time
	// AwaitsSignal is a destination no probe can bring back: only Queue.Unpark does, when
	// something outside the queue sees it return (a node's hello, a scan finding a root, a key
	// changed, room made).
	AwaitsSignal bool
	// IsProbe is a failure that was the destination's probe: a person's synchronous call to a
	// parked host counts as one (D231 §6). The queue knows its own probes; a caller outside a job
	// says so here.
	IsProbe bool
}

// DestinationState is a parked destination, as a person is shown it (D231 §12).
type DestinationState struct {
	Destination Destination
	Reason      string
	Since       time.Time
	// NextProbeAt is when it is looked at next; zero when only a signal brings it back.
	NextProbeAt  time.Time
	AwaitsSignal bool
	// IsProbing is whether its probe is running now.
	IsProbing bool
}

// ProbePolicy is what the queue reads when a destination parks: how long before its first probe,
// after which each waits twice as long, and the longest any waits. Each wait is jittered as a
// retry's is.
type ProbePolicy interface {
	FirstProbeDelay(ctx context.Context) (time.Duration, error)
	LongestProbeDelay(ctx context.Context) (time.Duration, error)
}

// The probe back-off a queue given no ProbePolicy keeps, and one whose policy cannot be read falls
// back to: 30 s doubling to an hour, D231 §6's. They are fixed here, not settings: this package
// knows none, and a program sets its own back-off through ProbePolicy (Orpheum's are
// destination_first_probe_delay_seconds and destination_longest_probe_delay_seconds).
const (
	FirstProbeDelay   = 30 * time.Second
	LongestProbeDelay = time.Hour
)

// parkedDestination is one parked destination's state and how far its probes have backed off.
type parkedDestination struct {
	state     DestinationState
	probeStep int64
}

// destinations are the parked destinations, held in memory: after a restart, the first job for
// each probes it again (D231 §6).
type destinations struct {
	mutex    sync.Mutex
	parkedBy map[Destination]*parkedDestination // guarded by mutex
}

// probeContextKey carries, in a handler's context, the destination its job is the probe of.
type probeContextKey struct{}

// ProbeOf is the destination a running job is the probe of, if it is one: a provider asked by the
// probe lets it through to a host it otherwise answers is parked.
func ProbeOf(ctx context.Context) (Destination, bool) {
	destination, isProbe := ctx.Value(probeContextKey{}).(Destination)
	return destination, isProbe
}

// jobContextKey marks a context as a running job's.
type jobContextKey struct{}

// IsInJob is whether ctx is a running job's: the queue records what the job's answer says of its
// destination, so a provider asked from a job leaves the destination's state to it.
func IsInJob(ctx context.Context) bool {
	_, inJob := ctx.Value(jobContextKey{}).(bool)
	return inJob
}

// Park marks a destination down, or, if it is down, notes the failure. A failed probe backs the
// next one off a step and says why it is down now; any other failure of a parked destination (a
// job that ran before it was parked, or one that names none) changes nothing of how it comes
// back, unless it names a later time to look. It wakes nothing: parked work waits.
func (queue *Queue[T]) Park(ctx context.Context, parking Parking) {
	if parking.Destination == "" {
		return
	}
	now := queue.clock.Now()
	queue.destinations.mutex.Lock()
	defer queue.destinations.mutex.Unlock()
	parked, isParked := queue.destinations.parkedBy[parking.Destination]
	if isParked && !parking.IsProbe {
		if !parked.state.AwaitsSignal && parking.ProbeAt.After(parked.state.NextProbeAt) {
			parked.state.NextProbeAt = parking.ProbeAt
		}
		return
	}
	if !isParked {
		parked = &parkedDestination{state: DestinationState{
			Destination: parking.Destination, Since: parking.Since,
		}}
		if parked.state.Since.IsZero() {
			parked.state.Since = now
		}
		queue.destinations.parkedBy[parking.Destination] = parked
		queue.logger.WarnContext(ctx, "a destination is down; its work waits", "destination", parking.Destination,
			"reason", parking.Reason)
	} else {
		parked.probeStep++ // a failed probe
		parked.state.IsProbing = false
	}
	parked.state.Reason, parked.state.AwaitsSignal = parking.Reason, parking.AwaitsSignal
	switch {
	case parking.AwaitsSignal:
		parked.state.NextProbeAt, parked.state.IsProbing = time.Time{}, false
	case !parking.ProbeAt.IsZero():
		parked.state.NextProbeAt = parking.ProbeAt
	default:
		parked.state.NextProbeAt = now.Add(queue.jittered(ctx, queue.probeDelay(ctx, parked.probeStep)))
	}
}

// probeDelay is the wait before a destination's probe at a step of its back-off, as the queue's
// ProbePolicy says.
func (queue *Queue[T]) probeDelay(ctx context.Context, step int64) time.Duration {
	first, longest := FirstProbeDelay, LongestProbeDelay
	if queue.probes != nil {
		readFirst, firstErr := queue.probes.FirstProbeDelay(ctx)
		readLongest, longestErr := queue.probes.LongestProbeDelay(ctx)
		if err := errors.Join(firstErr, longestErr); err != nil {
			queue.logger.WarnContext(ctx, "could not read the probe back-off; keeping 30 s to an hour", "err", err)
		} else {
			first, longest = readFirst, max(readLongest, readFirst)
		}
	}
	wait := first
	for range step {
		if wait >= longest/2 {
			return longest
		}
		wait *= 2
	}
	return wait
}

// Unpark marks a destination up and wakes every runner, so its work resumes, highest priority
// first. A destination not parked is left as it is.
func (queue *Queue[T]) Unpark(ctx context.Context, destination Destination) {
	queue.destinations.mutex.Lock()
	_, wasParked := queue.destinations.parkedBy[destination]
	delete(queue.destinations.parkedBy, destination)
	queue.destinations.mutex.Unlock()
	if wasParked {
		queue.logger.InfoContext(ctx, "a destination is back; its work resumes", "destination", destination)
		queue.Wake()
	}
}

// ProbeNow looks at a parked destination at once, for a person waiting on its work (D231 §6): its
// next probe is due now. One that only a signal brings back is left to it.
func (queue *Queue[T]) ProbeNow(destination Destination) {
	queue.destinations.mutex.Lock()
	parked, isParked := queue.destinations.parkedBy[destination]
	isDue := isParked && !parked.state.AwaitsSignal
	if isDue {
		parked.state.NextProbeAt = queue.clock.Now()
	}
	queue.destinations.mutex.Unlock()
	if isDue {
		queue.Wake()
	}
}

// DestinationState is a destination's state, and whether it is parked.
func (queue *Queue[T]) DestinationState(destination Destination) (DestinationState, bool) {
	queue.destinations.mutex.Lock()
	defer queue.destinations.mutex.Unlock()
	parked, isParked := queue.destinations.parkedBy[destination]
	if !isParked {
		return DestinationState{}, false
	}
	return parked.state, true
}

// ParkedDestinations are every parked destination, by name.
func (queue *Queue[T]) ParkedDestinations() []DestinationState {
	queue.destinations.mutex.Lock()
	defer queue.destinations.mutex.Unlock()
	states := make([]DestinationState, 0, len(queue.destinations.parkedBy))
	for _, parked := range queue.destinations.parkedBy {
		states = append(states, parked.state)
	}
	slices.SortFunc(states, func(one, other DestinationState) int {
		return compareDestinations(one.Destination, other.Destination)
	})
	return states
}

func compareDestinations(one, other Destination) int {
	switch {
	case one < other:
		return -1
	case one > other:
		return 1
	}
	return 0
}

// unclaimable are the destinations a claim passes over at now: every parked one whose probe is
// not due, or is running, or that only a signal brings back.
func (queue *Queue[T]) unclaimable(now time.Time) []Destination {
	queue.destinations.mutex.Lock()
	defer queue.destinations.mutex.Unlock()
	var parked []Destination
	for destination, state := range queue.destinations.parkedBy {
		if !isProbeDue(state.state, now) {
			parked = append(parked, destination)
		}
	}
	slices.SortFunc(parked, compareDestinations)
	return parked
}

func isProbeDue(state DestinationState, now time.Time) bool {
	return !state.AwaitsSignal && !state.IsProbing && !now.Before(state.NextProbeAt)
}

// beginProbe makes a claimed job its parked destination's probe, if its probe is due: the one job
// that looks at it. It says whether the job is the probe.
func (queue *Queue[T]) beginProbe(destination Destination, now time.Time) bool {
	queue.destinations.mutex.Lock()
	defer queue.destinations.mutex.Unlock()
	parked, isParked := queue.destinations.parkedBy[destination]
	if !isParked || !isProbeDue(parked.state, now) {
		return false
	}
	parked.state.IsProbing = true
	return true
}

// endProbeUnanswered ends a probe whose job said nothing of its destination (its tool failed):
// the next probe is at the same step of the back-off, from now.
func (queue *Queue[T]) endProbeUnanswered(ctx context.Context, destination Destination) {
	queue.destinations.mutex.Lock()
	defer queue.destinations.mutex.Unlock()
	parked, isParked := queue.destinations.parkedBy[destination]
	if !isParked || !parked.state.IsProbing {
		return
	}
	parked.state.IsProbing = false
	parked.state.NextProbeAt = queue.clock.Now().Add(queue.jittered(ctx, queue.probeDelay(ctx, parked.probeStep)))
}

// isParked is whether a destination is parked.
func (queue *Queue[T]) isParked(destination Destination) bool {
	_, isParked := queue.DestinationState(destination)
	return isParked
}

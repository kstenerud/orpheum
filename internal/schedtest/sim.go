// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"slices"
	"time"
)

// Sim is virtual time and the events waiting for it. Its time is a duration since the run began.
// It is not safe for concurrent use: a simulation runs on one goroutine.
type Sim struct {
	now time.Duration
	// events wait in time order, and in the order they were made at the same time. Few wait at once,
	// so a sorted slice serves.
	events []event
	order  int
}

// Now is the simulation's time since the run began.
func (sim *Sim) Now() time.Duration { return sim.now }

// At runs do when the simulation's time reaches at, or at once in time order if at has passed.
// Events at the same time run in the order they were made.
func (sim *Sim) At(at time.Duration, do func()) {
	sim.order++
	next := event{at: max(at, sim.now), order: sim.order, do: do}
	index, _ := slices.BinarySearchFunc(sim.events, next, compareEvents)
	sim.events = slices.Insert(sim.events, index, next)
}

// After runs do wait from now.
func (sim *Sim) After(wait time.Duration, do func()) { sim.At(sim.now+wait, do) }

// RunUntil runs every event due by end, in time order, and leaves the time at end.
func (sim *Sim) RunUntil(end time.Duration) {
	for len(sim.events) > 0 && sim.events[0].at <= end {
		next := sim.events[0]
		sim.events = sim.events[1:]
		sim.now = next.at
		next.do()
	}
	sim.now = max(sim.now, end)
}

// event is something to do at a time.
type event struct {
	at    time.Duration
	order int
	do    func()
}

// compareEvents orders events by time, then by when they were made.
func compareEvents(first, second event) int {
	if first.at != second.at {
		return int(first.at - second.at)
	}
	return first.order - second.order
}

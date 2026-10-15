// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"sync"
)

// backgroundPassedOverAtMost is how many people's writes may go ahead of the oldest waiting
// background write before it goes next: people come first, and background work is never starved
// (the independent audit's DL11 amendment: strict priority without aging starves). A safety bound,
// not a tuning: a person's write holds the writer for microseconds, so four of them delay
// background work by nothing anyone sees.
const backgroundPassedOverAtMost = 4

// writerGate hands a database's one writer to one write at a time, in its order: the oldest
// person's write first, unless the oldest background write has been passed over
// backgroundPassedOverAtMost times. Left to itself, database/sql's pool would hand it to a random
// waiter (Go's connRequests.TakeRandom), with no regard for who waits.
type writerGate struct {
	mutex  sync.Mutex
	isHeld bool
	// holder names the write that holds the writer, for a slow write's report; "" when no one
	// asked to time writes.
	holder     string
	people     []*writeWaiter
	background []*writeWaiter
	passedOver int // how many people's writes went ahead of background[0]
}

// writeWaiter is one write waiting for the writer; ready is closed when it has it.
type writeWaiter struct {
	ready chan struct{}
	name  string
}

// acquire waits for the writer and holds it, in the gate's order, or returns ctx's error. Its name,
// the write's caller, is kept while it holds the writer.
func (gate *writerGate) acquire(ctx context.Context, isPerson bool, name string) error {
	gate.mutex.Lock()
	if !gate.isHeld {
		gate.isHeld, gate.holder = true, name
		gate.mutex.Unlock()
		return nil
	}
	waiter := &writeWaiter{ready: make(chan struct{}), name: name}
	if isPerson {
		gate.people = append(gate.people, waiter)
	} else {
		gate.background = append(gate.background, waiter)
	}
	gate.mutex.Unlock()
	select {
	case <-waiter.ready:
		return nil
	case <-ctx.Done():
		gate.mutex.Lock()
		removed := gate.forget(waiter)
		gate.mutex.Unlock()
		if !removed {
			gate.release() // it was handed the writer as it gave up: hand it on
		}
		return fmt.Errorf("wait for the writer: %w", ctx.Err())
	}
}

// forget takes waiter out of its queue, and reports whether it was still waiting.
func (gate *writerGate) forget(waiter *writeWaiter) bool {
	for _, queue := range []*[]*writeWaiter{&gate.people, &gate.background} {
		for i, each := range *queue {
			if each == waiter {
				*queue = append((*queue)[:i], (*queue)[i+1:]...)
				if queue == &gate.background && i == 0 {
					gate.passedOver = 0
				}
				return true
			}
		}
	}
	return false
}

// release hands the writer to the next write in the gate's order, or frees it.
func (gate *writerGate) release() {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	next := gate.takeNext()
	if next == nil {
		gate.isHeld, gate.holder = false, ""
		return
	}
	gate.holder = next.name
	close(next.ready)
}

// takeNext takes the write that goes next off its queue: the oldest person's, unless the oldest
// background write has been passed over enough.
func (gate *writerGate) takeNext() *writeWaiter {
	switch {
	case len(gate.people) > 0 && (len(gate.background) == 0 || gate.passedOver < backgroundPassedOverAtMost):
		next := gate.people[0]
		gate.people = gate.people[1:]
		if len(gate.background) > 0 {
			gate.passedOver++
		}
		return next
	case len(gate.background) > 0:
		next := gate.background[0]
		gate.background = gate.background[1:]
		gate.passedOver = 0
		return next
	default:
		return nil
	}
}

// heldBy names the write that holds the writer now, "" when none does or no one times writes.
func (gate *writerGate) heldBy() string {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	return gate.holder
}

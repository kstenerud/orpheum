// SPDX-License-Identifier: AGPL-3.0-or-later

package clocktest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Fake is a Clock whose time moves only when a test moves it, and whose timers fire when it moves
// past them. It is safe for concurrent use.
type Fake struct {
	mutex  sync.Mutex
	now    time.Time
	timers []*fakeTimer // the armed ones; guarded by mutex
	// armed, once AwaitArmed waits, is closed at the next arming of a timer; guarded by mutex.
	armed chan struct{}
}

// Compile-time proof that *Fake satisfies clock.Clock.
var _ clock.Clock = (*Fake)(nil)

// NewFake returns a Fake that reads start until it is advanced or set.
func NewFake(start time.Time) *Fake {
	return &Fake{now: start}
}

// Now returns the Fake's current time.
func (fake *Fake) Now() time.Time {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.now
}

// Advance moves the Fake's time forward by step. A negative step moves it backward, which is
// how a test simulates a wall-clock correction.
func (fake *Fake) Advance(step time.Duration) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.now = fake.now.Add(step)
	fake.fireDueLocked()
}

// AdvanceToNext moves the Fake's time to the earliest armed timer's, firing it, and reports whether
// one was armed; with none, the time stays. A test that runs code over a span of fake time steps it
// so, and the time never runs ahead of code a busy machine slowed.
func (fake *Fake) AdvanceToNext() bool {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if len(fake.timers) == 0 {
		return false
	}
	next := fake.timers[0].firesAt
	for _, timer := range fake.timers[1:] {
		if timer.firesAt.Before(next) {
			next = timer.firesAt
		}
	}
	if next.After(fake.now) {
		fake.now = next
	}
	fake.fireDueLocked()
	return true
}

// AwaitArmed waits until at least count timers are armed, and says false when ctx ends first. A test
// that steps code over fake time, whose goroutines arm count timers between steps, steps it only once
// they have: stepped sooner, the time moved to a later timer, and a timer armed after counted from
// that later time.
func (fake *Fake) AwaitArmed(ctx context.Context, count int) bool {
	for {
		fake.mutex.Lock()
		if len(fake.timers) >= count {
			fake.mutex.Unlock()
			return true
		}
		if fake.armed == nil {
			fake.armed = make(chan struct{})
		}
		armed := fake.armed
		fake.mutex.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-armed:
		}
	}
}

// Sleep moves the Fake's time forward by wait, as though the wait had passed, and returns at once:
// a test never waits in real time. A context already ended is its error, and the time stays.
func (fake *Fake) Sleep(ctx context.Context, wait time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sleep: %w", err)
	}
	fake.Advance(wait)
	return nil
}

// Set makes the Fake read moment from now on.
func (fake *Fake) Set(moment time.Time) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.now = moment
	fake.fireDueLocked()
}

// NewTimer returns a timer that fires when the Fake's time reaches wait from now: at once for a
// wait of zero or less, otherwise when an Advance, Set or Sleep moves the time past it.
func (fake *Fake) NewTimer(wait time.Duration) clock.Timer {
	timer := &fakeTimer{fake: fake, fired: make(chan time.Time, 1)}
	timer.Reset(wait)
	return timer
}

// fireDueLocked fires every armed timer whose time has come. The caller holds the mutex.
func (fake *Fake) fireDueLocked() {
	armed := fake.timers[:0]
	for _, timer := range fake.timers {
		if timer.firesAt.After(fake.now) {
			armed = append(armed, timer)
			continue
		}
		timer.fired <- fake.now // never blocks: drained at each Reset and Stop, and fired once
	}
	fake.timers = armed
}

// fakeTimer is a Fake's timer.
type fakeTimer struct {
	fake    *Fake
	fired   chan time.Time
	firesAt time.Time // guarded by fake.mutex
}

// C receives the Fake's time once it has moved past the timer's.
func (timer *fakeTimer) C() <-chan time.Time { return timer.fired }

// Stop keeps the timer from firing, and reports whether it stopped it before it fired.
func (timer *fakeTimer) Stop() bool {
	timer.fake.mutex.Lock()
	defer timer.fake.mutex.Unlock()
	return timer.disarmLocked()
}

// Reset starts the wait again, wait from the Fake's now; a wait already passed fires at once.
func (timer *fakeTimer) Reset(wait time.Duration) {
	timer.fake.mutex.Lock()
	defer timer.fake.mutex.Unlock()
	timer.disarmLocked()
	timer.firesAt = timer.fake.now.Add(wait)
	timer.fake.timers = append(timer.fake.timers, timer)
	if timer.fake.armed != nil {
		close(timer.fake.armed)
		timer.fake.armed = nil
	}
	timer.fake.fireDueLocked()
}

// disarmLocked takes the timer off the Fake's armed timers and drops a firing not yet received, as
// a time.Timer's Stop and Reset do, and reports whether it was armed.
func (timer *fakeTimer) disarmLocked() bool {
	select {
	case <-timer.fired:
	default:
	}
	index := slices.Index(timer.fake.timers, timer)
	if index < 0 {
		return false
	}
	timer.fake.timers = slices.Delete(timer.fake.timers, index, index+1)
	return true
}

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

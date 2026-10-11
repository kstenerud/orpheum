// SPDX-License-Identifier: AGPL-3.0-or-later

package clock

import (
	"context"
	"fmt"
	"time"
)

// Clock tells the current time, and waits. Code that needs the time takes a Clock, so that a test
// can pass a clocktest.Fake and decide what "now" is and how long a wait takes.
type Clock interface {
	// Now returns the current time on the server's clock.
	Now() time.Time
	// Sleep waits for wait to pass on this clock, or until ctx ends, whose error it returns.
	Sleep(ctx context.Context, wait time.Duration) error
	// NewTimer returns a timer that fires once wait has passed on this clock.
	NewTimer(wait time.Duration) Timer
}

// Timer fires once, when its wait has passed on the clock that made it, and can be stopped or
// started again with a new wait.
type Timer interface {
	// C receives the time once the wait has passed.
	C() <-chan time.Time
	// Stop keeps the timer from firing, and reports whether it stopped it before it fired.
	Stop() bool
	// Reset starts the wait again, wait from now, whether or not the timer fired or was stopped.
	Reset(wait time.Duration)
}

// System is the Clock backed by the operating system. It is the only production Clock.
type System struct{}

// Now returns the operating system's current time.
func (System) Now() time.Time {
	return time.Now()
}

// Sleep waits for wait of real time, or until ctx ends.
func (System) Sleep(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("sleep: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// NewTimer returns a timer on the wall clock.
func (System) NewTimer(wait time.Duration) Timer {
	return systemTimer{timer: time.NewTimer(wait)}
}

// systemTimer is a time.Timer as a Timer.
type systemTimer struct{ timer *time.Timer }

// C receives the time once the wait has passed.
func (timer systemTimer) C() <-chan time.Time { return timer.timer.C }

// Stop keeps the timer from firing, and reports whether it stopped it before it fired.
func (timer systemTimer) Stop() bool { return timer.timer.Stop() }

// Reset starts the wait again, wait from now.
func (timer systemTimer) Reset(wait time.Duration) { timer.timer.Reset(wait) }

// Compile-time proof that System satisfies Clock.
var _ Clock = System{}

// SPDX-License-Identifier: GPL-3.0-or-later

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

// Compile-time proof that System satisfies Clock.
var _ Clock = System{}

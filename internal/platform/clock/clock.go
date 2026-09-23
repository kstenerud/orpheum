// SPDX-License-Identifier: GPL-3.0-or-later

package clock

import "time"

// Clock tells the current time. Code that needs the time takes a Clock, so that a test can
// pass a clocktest.Fake and decide what "now" is.
type Clock interface {
	// Now returns the current time on the server's clock.
	Now() time.Time
}

// System is the Clock backed by the operating system. It is the only production Clock.
type System struct{}

// Now returns the operating system's current time.
func (System) Now() time.Time {
	return time.Now()
}

// Compile-time proof that System satisfies Clock.
var _ Clock = System{}

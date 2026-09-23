// SPDX-License-Identifier: GPL-3.0-or-later

package clocktest

import (
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Fake is a Clock whose time moves only when a test moves it. It is safe for concurrent use.
type Fake struct {
	mutex sync.Mutex
	now   time.Time
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
}

// Set makes the Fake read moment from now on.
func (fake *Fake) Set(moment time.Time) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.now = moment
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// window is the failure window these tests count in.
const window = 15 * time.Minute

// A key nobody touches again is dropped once it has no failure left in the window: a key is made
// from whatever name a sign-in types, so keys were kept for good and grew without bound (the RAM
// gap, 2026-10-20).
func TestFailureLimiter_DropsKeysWithNoFailureLeftInTheWindow(t *testing.T) {
	t.Parallel()
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	limiter := newFailureLimiter(fakeClock)
	for _, name := range []string{"account:a", "account:b", "network:2001:db8::/48"} {
		limiter.recordFailure(window, name)
	}
	fakeClock.Advance(window + sweepEvery)
	limiter.recordFailure(window, "account:c")
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	if len(limiter.failures) != 1 {
		t.Errorf("the limiter holds %d keys after the window; want only the one failing now", len(limiter.failures))
	}
}

// Full, the limiter refuses an attempt that needs a key it does not hold, and drops no count to make
// room, which would let a flood reset a person's lockout; one counted already goes on, and once the
// counts age out there is room again. Being full is told once a sweep (the owner, 2026-10-20).
func TestFailureLimiter_FullRefusesAnAttemptNeedingANewKeyAndKeepsEveryCount(t *testing.T) {
	t.Parallel()
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	limiter := newFailureLimiter(fakeClock)
	limiter.maxKeys = 2
	told := 0
	limiter.tellWhenFull(func() { told++ })
	for _, key := range []string{"account:a", "account:b"} {
		if _, isAllowed, _ := limiter.attempt(window, map[string]int64{key: 5}, nil); !isAllowed {
			t.Fatalf("an attempt on %s with room was refused", key)
		}
	}
	for range 2 {
		if _, isAllowed, _ := limiter.attempt(window, map[string]int64{"account:c": 5}, nil); isAllowed {
			t.Error("an attempt needing a new key was allowed with the limiter full")
		}
	}
	if !limiter.isLimited(5, window, "pairing-user:7") {
		t.Error("a key with no room to be counted is not limited")
	}
	if told != 1 {
		t.Errorf("being full was told %d times in one sweep; want once", told)
	}
	if _, isAllowed, _ := limiter.attempt(window, map[string]int64{"account:a": 5}, nil); !isAllowed {
		t.Error("an attempt on a key already counted was refused for the limiter being full")
	}
	limiter.mutex.Lock()
	held := len(limiter.failures)
	limiter.mutex.Unlock()
	if held != 2 {
		t.Errorf("the full limiter holds %d keys; want the 2 it had, none dropped", held)
	}
	fakeClock.Advance(window + sweepEvery)
	if _, isAllowed, _ := limiter.attempt(window, map[string]int64{"account:c": 5}, nil); !isAllowed {
		t.Error("an attempt was still refused after the counts aged out")
	}
}

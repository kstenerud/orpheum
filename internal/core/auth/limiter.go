// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// failureLimiter counts recent failures per key and refuses once a key reaches the limit, because
// each attempt costs argon2id's memory and is a memory-exhaustion lever as well as a guess
// (D194 §2). The limit and window are settings, passed in on each call. It is safe for
// concurrent use.
type failureLimiter struct {
	clock    clock.Clock
	mutex    sync.Mutex
	failures map[string][]time.Time // guarded by mutex
}

func newFailureLimiter(clockSource clock.Clock) *failureLimiter {
	return &failureLimiter{clock: clockSource, failures: map[string][]time.Time{}}
}

// isLimited reports whether any of keys has reached limit within window.
func (limiter *failureLimiter) isLimited(maxFailures int64, window time.Duration, keys ...string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	for _, key := range keys {
		if int64(len(limiter.recent(key, window))) >= maxFailures {
			return true
		}
	}
	return false
}

// recordFailure records one failure against each of keys.
func (limiter *failureLimiter) recordFailure(window time.Duration, keys ...string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.clock.Now()
	for _, key := range keys {
		limiter.failures[key] = append(limiter.recent(key, window), now)
	}
}

// forget clears the account's failures after a success; an address's are kept, so one good
// password does not reset guessing at other accounts from the same place.
func (limiter *failureLimiter) forget(key string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	delete(limiter.failures, key)
}

// recent returns key's failures still inside the window, dropping older ones. The caller holds
// the mutex.
func (limiter *failureLimiter) recent(key string, window time.Duration) []time.Time {
	cutoff := limiter.clock.Now().Add(-window)
	kept := limiter.failures[key][:0]
	for _, at := range limiter.failures[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(limiter.failures, key)
		return nil
	}
	limiter.failures[key] = kept
	return kept
}

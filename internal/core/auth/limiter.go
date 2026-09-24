// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Failed sign-ins are limited per account and per address, because each attempt costs 64 MiB
// of argon2id and is a memory-exhaustion lever as well as a guess (D194 §2). The numbers are
// provisional: D194 says to limit, not how much.
const (
	failureLimit  = 5
	failureWindow = 15 * time.Minute
)

// failureLimiter counts recent failures per key and refuses once a key reaches the limit. It is
// safe for concurrent use.
type failureLimiter struct {
	clock    clock.Clock
	mutex    sync.Mutex
	failures map[string][]time.Time // guarded by mutex
}

func newFailureLimiter(now clock.Clock) *failureLimiter {
	return &failureLimiter{clock: now, failures: map[string][]time.Time{}}
}

// isLimited reports whether any of keys has reached the limit within the window.
func (limiter *failureLimiter) isLimited(keys ...string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	for _, key := range keys {
		if len(limiter.recent(key)) >= failureLimit {
			return true
		}
	}
	return false
}

// fail records one failure against each of keys.
func (limiter *failureLimiter) fail(keys ...string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.clock.Now()
	for _, key := range keys {
		limiter.failures[key] = append(limiter.recent(key), now)
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
func (limiter *failureLimiter) recent(key string) []time.Time {
	cutoff := limiter.clock.Now().Add(-failureWindow)
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

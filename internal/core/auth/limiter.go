// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"slices"
	"strings"
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

// attempt takes one attempt against each key of limits, refusing it, and recording nothing, when any
// key has reached its limit within window. An attempt is recorded as a failure before it is tried,
// so attempts made at once each count: checking the limit and recording after the slow hash let 53
// of 60 parallel guesses through (the second audit's K2, 2026-10-13). A success withdraws it.
func (limiter *failureLimiter) attempt(window time.Duration, limits map[string]int64) (time.Time, bool) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	for key, most := range limits {
		if int64(len(limiter.recent(key, window))) >= most {
			return time.Time{}, false
		}
	}
	now := limiter.clock.Now()
	for key := range limits {
		limiter.failures[key] = append(limiter.recent(key, window), now)
	}
	return now, true
}

// withdraw takes back the attempt recorded at attemptedAt against each of keys: it succeeded.
func (limiter *failureLimiter) withdraw(attemptedAt time.Time, keys ...string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	for _, key := range keys {
		kept := limiter.failures[key]
		for index, when := range slices.Backward(kept) {
			if when.Equal(attemptedAt) {
				limiter.failures[key] = slices.Delete(kept, index, index+1)
				break
			}
		}
		if len(limiter.failures[key]) == 0 {
			delete(limiter.failures, key)
		}
	}
}

// forgetPrefixed clears every key that is prefix or begins with it followed by "@": a person's
// failures everywhere, account-wide and per network.
func (limiter *failureLimiter) forgetPrefixed(prefix string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	for key := range limiter.failures {
		if key == prefix || strings.HasPrefix(key, prefix+"@") {
			delete(limiter.failures, key)
		}
	}
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

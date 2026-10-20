// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// sweepEvery is how often the limiter drops the keys with no failure left in the window: a key is
// made from whatever name a sign-in types, so keys nobody touches again would otherwise be kept for
// good, and grow without bound across networks (the RAM gap, 2026-10-20).
const sweepEvery = time.Minute

// maxLimiterKeys is how many keys the limiter holds at most. Each network is held to its own limit
// per window, so only an attack from thousands of networks at once reaches it; then an attempt that
// needs a new key is refused, not let in uncounted, and no count is dropped to make room, which
// would let a flood reset a person's lockout (the owner, 2026-10-20).
const maxLimiterKeys = 50_000

// failureLimiter counts recent failures per key and refuses once a key reaches the limit, because
// each attempt costs argon2id's memory and is a memory-exhaustion lever as well as a guess
// (D194 §2). The limit and window are settings, passed in on each call. It is safe for
// concurrent use.
type failureLimiter struct {
	clock clock.Clock
	// maxKeys is maxLimiterKeys, fewer in the tests.
	maxKeys int
	mutex   sync.Mutex
	// Guarded by mutex: failures are each key's recent failures, sweptAt when idle keys were last
	// dropped, and onFull is told, once a sweep at most, that an attempt was refused for want of room.
	failures   map[string][]time.Time
	sweptAt    time.Time
	toldFullAt time.Time
	onFull     func()
}

func newFailureLimiter(clockSource clock.Clock) *failureLimiter {
	return &failureLimiter{clock: clockSource, maxKeys: maxLimiterKeys, failures: map[string][]time.Time{}}
}

// tellWhenFull has onFull told when an attempt is refused for want of room, once a sweep at most.
func (limiter *failureLimiter) tellWhenFull(onFull func()) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.onFull = onFull
}

// sweep drops every key with no failure left in window, at most once a sweepEvery. The caller holds
// the mutex.
func (limiter *failureLimiter) sweep(window time.Duration) {
	now := limiter.clock.Now()
	if now.Sub(limiter.sweptAt) < sweepEvery {
		return
	}
	limiter.sweptAt = now
	for key := range limiter.failures {
		limiter.recent(key, window)
	}
}

// hasRoomFor is whether the keys among keys not held yet fit under maxKeys; when they do not, onFull
// is told, once a sweep at most. The caller holds the mutex.
func (limiter *failureLimiter) hasRoomFor(keys []string) bool {
	added := 0
	for _, key := range keys {
		if _, isHeld := limiter.failures[key]; !isHeld {
			added++
		}
	}
	if len(limiter.failures)+added <= limiter.maxKeys {
		return true
	}
	if limiter.onFull != nil && !limiter.toldFullAt.Equal(limiter.sweptAt) {
		limiter.toldFullAt = limiter.sweptAt
		limiter.onFull()
	}
	return false
}

// isLimited reports whether any of keys has reached limit within window, or has no room to be counted
// (hasRoomFor).
func (limiter *failureLimiter) isLimited(maxFailures int64, window time.Duration, keys ...string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.sweep(window)
	if !limiter.hasRoomFor(keys) {
		return true
	}
	for _, key := range keys {
		if int64(len(limiter.recent(key, window))) >= maxFailures {
			return true
		}
	}
	return false
}

// attempt takes one attempt against each key of refusing and of pacing, refusing it, and recording
// nothing, when a key of refusing has reached its limit within window; one past a limit of pacing
// is allowed, and said to be paced, for the caller to slow. An attempt is recorded as a failure
// before it is tried, so attempts made at once each count: checking the limit and recording after
// the slow hash let 53 of 60 parallel guesses through (the second audit's K2, 2026-10-13). A
// success withdraws it.
func (limiter *failureLimiter) attempt(
	window time.Duration, refusing, pacing map[string]int64,
) (attemptedAt time.Time, isAllowed, isPaced bool) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.sweep(window)
	for key, most := range refusing {
		if int64(len(limiter.recent(key, window))) >= most {
			return time.Time{}, false, false
		}
	}
	if !limiter.hasRoomFor(append(slices.Collect(maps.Keys(refusing)), slices.Collect(maps.Keys(pacing))...)) {
		return time.Time{}, false, false
	}
	for key, most := range pacing {
		isPaced = isPaced || int64(len(limiter.recent(key, window))) >= most
	}
	now := limiter.clock.Now()
	for _, limits := range []map[string]int64{refusing, pacing} {
		for key := range limits {
			limiter.failures[key] = append(limiter.recent(key, window), now)
		}
	}
	return now, true, isPaced
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

// recordFailure records one failure against each of keys; with no room for them, none (hasRoomFor).
func (limiter *failureLimiter) recordFailure(window time.Duration, keys ...string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.sweep(window)
	if !limiter.hasRoomFor(keys) {
		return
	}
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

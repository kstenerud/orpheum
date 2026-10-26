// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx

import (
	"sync"
	"time"
)

// FailingSaidEvery is how often a lasting failure is said again: once a minute, with how many times
// it failed since, is a log a person reading it during an outage can follow, where a loop that
// ticks each second said it 60 times a minute (the audit of 2026-10-25, C12). It is no setting:
// it changes what the log says of a failure, never what the server does.
const FailingSaidEvery = time.Minute

// Failing is a loop's lasting failure, as its log says it: the first failure at once, then once each
// FailingSaidEvery with how often it failed since it began, then its end. The loop logs with its
// own constant message; Failing says when, and with what attributes. Safe for concurrent use.
type Failing struct {
	mutex sync.Mutex
	// since is when the failure began, zero while none is going on; saidAt is when it was last
	// said; failures is how many times it failed since it began.
	since, saidAt time.Time
	failures      int
}

// Fail records a failure at now, and says whether the loop logs it, with the attributes that say
// how long it has lasted and how often it failed, for every failure but the first.
func (failing *Failing) Fail(now time.Time) (isSaid bool, attributes []any) {
	failing.mutex.Lock()
	defer failing.mutex.Unlock()
	failing.failures++
	if failing.since.IsZero() {
		failing.since, failing.saidAt = now, now
		return true, nil
	}
	if now.Sub(failing.saidAt) < FailingSaidEvery {
		return false, nil
	}
	failing.saidAt = now
	return true, []any{"failingSince", failing.since, "failureCount", failing.failures}
}

// Succeed records a success at now, and says whether it ends a failure the loop logged, with the
// attributes that say how long it lasted and how often it failed.
func (failing *Failing) Succeed(now time.Time) (isEnded bool, attributes []any) {
	failing.mutex.Lock()
	defer failing.mutex.Unlock()
	if failing.since.IsZero() {
		return false, nil
	}
	attributes = []any{"failedForMs", now.Sub(failing.since).Milliseconds(), "failureCount", failing.failures}
	failing.since, failing.saidAt, failing.failures = time.Time{}, time.Time{}, 0
	return true, attributes
}

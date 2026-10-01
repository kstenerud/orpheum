// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"runtime"
	"testing"
	"time"
)

// ReceiveLimit is how long Receive waits before it fails the test: far beyond any wait a passing
// test has, even under the race detector on a loaded machine, and far short of the test binary's
// own timeout, which ends a hung run with no word of what it was waiting for.
const ReceiveLimit = time.Minute

// Receive takes the next value from a channel, or fails the test once ReceiveLimit has passed,
// naming what it waited for (waitingFor: "the node's prepare command") and showing every
// goroutine's stack, so a hang says where it is stuck. Call it from the test's own goroutine; a
// goroutine the test started waits with a select on its context instead.
func Receive[Value any](tb testing.TB, from <-chan Value, waitingFor string) Value {
	tb.Helper()
	return receiveWithin(tb, from, waitingFor, ReceiveLimit)
}

func receiveWithin[Value any](tb testing.TB, from <-chan Value, waitingFor string, limit time.Duration) Value {
	tb.Helper()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case value := <-from:
		return value
	case <-timer.C:
		tb.Fatalf("waited %s for %s, and it never came; every goroutine:\n%s", limit, waitingFor, allStacks())
		var zero Value
		return zero
	}
}

// allStacks is every goroutine's stack, as a panic would print them.
func allStacks() string {
	stacks := make([]byte, 1<<20)
	return string(stacks[:runtime.Stack(stacks, true)])
}

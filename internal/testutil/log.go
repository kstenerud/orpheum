// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
)

// LogLimit is how long AwaitLine waits for a line before it fails the test: a failure bound only,
// since AwaitLine returns as soon as a matching line is written, so a passing test never waits for
// it. At ReceiveLimit's scale (test-architecture-2026-10-26.md finding 3): 5 s failed passing code
// under load.
const LogLimit = ReceiveLimit

// Log is a logger's output a test reads while the code under test still writes to it, from its own
// goroutines: a Watch Together device's disconnection, a node's run of chunks. Hand it to
// slog.NewTextHandler or slog.NewJSONHandler; the zero Log is ready.
type Log struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
	// wrote is signalled, without blocking, after each write: what AwaitLine waits on.
	wrote chan struct{}
}

func (log *Log) Write(written []byte) (int, error) {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	select {
	case log.wroteLocked() <- struct{}{}:
	default:
	}
	return log.buffer.Write(written)
}

// wroteLocked is the channel each write signals, made at first use.
func (log *Log) wroteLocked() chan struct{} {
	if log.wrote == nil {
		log.wrote = make(chan struct{}, 1)
	}
	return log.wrote
}

// AwaitLine waits up to LogLimit for a line holding every one of parts, and returns it; it fails
// the test, showing the whole log, when none comes.
func (log *Log) AwaitLine(tb testing.TB, parts ...string) string {
	tb.Helper()
	ctx, cancel := context.WithTimeout(tb.Context(), LogLimit)
	defer cancel()
	log.mutex.Lock()
	wrote := log.wroteLocked()
	log.mutex.Unlock()
	for {
		for line := range strings.Lines(log.String()) {
			if !slices.ContainsFunc(parts, func(part string) bool { return !strings.Contains(line, part) }) {
				return line
			}
		}
		select {
		case <-wrote:
		case <-ctx.Done():
			tb.Fatalf("no log line holds %q:\n%s", parts, log.String())
		}
	}
}

func (log *Log) String() string {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	return log.buffer.String()
}

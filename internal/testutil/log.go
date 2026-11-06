// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
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
	return log.await(tb, fmt.Sprintf("holds %q", parts), func(logged string) []string {
		var lines []string
		for line := range strings.Lines(logged) {
			if holdsEvery(line, parts) {
				lines = append(lines, line)
			}
		}
		return lines
	})
}

// await waits up to LogLimit for matching to find a line in the log, and returns the first; it
// fails the test, saying which line it wanted and showing the whole log, when none comes.
func (log *Log) await(tb testing.TB, wanted string, matching func(logged string) []string) string {
	tb.Helper()
	ctx, cancel := context.WithTimeout(tb.Context(), LogLimit)
	defer cancel()
	log.mutex.Lock()
	wrote := log.wroteLocked()
	log.mutex.Unlock()
	for {
		if lines := matching(log.String()); len(lines) > 0 {
			return lines[0]
		}
		select {
		case <-wrote:
		case <-ctx.Done():
			tb.Fatalf("no log line %s:\n%s", wanted, log.String())
		}
	}
}

func (log *Log) String() string {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	return log.buffer.String()
}

// AwaitEvent waits up to LogLimit for a line whose slogx.EventKey attribute is event and that
// holds every one of parts, and returns it; it fails the test, showing the whole log, when none
// comes. It matches a line by its event, not its message's English, so rewording the message does
// not break the test.
func (log *Log) AwaitEvent(tb testing.TB, event string, parts ...string) string {
	tb.Helper()
	return log.await(tb, fmt.Sprintf("with %s=%s holds %q", slogx.EventKey, event, parts),
		func(logged string) []string { return linesWith(logged, event, parts) })
}

// LineWith is the first line of logged whose slogx.EventKey attribute is event; it fails the test,
// showing logged, when there is none. For a log the code under test has finished writing; one it
// still writes to is a Log, and its AwaitEvent.
func LineWith(tb testing.TB, logged, event string) string {
	tb.Helper()
	lines := LinesWith(logged, event)
	if len(lines) == 0 {
		tb.Fatalf("no log line with %s=%s:\n%s", slogx.EventKey, event, logged)
		return ""
	}
	return lines[0]
}

// LinesWith is every line of logged whose slogx.EventKey attribute is event, in the order logged:
// how a test counts a line's repeats, or says it was not logged.
func LinesWith(logged, event string) []string {
	return linesWith(logged, event, nil)
}

// linesWith is every line of logged whose event is event and that holds every one of parts. It
// reads both of slogx's formats: event=value in text, "event":"value" in JSON.
func linesWith(logged, event string, parts []string) []string {
	text, json := slogx.EventKey+"="+event, `"`+slogx.EventKey+`":"`+event+`"`
	var lines []string
	for line := range strings.Lines(logged) {
		trimmed := strings.TrimSuffix(line, "\n")
		isEvent := strings.Contains(trimmed, json) || strings.HasSuffix(trimmed, " "+text) ||
			strings.HasPrefix(trimmed, text+" ") || strings.Contains(trimmed, " "+text+" ")
		if isEvent && holdsEvery(line, parts) {
			lines = append(lines, line)
		}
	}
	return lines
}

// holdsEvery says whether line holds every one of parts.
func holdsEvery(line string, parts []string) bool {
	return !slices.ContainsFunc(parts, func(part string) bool { return !strings.Contains(line, part) })
}

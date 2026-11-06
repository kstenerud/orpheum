// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// LinesWith finds a line by its event in either of slogx's formats, wherever the attribute stands and
// whatever the message says, and not by an event whose name only begins with the one asked for.
func TestLinesWith_MatchesTheEventInTextAndJSONNotItsMessage(t *testing.T) {
	t.Parallel()
	for name, handler := range map[string]func(*bytes.Buffer) slog.Handler{
		"text": func(buffer *bytes.Buffer) slog.Handler { return slog.NewTextHandler(buffer, nil) },
		"json": func(buffer *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(buffer, nil) },
	} {
		var buffer bytes.Buffer
		logger := slog.New(handler(&buffer))
		logger.Info("a run started", slogx.EventKey, "runStartedAgain", "jobId", 1)
		logger.Info("a run started", slogx.EventKey, "runStarted", "jobId", 2)
		logger.Info("a run finished", "jobId", 3, slogx.EventKey, "runStarted")
		logger.Info("runStarted", "jobId", 4)
		logged := buffer.String()
		isJob := func(line, id string) bool {
			return strings.Contains(line, "jobId="+id) || strings.Contains(line, `"jobId":`+id)
		}
		lines := LinesWith(logged, "runStarted")
		if len(lines) != 2 || !isJob(lines[0], "2") || !isJob(lines[1], "3") {
			t.Errorf("%s: LinesWith(runStarted) = %q; want jobs 2 and 3's lines, of\n%s", name, lines, logged)
		}
		if line := LineWith(t, logged, "runStartedAgain"); !isJob(line, "1") {
			t.Errorf("%s: LineWith(runStartedAgain) = %q; want job 1's line", name, line)
		}
	}
}

// AwaitEvent returns the line with the event and every part once something writes it, from another
// goroutine.
func TestLog_AwaitEventWaitsForALineWithTheEventHoldingEveryPart(t *testing.T) {
	t.Parallel()
	log := &Log{}
	logger := slog.New(slog.NewTextHandler(log, nil))
	logger.Info("a run finished", slogx.EventKey, "runFinished", "jobId", 2)
	go func() {
		logger.Info("a run finished", slogx.EventKey, "runStarted", "jobId", 1)
		logger.Info("a run finished", slogx.EventKey, "runFinished", "jobId", 1)
	}()
	if line := log.AwaitEvent(t, "runFinished", "jobId=1"); !strings.HasSuffix(line, "logEvent=runFinished jobId=1\n") {
		t.Errorf("AwaitEvent = %q; want the finished line of job 1", line)
	}
}

// AwaitLine returns the line holding every part once something writes it, from another goroutine.
func TestLog_AwaitLineWaitsForALineHoldingEveryPart(t *testing.T) {
	t.Parallel()
	log := &Log{}
	writeLine := func(line string) {
		if _, err := fmt.Fprintln(log, line); err != nil {
			t.Error(err)
		}
	}
	writeLine("a run started jobId=1")
	go func() {
		writeLine("a run finished jobId=2")
		writeLine("a run finished jobId=1 speed=3.3")
	}()
	if line := log.AwaitLine(t, "a run finished", "jobId=1"); line != "a run finished jobId=1 speed=3.3\n" {
		t.Errorf("AwaitLine = %q; want the finished line of job 1", line)
	}
}

// LogLimit is a failure bound only: AwaitLine returns the moment a matching line is written, so a
// passing test never waits for it. At 5 s it failed passing code under load (test architecture
// review, 2026-10-26, finding 3), so it is at ReceiveLimit's scale, not a tighter one.
func TestLogLimit_IsAtReceiveLimitsScale(t *testing.T) {
	t.Parallel()
	if LogLimit != ReceiveLimit {
		t.Errorf("LogLimit = %v, want ReceiveLimit (%v)", LogLimit, ReceiveLimit)
	}
}

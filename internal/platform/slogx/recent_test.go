// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// recentStart is the time the recent log's tests log from.
var recentStart = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

// keepingLogger is a text logger writing at info into output, keeping its lines in a recent log
// of limits.
func keepingLogger(t *testing.T, limits slogx.RecentLimits) (*slog.Logger, *slogx.Recent, *bytes.Buffer) {
	t.Helper()
	recent := slogx.NewRecent(limits)
	var output bytes.Buffer
	logger, err := slogx.NewKeeping(&output, slogx.FormatText, slog.LevelInfo, recent)
	if err != nil {
		t.Fatalf("NewKeeping: %v", err)
	}
	return logger, recent, &output
}

// logAt logs message at level, as of at, through logger's handler.
func logAt(t *testing.T, logger *slog.Logger, at time.Time, level slog.Level, message string) {
	t.Helper()
	record := slog.NewRecord(at, level, message, 0)
	if err := logger.Handler().Handle(t.Context(), record); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

// messages are lines' messages, in order.
func messages(lines []slogx.Line) []string {
	said := make([]string, 0, len(lines))
	for _, line := range lines {
		said = append(said, line.Message)
	}
	return said
}

func TestRecent_KeepsALineBelowTheLevelItDoesNotWrite(t *testing.T) {
	t.Parallel()
	logger, recent, output := keepingLogger(t, slogx.ProcessRecentLimits())
	logAt(t, logger, recentStart, slog.LevelDebug, "a detail")
	logAt(t, logger, recentStart.Add(time.Second), slog.LevelInfo, "a step")

	if got := strings.Join(messages(recent.Lines(recentStart.Add(time.Minute))), "|"); got != "a detail|a step" {
		t.Errorf("kept %q; want both lines, the debug one too", got)
	}
	if strings.Contains(output.String(), "a detail") || !strings.Contains(output.String(), "a step") {
		t.Errorf("wrote %q; want the info line alone, the level still info", output.String())
	}
}

func TestRecent_KeepsEveryLineHalfAnHourAndThoseAtInfoADay(t *testing.T) {
	t.Parallel()
	logger, recent, _ := keepingLogger(t, slogx.ProcessRecentLimits())
	logAt(t, logger, recentStart, slog.LevelDebug, "an old detail")
	logAt(t, logger, recentStart, slog.LevelWarn, "an old warning")
	logAt(t, logger, recentStart.Add(40*time.Minute), slog.LevelDebug, "a new detail")

	got := strings.Join(messages(recent.Lines(recentStart.Add(41*time.Minute))), "|")
	if got != "an old warning|a new detail" {
		t.Errorf("after 41 minutes kept %q; want the warning and the new detail, the old detail gone", got)
	}
	if got := messages(recent.Lines(recentStart.Add(25 * time.Hour))); len(got) != 0 {
		t.Errorf("after 25 hours kept %q; want nothing", got)
	}
}

func TestRecent_KeepsNoMoreThanItsCountOfEachKind(t *testing.T) {
	t.Parallel()
	limits := slogx.RecentLimits{Fine: time.Hour, Kept: time.Hour, FineCount: 2, KeptCount: 1}
	logger, recent, _ := keepingLogger(t, limits)
	for index, message := range []string{"detail 1", "detail 2", "detail 3"} {
		logAt(t, logger, recentStart.Add(time.Duration(index)*time.Second), slog.LevelDebug, message)
	}
	logAt(t, logger, recentStart, slog.LevelError, "failure 1")
	logAt(t, logger, recentStart.Add(5*time.Second), slog.LevelError, "failure 2")

	got := strings.Join(messages(recent.Lines(recentStart.Add(time.Minute))), "|")
	if got != "detail 2|detail 3|failure 2" {
		t.Errorf("kept %q; want the last two details and the last failure, in time order", got)
	}
}

// secretValue logs as a secret does, by a LogValuer.
type secretValue struct{}

func (secretValue) LogValue() slog.Value { return slog.StringValue("[redacted]") }

func TestRecent_ALineHasTheLoggersFieldsItsGroupsAndItsRequestsID(t *testing.T) {
	t.Parallel()
	forever := time.Duration(1 << 62) // the line is logged at the clock's time, which the test does not set
	logger, recent, _ := keepingLogger(t, slogx.RecentLimits{Fine: forever, Kept: forever, FineCount: 1, KeptCount: 1})
	logger = logger.With("component", "scanner").WithGroup("job")
	ctx := slogx.WithRequestID(t.Context(), "REQUEST1")
	logger.DebugContext(ctx, "examined a file",
		"fileId", 7, slog.Group("probe", "codec", "hevc"), "token", secretValue{})

	lines := recent.Lines(recentStart)
	if len(lines) != 1 {
		t.Fatalf("kept %d lines; want 1", len(lines))
	}
	var got []string
	for _, field := range lines[0].Fields {
		got = append(got, field.Key+"="+field.Value.String())
	}
	want := "component=scanner job.fileId=7 job.probe.codec=hevc job.token=[redacted] requestId=REQUEST1"
	if strings.Join(got, " ") != want {
		t.Errorf("fields %q; want %q", strings.Join(got, " "), want)
	}
}

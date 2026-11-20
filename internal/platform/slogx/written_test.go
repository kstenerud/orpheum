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

// A written log reads back as the recent log its logger kept: each line's time, level, message
// and fields in order, a JSON line's values of their kinds and a text line's as strings, bounded as
// a line logged is, measured from the newest line; a line that is none of the format's is counted
// unread (D524 §5).
func TestRecent_KeepsWhatALoggerWroteAsItKeptIt(t *testing.T) {
	t.Parallel()
	for _, format := range []slogx.Format{slogx.FormatJSON, slogx.FormatText} {
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			logged, output := writeEachKind(t, format)
			output.WriteString("a line cut short {\"time\n")

			read := slogx.NewRecent(slogx.ProcessRecentLimits())
			newest, unread, err := read.KeepWritten(output, format)
			if err != nil || unread != 1 || !newest.Equal(recentStart.Add(2*time.Second)) {
				t.Fatalf("KeepWritten = %v, %d unread, %v; want the newest line's time, the cut line unread",
					newest, unread, err)
			}
			want, got := logged.Lines(newest), read.Lines(newest)
			if len(want) != 3 || len(got) != len(want) {
				t.Fatalf("read %d lines; want the %d logged within the day before the newest:\n%v", len(got),
					len(want), got)
			}
			for index := range want {
				sameLine(t, format, index, got[index], want[index])
			}
			if strings.Contains(strings.Join(messages(got), "|"), "a day before") {
				t.Errorf("read %v; want the line a day before the newest left out", messages(got))
			}
		})
	}
}

// writeEachKind logs, in format, a warning a day before recentStart, then a line at each of three
// levels with a field of each kind and a group, and returns what was kept and what was written.
func writeEachKind(t *testing.T, format slogx.Format) (*slogx.Recent, *bytes.Buffer) {
	t.Helper()
	logged := slogx.NewRecent(slogx.ProcessRecentLimits())
	var output bytes.Buffer
	logger, err := slogx.NewKeeping(&output, format, slog.LevelDebug, logged)
	if err != nil {
		t.Fatal(err)
	}
	logger = logger.With("component", "core")
	logAt(t, logger, recentStart.Add(-25*time.Hour), slog.LevelWarn, "a warning a day before")
	for at, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelError} {
		record := slog.NewRecord(recentStart.Add(time.Duration(at)*time.Second), level, "a step", 0)
		record.AddAttrs(slog.String("path", `/srv/a b/"c".mkv`), slog.Int("count", at),
			slog.Bool("ok", true), slog.Float64("rate", 1.5), slog.Group("request", slog.String("id", "r1")))
		if err = logger.Handler().Handle(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	return logged, &output
}

// sameLine fails unless got, read back, is want as logged: a text line's values as strings.
func sameLine(t *testing.T, format slogx.Format, index int, got, want slogx.Line) {
	t.Helper()
	if !got.Time.Equal(want.Time) || got.Level != want.Level || got.Message != want.Message {
		t.Errorf("line %d reads %v; want %v", index, got, want)
	}
	if len(got.Fields) != len(want.Fields) {
		t.Fatalf("line %d's fields read %v; want %v", index, got.Fields, want.Fields)
	}
	for at, field := range want.Fields {
		wanted := field.Value
		if format == slogx.FormatText {
			wanted = slog.StringValue(field.Value.String())
		}
		if read := got.Fields[at]; read.Key != field.Key || !read.Value.Equal(wanted) {
			t.Errorf("line %d's field %d reads %s=%v (%s); want %s=%v (%s)", index, at, read.Key, read.Value,
				read.Value.Kind(), field.Key, wanted, wanted.Kind())
		}
	}
}

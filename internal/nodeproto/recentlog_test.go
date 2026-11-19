// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"log/slog"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// A line the node sent reads at the core with each value of the kind it was logged as, so a report
// keeps a number and pseudonymises a string (D524); a value that does not read as its kind is kept as
// the string it is.
func TestLogLine_ReadsAtTheCoreWithEachValuesKind(t *testing.T) {
	t.Parallel()
	loggedAt := time.Date(2026, 11, 19, 12, 0, 0, 5, time.UTC)
	sent := LogLine{Time: loggedAt, Level: int(slog.LevelWarn), Message: "a step", Fields: []LogField{
		{Key: "count", Kind: LogValueInt, Value: "-3"},
		{Key: "bytes", Kind: LogValueUint, Value: "9223372036854775808"},
		{Key: "ratio", Kind: LogValueFloat, Value: "0.25"},
		{Key: "isDone", Kind: LogValueBool, Value: "true"},
		{Key: "took", Kind: LogValueDuration, Value: "1500000000"},
		{Key: "loggedAt", Kind: LogValueTime, Value: "2026-11-19T12:00:00.000000005Z"},
		{Key: "path", Kind: LogValueString, Value: "/media/films"},
		{Key: "garbled", Kind: LogValueInt, Value: "x"},
	}}
	want := []slog.Value{
		slog.Int64Value(-3), slog.Uint64Value(1 << 63), slog.Float64Value(0.25), slog.BoolValue(true),
		slog.DurationValue(1500 * time.Millisecond), slog.TimeValue(loggedAt), slog.StringValue("/media/films"),
		slog.StringValue("x"),
	}
	read := sent.Line()
	if !read.Time.Equal(loggedAt) || read.Level != slog.LevelWarn || read.Message != "a step" ||
		len(read.Fields) != len(want) {
		t.Fatalf("read %+v; want the warning a step with %d fields", read, len(want))
	}
	for index, field := range read.Fields {
		if field.Key != sent.Fields[index].Key || !field.Value.Equal(want[index]) {
			t.Errorf("field %s read %s %q; want %s %q", field.Key, field.Value.Kind(), field.Value.String(),
				want[index].Kind(), want[index].String())
		}
	}
}

// A line added to a recent log reads back as it was logged, each value of its kind, whether the node
// sends it or the core keeps its own in a problem report (D524 §3).
func TestRecentLog_AnAddedLineReadsBackAsItWasLogged(t *testing.T) {
	t.Parallel()
	loggedAt := time.Date(2026, 11, 19, 12, 0, 0, 5, time.UTC)
	logged := []slog.Value{
		slog.Int64Value(-3), slog.Uint64Value(1 << 63), slog.Float64Value(0.25), slog.BoolValue(true),
		slog.DurationValue(1500 * time.Millisecond), slog.TimeValue(loggedAt), slog.StringValue("/media/films"),
		slog.AnyValue([]int{1, 2}),
	}
	line := slogx.Line{Time: loggedAt, Level: slog.LevelDebug, Message: "a detail"}
	for index, value := range logged {
		line.Fields = append(line.Fields, slogx.Field{Key: "field" + strconv.Itoa(index), Value: value})
	}
	var log RecentLog
	log.Add(line)
	read := log.SlogLines()
	if len(read) != 1 || !read[0].Time.Equal(loggedAt) || read[0].Level != slog.LevelDebug ||
		read[0].Message != "a detail" || len(read[0].Fields) != len(logged) {
		t.Fatalf("read %+v; want the one debug line with %d fields", read, len(logged))
	}
	// A value of no kind the wire knows reads back as the string it was written as.
	want := slices.Concat(logged[:len(logged)-1], []slog.Value{slog.StringValue("[1 2]")})
	for index, field := range read[0].Fields {
		if field.Key != line.Fields[index].Key || !field.Value.Equal(want[index]) {
			t.Errorf("field %s read %s %q; want %s %q", field.Key, field.Value.Kind(), field.Value.String(),
				want[index].Kind(), want[index].String())
		}
	}
}

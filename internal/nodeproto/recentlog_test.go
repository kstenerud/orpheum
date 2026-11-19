// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"log/slog"
	"testing"
	"time"
)

// A line the node sent reads loggedAt the core with each value of the kind it was logged as, so a report
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

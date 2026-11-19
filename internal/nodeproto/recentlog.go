// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// PathBytesRecentLog is the node byte socket's route for the node's recent log, every line it kept
// in memory whatever level it writes, for a problem report (D524 §3). It is answered outside every
// queue, as a probe is.
const PathBytesRecentLog = "/bytes/v1/recent-log"

// RecentLog is a node's answer on PathBytesRecentLog: its recent lines, the oldest first. Its
// version is its hello's.
type RecentLog struct {
	Lines []LogLine `json:"lines"`
}

// LogLine is one line of a node's recent log: when, its level as slog numbers it (debug -4, info 0,
// warn 4, error 8), its constant message and its fields.
type LogLine struct {
	Time    time.Time  `json:"time"`
	Level   int        `json:"level"`
	Message string     `json:"message"`
	Fields  []LogField `json:"fields,omitempty"`
}

// LogField is one field of a LogLine: its key, dotted by its groups, its value's kind and the value
// written out (a number in decimal, a duration in nanoseconds, a time in RFC 3339 with nanoseconds,
// a boolean as true or false).
type LogField struct {
	Key   string       `json:"key"`
	Kind  LogValueKind `json:"kind"`
	Value string       `json:"value"`
}

// LogValueKind is what a LogField's value is, so the core reads a number as a number: a report keeps
// a number and pseudonymises a string (D524). The set is closed.
type LogValueKind string

// The kinds of a LogField's value; any other slog kind is written as a string.
const (
	LogValueString   LogValueKind = "string"
	LogValueInt      LogValueKind = "int"
	LogValueUint     LogValueKind = "uint"
	LogValueFloat    LogValueKind = "float"
	LogValueBool     LogValueKind = "bool"
	LogValueDuration LogValueKind = "duration"
	LogValueTime     LogValueKind = "time"
)

// Line is the line as the node logged it, its values of the kinds they were; a value that does not
// read as its kind says is kept as the string it is.
func (line LogLine) Line() slogx.Line {
	fields := make([]slogx.Field, 0, len(line.Fields))
	for _, field := range line.Fields {
		fields = append(fields, slogx.Field{Key: field.Key, Value: field.value()})
	}
	return slogx.Line{Time: line.Time, Level: slog.Level(line.Level), Message: line.Message, Fields: fields}
}

// value is the field's value of its kind.
func (field LogField) value() slog.Value {
	switch field.Kind {
	case LogValueInt:
		if parsed, err := strconv.ParseInt(field.Value, 10, 64); err == nil {
			return slog.Int64Value(parsed)
		}
	case LogValueUint:
		if parsed, err := strconv.ParseUint(field.Value, 10, 64); err == nil {
			return slog.Uint64Value(parsed)
		}
	case LogValueFloat:
		if parsed, err := strconv.ParseFloat(field.Value, 64); err == nil {
			return slog.Float64Value(parsed)
		}
	case LogValueBool:
		if parsed, err := strconv.ParseBool(field.Value); err == nil {
			return slog.BoolValue(parsed)
		}
	case LogValueDuration:
		if parsed, err := strconv.ParseInt(field.Value, 10, 64); err == nil {
			return slog.DurationValue(time.Duration(parsed))
		}
	case LogValueTime:
		if parsed, err := time.Parse(time.RFC3339Nano, field.Value); err == nil {
			return slog.TimeValue(parsed)
		}
	case LogValueString:
	}
	return slog.StringValue(field.Value)
}

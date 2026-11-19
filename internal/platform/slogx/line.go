// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx

import (
	"log/slog"
	"time"
)

// Line is one record as the process's recent log keeps it (D524 §3), for a problem report to
// read: its message, constant at every call (sloglint), and its fields, which are where a film, a
// person or a house can be named.
type Line struct {
	Time    time.Time
	Level   slog.Level
	Message string
	// Fields are the record's attributes, the logger's own (With) first, in the order logged, each
	// group's name joined to its keys by a dot.
	Fields []Field
}

// Field is one attribute of a Line. Its Value is resolved (a LogValuer has given its value, so a
// secrets.Value is already "[redacted]") and never a group: a group's attributes are fields of
// their own.
type Field struct {
	Key   string
	Value slog.Value
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// RecentLimits bound what a Recent keeps: every line for Fine, a line at info or above for Kept,
// and no more than the counts of either, the oldest going first.
type RecentLimits struct {
	// Fine is how long a line below info is kept (D524 §3: 30 minutes).
	Fine time.Duration
	// Kept is how long a line at info or above is kept (D524 §3: 24 hours).
	Kept time.Duration
	// FineCount and KeptCount are the most lines of each kind kept, whatever their age.
	FineCount, KeptCount int
}

// ProcessRecentLimits are D524 §3's: 30 minutes of every line and 24 hours of those at info and
// above, each bounded by a count a busy hour's log stays under.
func ProcessRecentLimits() RecentLimits {
	return RecentLimits{Fine: 30 * time.Minute, Kept: 24 * time.Hour, FineCount: 20000, KeptCount: 20000}
}

// Recent is a process's recent log, kept in memory whatever level the log is written at, for a
// problem report to read (D524 §3). It is lost when the process ends. Safe for concurrent use.
type Recent struct {
	limits RecentLimits
	mutex  sync.Mutex
	// fine and kept are in the order logged: lines below info, and at info or above.
	fine, kept []Line
}

// NewRecent is an empty recent log keeping what limits allow.
func NewRecent(limits RecentLimits) *Recent {
	return &Recent{limits: limits}
}

// Lines are the lines kept as of now, the oldest first; a line older than its kind is kept is
// left out (and forgotten).
func (recent *Recent) Lines(now time.Time) []Line {
	recent.mutex.Lock()
	defer recent.mutex.Unlock()
	recent.fine = withinAge(recent.fine, now.Add(-recent.limits.Fine))
	recent.kept = withinAge(recent.kept, now.Add(-recent.limits.Kept))
	lines := make([]Line, 0, len(recent.fine)+len(recent.kept))
	lines = append(append(lines, recent.kept...), recent.fine...)
	slices.SortStableFunc(lines, func(a, b Line) int { return a.Time.Compare(b.Time) })
	return lines
}

// add keeps line, forgetting what its kind no longer has room or time for, measured from the
// line's own time.
func (recent *Recent) add(line Line) {
	recent.mutex.Lock()
	defer recent.mutex.Unlock()
	if line.Level < slog.LevelInfo {
		recent.fine = withinCount(withinAge(append(recent.fine, line), line.Time.Add(-recent.limits.Fine)),
			recent.limits.FineCount)
		return
	}
	recent.kept = withinCount(withinAge(append(recent.kept, line), line.Time.Add(-recent.limits.Kept)),
		recent.limits.KeptCount)
}

// withinAge is lines from the first logged at or after since.
func withinAge(lines []Line, since time.Time) []Line {
	first := 0
	for first < len(lines) && lines[first].Time.Before(since) {
		first++
	}
	return dropFirst(lines, first)
}

// withinCount is the last count of lines.
func withinCount(lines []Line, count int) []Line {
	return dropFirst(lines, max(0, len(lines)-count))
}

// dropFirst is lines without its first count, copied down once a quarter of the slice is dropped
// so the array beneath does not grow without end.
func dropFirst(lines []Line, count int) []Line {
	if count == 0 {
		return lines
	}
	rest := lines[count:]
	if count*4 >= cap(lines) {
		return slices.Clone(rest)
	}
	return rest
}

// recentHandler keeps every record in recent, at whatever level, and hands those its written
// handler is enabled for on to it.
type recentHandler struct {
	written slog.Handler
	recent  *Recent
	// fields are the logger's own attributes (With), already prefixed by their groups; prefix is
	// the groups opened since (WithGroup), each followed by a dot.
	fields []Field
	prefix string
}

// Enabled is true at every level: the recent log keeps what is not written.
func (handler recentHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

// Handle keeps the record, then writes it where the written handler's level allows.
func (handler recentHandler) Handle(ctx context.Context, record slog.Record) error {
	fields := slices.Clone(handler.fields)
	record.Attrs(func(attr slog.Attr) bool {
		fields = appendField(fields, handler.prefix, attr)
		return true
	})
	if id := RequestID(ctx); id != "" {
		fields = append(fields, Field{Key: RequestIDKey, Value: slog.StringValue(id)})
	}
	handler.recent.add(Line{Time: record.Time, Level: record.Level, Message: record.Message, Fields: fields})
	if !handler.written.Enabled(ctx, record.Level) {
		return nil
	}
	return handler.written.Handle(ctx, record)
}

// WithAttrs keeps attrs as fields of every line, and gives them to the written handler.
func (handler recentHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	fields := slices.Clone(handler.fields)
	for _, attr := range attrs {
		fields = appendField(fields, handler.prefix, attr)
	}
	return recentHandler{
		written: handler.written.WithAttrs(attrs), recent: handler.recent, fields: fields, prefix: handler.prefix,
	}
}

// WithGroup prefixes the keys logged after it with name.
func (handler recentHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return handler
	}
	return recentHandler{
		written: handler.written.WithGroup(name), recent: handler.recent, fields: handler.fields,
		prefix: handler.prefix + name + ".",
	}
}

// appendField adds attr to fields as slog's own handlers would write it: resolved, a group's
// attributes each a field under its name, an empty attribute or group left out.
func appendField(fields []Field, prefix string, attr slog.Attr) []Field {
	value := attr.Value.Resolve()
	if value.Kind() != slog.KindGroup {
		if attr.Equal(slog.Attr{}) {
			return fields
		}
		return append(fields, Field{Key: prefix + attr.Key, Value: value})
	}
	inner := prefix
	if attr.Key != "" {
		inner = prefix + attr.Key + "."
	}
	for _, member := range value.Group() {
		fields = appendField(fields, inner, member)
	}
	return fields
}

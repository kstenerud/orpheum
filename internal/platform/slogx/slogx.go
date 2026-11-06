// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// EventKey is the attribute key of a log line's event: a short camelCase name, stable while the
// message's English is reworded, that a test matches the line by (testutil.LineWith,
// testutil.Log.AwaitEvent) instead of by its sentence (test architecture review, 2026-10-26,
// finding 12). One key for the whole tree.
const EventKey = "logEvent"

// Format is how a logger renders each record.
type Format string

// The formats a logger can write. The set is closed: a switch over Format names every one.
const (
	// FormatJSON writes one JSON object per record, for the service's journal.
	FormatJSON Format = "json"
	// FormatText writes key=value lines, for a developer's terminal.
	FormatText Format = "text"
)

// ParseFormat turns a configuration value into a Format, or says which values are accepted.
func ParseFormat(value string) (Format, error) {
	format := Format(value)
	switch format {
	case FormatJSON, FormatText:
		return format, nil
	}
	return "", fmt.Errorf("log format %q: want %q or %q", value, FormatJSON, FormatText)
}

// New builds the process's logger. Only cmd/orpheum calls it; every other package receives
// the resulting *slog.Logger as a constructor parameter and adds its own attributes with
// With("component", ...). It logs at level, which a *slog.LevelVar changes while it runs (D347),
// and a line logged with a request's context carries the request's ID (WithRequestID).
func New(output io.Writer, format Format, level slog.Leveler) (*slog.Logger, error) {
	options := &slog.HandlerOptions{Level: level}
	switch format {
	case FormatJSON:
		return slog.New(requestHandler{slog.NewJSONHandler(output, options)}), nil
	case FormatText:
		return slog.New(requestHandler{slog.NewTextHandler(output, options)}), nil
	}
	return nil, fmt.Errorf("log format %q: want %q or %q", format, FormatJSON, FormatText)
}

// UserAgent is the browser's own words for itself, as a log attribute and nothing else: behaviour
// never branches on it (D2, D103), but a log that names the browser tells an iPhone's player from a
// desktop's when one stalls (staging, 2026-10-06).
func UserAgent(request *http.Request) slog.Attr {
	return slog.String("userAgent", request.UserAgent())
}

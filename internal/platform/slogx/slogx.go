// SPDX-License-Identifier: GPL-3.0-or-later

package slogx

import (
	"fmt"
	"io"
	"log/slog"
)

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
// With("component", ...).
func New(output io.Writer, format Format, level slog.Leveler) (*slog.Logger, error) {
	options := &slog.HandlerOptions{Level: level}
	switch format {
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(output, options)), nil
	case FormatText:
		return slog.New(slog.NewTextHandler(output, options)), nil
	}
	return nil, fmt.Errorf("log format %q: want %q or %q", format, FormatJSON, FormatText)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The signed objects' line format (the root key audit's §3.1; D393 §5): UTF-8 text, LF line ends, the
// last line ended, no CR, tab, trailing space or empty line, at most maxPayloadBytes. Line 1 is
// "orpheum-signed <type> <format>"; each other line is "<key> <value>", the keys in the schema's
// order, repeated only where the schema says. A key the reader does not know is ignored unless it
// begins with "!", which refuses the object: new lines that grant are added plain, new lines that
// restrict are added critical.
const (
	headerWord      = "orpheum-signed"
	maxPayloadBytes = 64 << 10
	criticalMark    = "!"
)

// Errors reading a payload returns.
var (
	ErrNotStrict = errors.New("not a signed object's strict text: " +
		"LF lines, no tab, CR, trailing space or empty line")
	ErrWrongType       = errors.New("the signed object is of another type")
	ErrUnknownFormat   = errors.New("the signed object's format is newer than this build: update Orpheum")
	ErrCritical        = errors.New("the signed object has a critical line this build does not know: update Orpheum")
	ErrOutOfOrder      = errors.New("the signed object's lines are out of their order, or one is repeated")
	ErrMissingLine     = errors.New("the signed object lacks a line it must have")
	ErrBadValue        = errors.New("a line of the signed object holds a value it may not")
	ErrPayloadTooLarge = errors.New("the signed object is larger than 64 KiB")
)

// Field is one key a type's lines may hold: its name, and whether it may repeat or must be present.
type Field struct {
	Name       string
	IsRepeated bool
	IsRequired bool
}

// Schema is a type's lines: its type word, the format this build reads, and its fields in order.
type Schema struct {
	Type   string
	Format int
	Fields []Field
}

// Lines are a payload's values, by key, in the order they came.
type Lines map[string][]string

// One is key's value, empty when the payload has none.
func (lines Lines) One(key string) string {
	if values := lines[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}

// Parse reads payload as schema's type: strict, in order, every required line present, unknown
// plain lines skipped and an unknown critical one refused.
func (schema Schema) Parse(payload []byte) (Lines, error) {
	if len(payload) > maxPayloadBytes {
		return nil, ErrPayloadTooLarge
	}
	rows, err := strictLines(payload)
	if err != nil {
		return nil, err
	}
	header := strings.Split(rows[0], " ")
	if len(header) != 3 || header[0] != headerWord {
		return nil, fmt.Errorf("%w: its first line is not %q", ErrNotStrict, headerWord+" <type> <format>")
	}
	if header[1] != schema.Type {
		return nil, fmt.Errorf("%w: %q, not %q", ErrWrongType, header[1], schema.Type)
	}
	if format, formatErr := strconv.Atoi(header[2]); formatErr != nil || format != schema.Format {
		return nil, fmt.Errorf("%w: %s %s", ErrUnknownFormat, header[1], header[2])
	}
	return schema.fields(rows[1:])
}

// fields are rows' values, checked against the schema's fields.
func (schema Schema) fields(rows []string) (Lines, error) {
	lines := Lines{}
	position := 0
	for _, row := range rows {
		key, value, _ := strings.Cut(row, " ")
		index := schema.index(key)
		switch {
		case index < 0 && strings.HasPrefix(key, criticalMark):
			return nil, fmt.Errorf("%w: %q", ErrCritical, key)
		case index < 0:
			continue
		case value == "":
			return nil, fmt.Errorf("%w: %q has no value", ErrBadValue, key)
		case index < position, index == position && len(lines[key]) > 0 && !schema.Fields[index].IsRepeated:
			return nil, fmt.Errorf("%w: %q", ErrOutOfOrder, key)
		}
		position = index
		lines[key] = append(lines[key], value)
	}
	for _, field := range schema.Fields {
		if field.IsRequired && len(lines[field.Name]) == 0 {
			return nil, fmt.Errorf("%w: %q", ErrMissingLine, field.Name)
		}
	}
	return lines, nil
}

// index is key's place among the schema's fields, or -1.
func (schema Schema) index(key string) int {
	for index, field := range schema.Fields {
		if field.Name == key {
			return index
		}
	}
	return -1
}

// Write is the payload of values, in the schema's order: each value written as it is given. A value
// that would break the strict text is refused.
func (schema Schema) Write(values Lines) ([]byte, error) {
	var written bytes.Buffer
	fmt.Fprintf(&written, "%s %s %d\n", headerWord, schema.Type, schema.Format)
	for _, field := range schema.Fields {
		for _, value := range values[field.Name] {
			fmt.Fprintf(&written, "%s %s\n", field.Name, value)
		}
	}
	payload := written.Bytes()
	if _, err := schema.Parse(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// strictLines are payload's lines, once it is strict text.
func strictLines(payload []byte) ([]string, error) {
	text := string(payload)
	if text == "" || !strings.HasSuffix(text, "\n") || strings.ContainsAny(text, "\r\t") {
		return nil, ErrNotStrict
	}
	rows := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for _, row := range rows {
		if row == "" || strings.HasSuffix(row, " ") || strings.HasPrefix(row, " ") || strings.Contains(row, "  ") {
			return nil, ErrNotStrict
		}
	}
	return rows, nil
}

// TimeText is at as the formats write a time: RFC 3339, UTC, to the second, with Z.
func TimeText(at time.Time) string {
	return at.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// ParseTime is a format's time, which must be written as TimeText writes it.
func ParseTime(text string) (time.Time, error) {
	at, err := time.Parse(time.RFC3339, text)
	if err != nil || TimeText(at) != text {
		return time.Time{}, fmt.Errorf("%w: the time %q is not RFC 3339 UTC to the second", ErrBadValue, text)
	}
	return at, nil
}

// ParseCount is a format's version or sequence: a decimal number above zero.
func ParseCount(text string) (int64, error) {
	count, err := strconv.ParseInt(text, 10, 64)
	if err != nil || count < 1 || strconv.FormatInt(count, 10) != text {
		return 0, fmt.Errorf("%w: %q is not a count", ErrBadValue, text)
	}
	return count, nil
}

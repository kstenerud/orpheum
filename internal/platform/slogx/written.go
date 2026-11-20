// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// writtenLineMostBytes is the longest written line KeepWritten reads; a longer one ends the reading.
const writtenLineMostBytes = 4 << 20

// KeepWritten keeps in recent every line input holds as a logger of format wrote it, each kept and
// bounded as a line logged would be, so a process that is not running is read as the recent log it
// would have held (a core that does not start keeps none in memory, D524 §5). It says the newest
// line's time, which Lines takes to give what the process held just after it, and how many lines
// did not read as a line of format (the first, cut, of a file read from its middle, say).
//
// A JSON line's values keep their kinds: a number is an int64 or a float64, a boolean a bool, an
// object's keys fields under its name and a dot. A text line does not say a value's kind, so every
// value is read as a string, which a problem report names unless its key says it is only shape.
func (recent *Recent) KeepWritten(input io.Reader, format Format) (newest time.Time, unread int, err error) {
	var read func([]byte) (Line, bool)
	switch format {
	case FormatJSON:
		read = readJSONLine
	case FormatText:
		read = readTextLine
	default:
		return time.Time{}, 0, fmt.Errorf("log format %q: want %q or %q", format, FormatJSON, FormatText)
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64<<10), writtenLineMostBytes)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		line, ok := read(scanner.Bytes())
		if !ok {
			unread++
			continue
		}
		recent.add(line)
		if line.Time.After(newest) {
			newest = line.Time
		}
	}
	if err = scanner.Err(); err != nil {
		return newest, unread, fmt.Errorf("read the written log: %w", err)
	}
	return newest, unread, nil
}

// readJSONLine is a line slog's JSON handler wrote, its fields in the order written.
func readJSONLine(written []byte) (Line, bool) {
	var line Line
	var hasTime, hasLevel, hasMessage bool
	fields, err := readJSONObject(written, "")
	if err != nil {
		return Line{}, false
	}
	for _, each := range fields {
		switch each.Key {
		case slog.TimeKey:
			line.Time, hasTime = parseWrittenTime(each.Value)
		case slog.LevelKey:
			hasLevel = each.Value.Kind() == slog.KindString &&
				line.Level.UnmarshalText([]byte(each.Value.String())) == nil
		case slog.MessageKey:
			line.Message, hasMessage = each.Value.String(), each.Value.Kind() == slog.KindString
		default:
			line.Fields = append(line.Fields, each)
		}
	}
	return line, hasTime && hasLevel && hasMessage
}

// readJSONObject is the fields of the JSON object written, each key after prefix, an object's
// fields under its key and a dot, in the order written. It is read by strictjson (D456), and its
// order by objectKeys, as a map does not keep one.
func readJSONObject(written []byte, prefix string) ([]Field, error) {
	var members map[string]json.RawMessage
	if err := strictjson.Unmarshal(written, &members); err != nil {
		return nil, fmt.Errorf("read a written line: %w", err)
	}
	if members == nil {
		return nil, errors.New("a written line is not a JSON object")
	}
	var fields []Field
	for _, key := range objectKeys(written) {
		raw := members[key]
		if len(raw) == 0 || raw[0] != '{' {
			fields = append(fields, Field{Key: prefix + key, Value: jsonValue(raw)})
			continue
		}
		inner, err := readJSONObject(raw, prefix+key+".")
		if err != nil {
			return nil, err
		}
		fields = append(fields, inner...)
	}
	return fields, nil
}

// objectKeys is the keys of the JSON object written, which strictjson has read whole, in the order
// written: each string directly in the object that a colon follows.
func objectKeys(written []byte) []string {
	var keys []string
	depth := 0
	for offset := 0; offset < len(written); offset++ {
		switch written[offset] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case '"':
			end := stringEnd(written, offset)
			if depth == 1 && bytes.HasPrefix(bytes.TrimLeft(written[end+1:], " \t\r\n"), []byte(":")) {
				var key string
				if strictjson.Unmarshal(written[offset:end+1], &key) == nil {
					keys = append(keys, key)
				}
			}
			offset = end
		}
	}
	return keys
}

// stringEnd is the index of the quote that ends the JSON string starting at start.
func stringEnd(written []byte, start int) int {
	for at := start + 1; at < len(written); at++ {
		switch written[at] {
		case '\\':
			at++
		case '"':
			return at
		}
	}
	return len(written) - 1
}

// jsonValue is one JSON value of a field, which strictjson has read: a number, a boolean or a
// string as its kind, null as an empty string and an array as the JSON it is.
func jsonValue(raw json.RawMessage) slog.Value {
	text := string(bytes.TrimSpace(raw))
	switch {
	case text == "null":
		return slog.StringValue("")
	case text == "true", text == "false":
		return slog.BoolValue(text == "true")
	case strings.HasPrefix(text, `"`):
		var decoded string
		if strictjson.Unmarshal(raw, &decoded) == nil {
			return slog.StringValue(decoded)
		}
	case strings.HasPrefix(text, "["):
	default:
		if whole, err := strconv.ParseInt(text, 10, 64); err == nil {
			return slog.Int64Value(whole)
		}
		if float, err := strconv.ParseFloat(text, 64); err == nil {
			return slog.Float64Value(float)
		}
	}
	return slog.StringValue(text)
}

// parseWrittenTime is the time a handler wrote, as either format writes it.
func parseWrittenTime(value slog.Value) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339Nano, value.String())
	return at, err == nil && value.Kind() == slog.KindString
}

// readTextLine is a line slog's text handler wrote: key=value pairs, either quoted where it would
// not read as one word.
func readTextLine(written []byte) (Line, bool) {
	var line Line
	var hasTime, hasLevel, hasMessage bool
	rest := string(written)
	for rest != "" {
		pair, after, ok := nextTextPair(rest)
		if !ok {
			return Line{}, false
		}
		rest = after
		switch pair.Key {
		case slog.TimeKey:
			line.Time, hasTime = parseWrittenTime(pair.Value)
		case slog.LevelKey:
			hasLevel = line.Level.UnmarshalText([]byte(pair.Value.String())) == nil
		case slog.MessageKey:
			line.Message, hasMessage = pair.Value.String(), true
		default:
			line.Fields = append(line.Fields, pair)
		}
	}
	return line, hasTime && hasLevel && hasMessage
}

// nextTextPair is the first key=value of written, its value a string, and what follows it, its
// space dropped.
func nextTextPair(written string) (pair Field, rest string, ok bool) {
	key, rest, ok := nextTextWord(written, "=")
	if !ok || !strings.HasPrefix(rest, "=") {
		return Field{}, "", false
	}
	value, rest, ok := nextTextWord(rest[1:], " ")
	if !ok {
		return Field{}, "", false
	}
	return Field{Key: key, Value: slog.StringValue(value)}, strings.TrimPrefix(rest, " "), true
}

// nextTextWord is written's first word, quoted or ending before end, and the rest from end.
func nextTextWord(written, end string) (word, rest string, ok bool) {
	if !strings.HasPrefix(written, `"`) {
		at := strings.Index(written, end)
		if at < 0 {
			at = len(written)
		}
		return written[:at], written[at:], true
	}
	for at := 1; at < len(written); at++ {
		switch written[at] {
		case '\\':
			at++
		case '"':
			unquoted, err := strconv.Unquote(written[:at+1])
			return unquoted, written[at+1:], err == nil
		}
	}
	return "", "", false
}

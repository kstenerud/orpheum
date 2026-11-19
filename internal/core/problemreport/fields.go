// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// shapeKey says whether a log key's string values are only shape: an enum the code chose, never a
// title, a path, a name, an address, a URL or free text. Their values are kept as logged. The list
// is short on purpose (default deny, D524 "How"): a key not on it has its value named.
// Surveyed from the logger calls under internal/ and cmd/ on 2026-11-19.
func shapeKey(key string) bool {
	switch key {
	case slogx.EventKey, // a constant camelCase event
		slogx.RequestIDKey, // a random request ID
		"kind",             // a job's, a note's, an attempt's kind
		"jobKind",          // a job's kind
		"action",           // a together or cast event's action
		"state",            // a listing's, a group's or a progress' state
		"outcome",          // a request's or an answer's outcome
		"mechanism",        // how a connection was made
		"method",           // an HTTP or RPC method
		"chosenEncoder",    // the encoder FFmpeg was given
		"commandType":      // a node command's type
		return true
	}
	return false
}

// freeTextKey says whether a log key's string values are words kept for their sense (an error's,
// a refusal's): they are scrubbed, not named whole. "reason" is here and not a shape key: some
// callers log an error's words or a client's own under it.
func freeTextKey(key string) bool {
	switch key {
	case "err", "why", "reason", "cause", "failure", "detail", "problem", "userAgent":
		return true
	}
	return false
}

// redacted is what a secret logs as (internal/core/secrets): it names nothing, and is kept.
const redacted = "[redacted]"

// field is one log field as the report shows it.
type field struct {
	key   string
	value text
}

// lastSegment is a dotted key's own name, without its groups'.
func lastSegment(key string) string {
	if at := strings.LastIndexByte(key, '.'); at >= 0 {
		return key[at+1:]
	}
	return key
}

// fieldOf is a log field made safe: a number, a duration, a time or a boolean as logged; a string
// as logged under a shape key, scrubbed under a free-text key or when it holds a URL, named
// under any other (the scrubber
// then looks for it in free text too).
func (builder *builder) fieldOf(each slogx.Field) field {
	kept := func(words string) field { return field{key: each.Key, value: literal(words)} }
	value := each.Value
	var words string
	switch value.Kind() {
	case slog.KindInt64:
		return kept(strconv.FormatInt(value.Int64(), 10))
	case slog.KindUint64:
		return kept(strconv.FormatUint(value.Uint64(), 10))
	case slog.KindFloat64:
		return kept(strconv.FormatFloat(value.Float64(), 'g', -1, 64))
	case slog.KindBool:
		return kept(strconv.FormatBool(value.Bool()))
	case slog.KindDuration:
		return kept(value.Duration().String())
	case slog.KindTime:
		return kept(value.Time().UTC().Format(time.RFC3339Nano))
	case slog.KindString:
		words = value.String()
	case slog.KindAny, slog.KindGroup, slog.KindLogValuer:
		words = fmt.Sprint(value.Any())
	}
	key := lastSegment(each.Key)
	switch {
	case words == "" || words == redacted || shapeKey(key):
		return kept(words)
	case freeTextKey(key) || strings.Contains(words, "://"):
		// A URL is scrubbed, not named whole: its query, which may hold a token, is dropped from
		// the full form too.
		return field{key: each.Key, value: builder.freeText(words)}
	}
	return field{key: each.Key, value: named(builder.nameValue(words))}
}

// nameValue is the entry for a value a field or a fact gives whole: a known value's own kind, else
// a path's if it is absolute, an address's if it is one, else text's. The scrubber looks for it in
// free text after.
func (builder *builder) nameValue(value string) *entry {
	kind, other := KindText, false
	if found, ok := builder.known[value]; ok {
		kind, other = found.Kind, found.Other
	} else if strings.HasPrefix(value, "/") {
		kind = KindPath
	} else if _, err := netip.ParseAddr(value); err == nil {
		kind = KindAddress
	}
	builder.scrubber.know(kind, value, other)
	return builder.names.name(kind, value, other)
}

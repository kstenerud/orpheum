// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"fmt"
	"log/slog"
)

// redacted is what every printing, logging or serialising method of Value emits.
const redacted = "[redacted]"

// Value is a third-party secret in memory. Nothing that prints, logs or serialises it can show
// its contents; only Reveal can, and the linter allows Reveal only in the provider clients that
// send the secret to its owner's service (D194 §5).
type Value struct {
	text string
}

// NewValue wraps text as a secret the moment it is read.
func NewValue(text string) Value {
	return Value{text: text}
}

// Reveal returns the secret's text, for the one client that must present it.
func (value Value) Reveal() string {
	return value.text
}

// IsZero reports whether the secret is empty.
func (value Value) IsZero() bool {
	return value.text == ""
}

// String returns "[redacted]".
func (Value) String() string {
	return redacted
}

// GoString returns "[redacted]", so %#v cannot show the field.
func (Value) GoString() string {
	return redacted
}

// Format writes "[redacted]" for every verb, so no fmt verb reaches the field.
func (Value) Format(state fmt.State, _ rune) {
	// fmt.Formatter has no way to report a failed write, and fmt ignores it too.
	fmt.Fprint(state, redacted) //nolint:errcheck // Format cannot return an error; see above
}

// MarshalJSON returns the JSON string "[redacted]".
func (Value) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}

// MarshalText returns "[redacted]".
func (Value) MarshalText() ([]byte, error) {
	return []byte(redacted), nil
}

// LogValue logs as "[redacted]".
func (Value) LogValue() slog.Value {
	return slog.StringValue(redacted)
}

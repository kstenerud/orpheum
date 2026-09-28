// SPDX-License-Identifier: GPL-3.0-or-later

package outbound

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetryAfter caps how long a Retry-After may hold us: a server that asks for a day is more
// likely wrong than meaning it (D213 §5).
const maxRetryAfter = time.Hour

// RetryAfter reads a Retry-After in either form: seconds, or an HTTP date, measured against the
// reply's own Date so no clock of ours is needed. It is capped at an hour.
func RetryAfter(header http.Header) (time.Duration, bool) {
	value := strings.TrimSpace(header.Get("Retry-After"))
	var after time.Duration
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		after = time.Duration(seconds) * time.Second
	} else {
		at, atErr := http.ParseTime(value)
		sent, sentErr := http.ParseTime(header.Get("Date"))
		if atErr != nil || sentErr != nil {
			return 0, false
		}
		after = max(at.Sub(sent), 0)
	}
	return min(after, maxRetryAfter), true
}

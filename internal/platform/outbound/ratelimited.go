// SPDX-License-Identifier: AGPL-3.0-or-later

package outbound

import (
	"net/http"
	"time"
)

// unnamedRetryAfter is how long a provider that answered 429 without a usable Retry-After is left
// alone: it asked to be left, and said not for how long.
const unnamedRetryAfter = time.Second

// RateLimitedError is a provider asking to be called again after a time: 429, with or without a
// Retry-After (D170). Every provider client returns this one type, so a job waits for it and the
// API answers it the same way whichever provider asked.
type RateLimitedError struct {
	// Provider is the provider's name as a person knows it: "TMDB", "OpenSubtitles".
	Provider string
	// RetryAfter is how long to wait before calling it again.
	RetryAfter time.Duration
}

func (limited *RateLimitedError) Error() string {
	return limited.Provider + " asks us to wait " + limited.RetryAfter.String()
}

// RateLimited is provider's 429: the wait its Retry-After names, or a second when it names none
// that can be read.
func RateLimited(provider string, header http.Header) *RateLimitedError {
	after, found := RetryAfter(header)
	if !found {
		after = unnamedRetryAfter
	}
	return &RateLimitedError{Provider: provider, RetryAfter: after}
}

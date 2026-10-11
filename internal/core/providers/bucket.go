// SPDX-License-Identifier: AGPL-3.0-or-later

package providers

import "time"

// bucket is a token bucket on the server's clock: it holds up to capacity tokens, a request takes
// one, and ratePerSecond flow back in. It is how a provider keeps to a request rate.
type bucket struct {
	ratePerSecond float64
	capacity      float64
	tokens        float64
	updatedAt     time.Time
}

// newBucket is a full bucket of a second's worth of requests at ratePerSecond, at least one.
func newBucket(ratePerSecond float64, now time.Time) *bucket {
	capacity := max(ratePerSecond, 1)
	return &bucket{ratePerSecond: ratePerSecond, capacity: capacity, tokens: capacity, updatedAt: now}
}

// refill adds what has flowed in since the last look.
func (tokens *bucket) refill(now time.Time) {
	if elapsed := now.Sub(tokens.updatedAt); elapsed > 0 {
		tokens.tokens = min(tokens.capacity, tokens.tokens+elapsed.Seconds()*tokens.ratePerSecond)
		tokens.updatedAt = now
	}
}

// readyAt is when the bucket holds a whole token: now, or when one has flowed in.
func (tokens *bucket) readyAt(now time.Time) time.Time {
	tokens.refill(now)
	if tokens.tokens >= 1 {
		return now
	}
	missing := 1 - tokens.tokens
	return now.Add(time.Duration(missing / tokens.ratePerSecond * float64(time.Second)))
}

// setRate has ratePerSecond and a second's worth of capacity flow from now on, keeping the
// tokens the bucket holds within its new capacity.
func (tokens *bucket) setRate(ratePerSecond float64, now time.Time) {
	if ratePerSecond == tokens.ratePerSecond {
		return
	}
	tokens.refill(now)
	tokens.ratePerSecond, tokens.capacity = ratePerSecond, max(ratePerSecond, 1)
	tokens.tokens = min(tokens.tokens, tokens.capacity)
}

// take takes a token the caller found ready.
func (tokens *bucket) take() { tokens.tokens-- }

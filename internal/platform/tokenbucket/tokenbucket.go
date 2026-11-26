// SPDX-License-Identifier: AGPL-3.0-or-later

package tokenbucket

import "time"

// Bucket is a token bucket on the caller's clock: it holds up to capacity tokens, a use takes some,
// and ratePerSecond flow back in. It is not safe for concurrent use; its owner guards it.
type Bucket struct {
	ratePerSecond float64
	capacity      float64
	tokens        float64
	updatedAt     time.Time
}

// New is a full bucket of a second's worth of tokens at ratePerSecond, at least one.
func New(ratePerSecond float64, now time.Time) *Bucket {
	capacity := max(ratePerSecond, 1)
	return &Bucket{ratePerSecond: ratePerSecond, capacity: capacity, tokens: capacity, updatedAt: now}
}

// refill adds what has flowed in since the last look.
func (tokens *Bucket) refill(now time.Time) {
	if elapsed := now.Sub(tokens.updatedAt); elapsed > 0 {
		tokens.tokens = min(tokens.capacity, tokens.tokens+elapsed.Seconds()*tokens.ratePerSecond)
		tokens.updatedAt = now
	}
}

// ReadyAt is when the bucket holds a whole token: now, or when one has flowed in.
func (tokens *Bucket) ReadyAt(now time.Time) time.Time {
	tokens.refill(now)
	if tokens.tokens >= 1 {
		return now
	}
	missing := 1 - tokens.tokens
	return now.Add(time.Duration(missing / tokens.ratePerSecond * float64(time.Second)))
}

// SetRate has ratePerSecond and a second's worth of capacity flow from now on, keeping the tokens
// the bucket holds within its new capacity.
func (tokens *Bucket) SetRate(ratePerSecond float64, now time.Time) {
	if ratePerSecond == tokens.ratePerSecond {
		return
	}
	tokens.refill(now)
	tokens.ratePerSecond, tokens.capacity = ratePerSecond, max(ratePerSecond, 1)
	tokens.tokens = min(tokens.tokens, tokens.capacity)
}

// Rate is the tokens a second that flow back in.
func (tokens *Bucket) Rate() float64 { return tokens.ratePerSecond }

// Take takes a token the caller found ready (ReadyAt).
func (tokens *Bucket) Take() { tokens.tokens-- }

// TakeIfHeld takes count tokens if the bucket holds them now, and says whether it did; a bucket
// holding fewer is left as it is.
func (tokens *Bucket) TakeIfHeld(count float64, now time.Time) bool {
	tokens.refill(now)
	if tokens.tokens < count {
		return false
	}
	tokens.tokens -= count
	return true
}

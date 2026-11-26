// SPDX-License-Identifier: AGPL-3.0-or-later

package tokenbucket

import (
	"testing"
	"time"
)

var start = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

func TestBucket_StartsFullAndRefillsAtItsRateUpToASecondsWorth(t *testing.T) {
	t.Parallel()
	bucket := New(100, start)
	if !bucket.TakeIfHeld(100, start) {
		t.Fatal("a new bucket did not hold a second's worth")
	}
	if bucket.TakeIfHeld(1, start) {
		t.Fatal("an emptied bucket gave a token at once")
	}
	if !bucket.TakeIfHeld(50, start.Add(500*time.Millisecond)) {
		t.Fatal("half a second did not refill half a second's worth")
	}
	if bucket.TakeIfHeld(101, start.Add(time.Hour)) {
		t.Fatal("an hour filled the bucket beyond a second's worth")
	}
	if !bucket.TakeIfHeld(100, start.Add(time.Hour)) {
		t.Fatal("an hour did not fill the bucket")
	}
	slow := New(0.5, start)
	slow.Take()
	if got := slow.ReadyAt(start); !got.Equal(start.Add(2 * time.Second)) {
		t.Errorf("a half-token-a-second bucket is ready at %v, want two seconds on", got)
	}
}

func TestBucket_TakeIfHeldTakesAllOrNothing(t *testing.T) {
	t.Parallel()
	bucket := New(10, start)
	if bucket.TakeIfHeld(11, start) {
		t.Fatal("took more than the bucket held")
	}
	if !bucket.TakeIfHeld(10, start) {
		t.Fatal("a refused take took some tokens")
	}
}

func TestBucket_SetRateKeepsTheTokensWithinTheNewCapacity(t *testing.T) {
	t.Parallel()
	bucket := New(100, start)
	bucket.SetRate(10, start)
	if bucket.TakeIfHeld(11, start) {
		t.Fatal("a lowered rate kept more than its second's worth")
	}
	if !bucket.TakeIfHeld(10, start) {
		t.Fatal("a lowered rate lost the tokens within its capacity")
	}
	if !bucket.TakeIfHeld(10, start.Add(time.Second)) {
		t.Fatal("a second at the new rate did not refill its worth")
	}
}

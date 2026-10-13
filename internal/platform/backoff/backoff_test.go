// SPDX-License-Identifier: AGPL-3.0-or-later

package backoff_test

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/backoff"
)

func TestDoubled_DoublesToTheMostAndStaysThere(t *testing.T) {
	t.Parallel()
	for doublings, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second} {
		if got := backoff.Doubled(time.Second, 5*time.Second, int64(doublings)); got != want {
			t.Errorf("Doubled(1s, 5s, %d) = %s, want %s", doublings, got, want)
		}
	}
	if got := backoff.Doubled(time.Second, 5*time.Second, math.MaxInt64); got != 5*time.Second {
		t.Errorf("Doubled with every doubling there is = %s; want the most, not an overflow", got)
	}
	if got := backoff.Doubled(time.Minute, 5*time.Second, 0); got != 5*time.Second {
		t.Errorf("a first wait past the most = %s; want the most", got)
	}
}

func TestJittered_MovesAWaitWithinItsFraction(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		random byte
		want   time.Duration
	}{
		{0x00, 90 * time.Second},  // the least share: a tenth down
		{0x80, 100 * time.Second}, // the middle: unmoved
		{0xff, 110 * time.Second}, // nearly the most: a tenth up
	} {
		got, err := backoff.Jittered(bytes.NewReader(bytes.Repeat([]byte{tc.random}, 8)), 100*time.Second,
			time.Hour, 0.1)
		if err != nil || got.Round(time.Second) != tc.want {
			t.Errorf("Jittered from bytes of %#x = %s, %v; want about %s", tc.random, got, err, tc.want)
		}
	}
	got, err := backoff.Jittered(bytes.NewReader(bytes.Repeat([]byte{0xff}, 8)), time.Hour, time.Hour, 0.1)
	if err != nil || got != time.Hour {
		t.Errorf("a jittered wait at the most = %s, %v; want never past it", got, err)
	}
	got, err = backoff.Jittered(bytes.NewReader(nil), 100*time.Second, time.Hour, 0.1)
	if err == nil || got != 100*time.Second {
		t.Errorf("with a failing random source = %s, %v; want the wait unmoved and the failure said", got, err)
	}
}

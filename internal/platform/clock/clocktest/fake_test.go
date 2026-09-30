// SPDX-License-Identifier: GPL-3.0-or-later

package clocktest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

func TestFake_MovesOnlyWhenTold(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)
	fake := clocktest.NewFake(start)

	if got := fake.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want the start time %v", got, start)
	}
	if got := fake.Now(); !got.Equal(start) {
		t.Fatalf("second Now() = %v, want it unchanged at %v", got, start)
	}

	fake.Advance(90 * time.Second)
	if want := start.Add(90 * time.Second); !fake.Now().Equal(want) {
		t.Errorf("after Advance, Now() = %v, want %v", fake.Now(), want)
	}

	fake.Advance(-time.Hour)
	if want := start.Add(90*time.Second - time.Hour); !fake.Now().Equal(want) {
		t.Errorf("after a backward Advance, Now() = %v, want %v", fake.Now(), want)
	}

	moment := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	fake.Set(moment)
	if !fake.Now().Equal(moment) {
		t.Errorf("after Set, Now() = %v, want %v", fake.Now(), moment)
	}
}

func TestFake_ASleepPassesItsTimeAtOnceUnlessItsContextEnded(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.September, 30, 12, 0, 0, 0, time.UTC)
	fake := clocktest.NewFake(start)
	if err := fake.Sleep(t.Context(), 200*time.Millisecond); err != nil ||
		!fake.Now().Equal(start.Add(200*time.Millisecond)) {
		t.Errorf("after a sleep: %v, now %v; want the wait passed", err, fake.Now())
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	if err := fake.Sleep(ended, time.Hour); !errors.Is(err, context.Canceled) ||
		!fake.Now().Equal(start.Add(200*time.Millisecond)) {
		t.Errorf("a sleep whose context ended: %v, now %v; want its error and the time kept", err, fake.Now())
	}
}

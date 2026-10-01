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

func TestFake_ATimerFiresWhenTheTimeMovesPastItAndNotBefore(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	fake := clocktest.NewFake(start)
	timer := fake.NewTimer(time.Minute)
	fake.Advance(59 * time.Second)
	if hasFired(timer.C()) {
		t.Fatal("a timer fired a second before its minute")
	}
	fake.Advance(time.Second)
	select {
	case at := <-timer.C():
		if !at.Equal(start.Add(time.Minute)) {
			t.Errorf("the timer fired at %v, want %v", at, start.Add(time.Minute))
		}
	default:
		t.Fatal("a timer did not fire when its minute passed")
	}
	fake.Advance(time.Hour)
	if hasFired(timer.C()) {
		t.Error("a timer fired twice")
	}

	timer.Reset(time.Minute)
	if err := fake.Sleep(t.Context(), time.Minute); err != nil || !hasFired(timer.C()) {
		t.Errorf("a sleep past a timer reset for a minute: %v, and it did not fire", err)
	}
	timer.Reset(time.Minute)
	if !timer.Stop() {
		t.Error("Stop on an armed timer said it was not armed")
	}
	fake.Set(start.Add(24 * time.Hour))
	if hasFired(timer.C()) || timer.Stop() {
		t.Error("a stopped timer fired, or said it was still armed")
	}

	fake.Advance(time.Minute)
	timer.Reset(time.Minute)
	timer.Reset(2 * time.Minute) // a reset before it fired starts the wait again
	fake.Advance(time.Minute)
	if hasFired(timer.C()) {
		t.Error("a timer reset to two minutes fired after one")
	}
	if !hasFired(fake.NewTimer(0).C()) {
		t.Error("a timer of no wait did not fire at once")
	}
}

// hasFired is whether a timer's channel holds a firing, without waiting for one.
func hasFired(fired <-chan time.Time) bool {
	select {
	case <-fired:
		return true
	default:
		return false
	}
}

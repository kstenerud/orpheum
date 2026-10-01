// SPDX-License-Identifier: GPL-3.0-or-later

package clock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

func TestSystem_NowAdvances(t *testing.T) {
	t.Parallel()
	var system clock.System
	first := system.Now()
	second := system.Now()
	if second.Before(first) {
		t.Errorf("System.Now went backwards: %v then %v", first, second)
	}
	if first.IsZero() {
		t.Error("System.Now returned the zero time")
	}
}

func TestSystem_SleepWaitsOrEndsWithItsContext(t *testing.T) {
	t.Parallel()
	var system clock.System
	if err := system.Sleep(t.Context(), time.Millisecond); err != nil {
		t.Errorf("a short sleep: %v", err)
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	if err := system.Sleep(ended, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("a sleep whose context ended: %v, want its error at once", err)
	}
}

func TestSystem_ATimerFiresAfterItsWaitAndAStoppedOneDoesNot(t *testing.T) {
	t.Parallel()
	var system clock.System
	fired := system.NewTimer(time.Millisecond)
	select {
	case <-fired.C():
	case <-time.After(time.Minute):
		t.Fatal("a timer of a millisecond did not fire within a minute")
	}
	stopped := system.NewTimer(time.Hour)
	if !stopped.Stop() {
		t.Error("Stop on an armed timer said it was not armed")
	}
	stopped.Reset(time.Millisecond)
	select {
	case <-stopped.C():
	case <-time.After(time.Minute):
		t.Fatal("a timer reset to a millisecond did not fire within a minute")
	}
}

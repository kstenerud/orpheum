// SPDX-License-Identifier: GPL-3.0-or-later

package clocktest_test

import (
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

// SPDX-License-Identifier: GPL-3.0-or-later

package clock_test

import (
	"testing"

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

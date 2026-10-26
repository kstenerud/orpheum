// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx_test

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// A failure lasting as a loop ticks each second is said at once, then once a minute with how often
// it failed, then once as it ends; a success with no failure going on says nothing.
func TestFailing_SaysAFailureOnceAMinuteAndItsEnd(t *testing.T) {
	t.Parallel()
	var failing slogx.Failing
	start := time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC)
	if isEnded, _ := failing.Succeed(start); isEnded {
		t.Error("a success with no failure going on was said")
	}
	said := 0
	var lastAttributes []any
	for second := range 125 {
		if isSaid, attributes := failing.Fail(start.Add(time.Duration(second) * time.Second)); isSaid {
			said++
			lastAttributes = attributes
		}
	}
	if said != 3 || len(lastAttributes) != 4 || lastAttributes[3] != 121 {
		t.Errorf("125 failures a second apart were said %d times, the last with %v; want 3, at 0, 60 and "+
			"120 s, the last counting 121", said, lastAttributes)
	}
	isEnded, attributes := failing.Succeed(start.Add(125 * time.Second))
	if !isEnded || len(attributes) != 4 || attributes[1] != int64(125_000) || attributes[3] != 125 {
		t.Errorf("the success after them = %t, %v; want the end said, 125 s and 125 failures", isEnded, attributes)
	}
	if isSaid, _ := failing.Fail(start.Add(126 * time.Second)); !isSaid {
		t.Error("a new failure after the end was not said at once")
	}
}

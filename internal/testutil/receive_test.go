// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"strings"
	"testing"
	"time"
)

func TestReceive_TakesWhatComesAndFailsNamingWhatItWaitedForWhenNothingDoes(t *testing.T) {
	t.Parallel()
	ready := make(chan int, 1)
	ready <- 7
	if got := Receive(t, ready, "a number"); got != 7 {
		t.Errorf("Receive = %d, want the 7 sent", got)
	}

	rec := &recorder{TB: t}
	never := make(chan int)
	receiveWithin(rec, never, "the node's prepare command", time.Millisecond)
	if !strings.Contains(rec.fatal, "the node's prepare command") {
		t.Errorf("the failure %q does not say what it waited for", rec.fatal)
	}
	if !strings.Contains(rec.fatal, "TestReceive_TakesWhatComesAndFailsNamingWhatItWaitedForWhenNothingDoes") {
		t.Errorf("the failure does not show the goroutines' stacks, this test's among them:\n%s", rec.fatal)
	}
}

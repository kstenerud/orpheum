// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"fmt"
	"testing"
)

// AwaitLine returns the line holding every part once something writes it, from another goroutine.
func TestLog_AwaitLineWaitsForALineHoldingEveryPart(t *testing.T) {
	t.Parallel()
	log := &Log{}
	writeLine := func(line string) {
		if _, err := fmt.Fprintln(log, line); err != nil {
			t.Error(err)
		}
	}
	writeLine("a run started jobId=1")
	go func() {
		writeLine("a run finished jobId=2")
		writeLine("a run finished jobId=1 speed=3.3")
	}()
	if line := log.AwaitLine(t, "a run finished", "jobId=1"); line != "a run finished jobId=1 speed=3.3\n" {
		t.Errorf("AwaitLine = %q; want the finished line of job 1", line)
	}
}

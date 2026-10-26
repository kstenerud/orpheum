// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"bytes"
	"testing"
	"time"
)

// An answer's retry waits double from the first to the longest, each moved at random by up to a
// tenth either way, so nodes a restarted core left waiting spread out rather than all trying again
// at once (the audit of 2026-10-25, G9); with no random they are not moved.
func TestAnswerRetry_DoublesToTheLongestAndJittersEachWait(t *testing.T) {
	t.Parallel()
	unmoved := answerRetry{firstWait: time.Second, longestWait: 30 * time.Second}
	doubled := map[int64]time.Duration{1: time.Second, 2: 2 * time.Second, 5: 16 * time.Second, 9: 30 * time.Second}
	for tries, want := range doubled {
		if got := unmoved.waitAfter(tries); got != want {
			t.Errorf("unjittered, the wait after %d tries is %v; want %v", tries, got, want)
		}
	}
	latest := unmoved
	latest.random = bytes.NewReader(bytes.Repeat([]byte{0xff}, 16))
	if got := latest.waitAfter(2); got <= 2*time.Second || got > 2200*time.Millisecond {
		t.Errorf("jittered as late as it goes, the wait after 2 tries is %v; want just under 2.2 s", got)
	}
	if got := latest.waitAfter(9); got != 30*time.Second {
		t.Errorf("jittered late, the longest wait is %v; want it held to 30 s", got)
	}
}

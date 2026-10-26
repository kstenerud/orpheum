// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"testing"
	"time"
)

// A film no playback is left of has its stores forgotten, but for one with a run on its way, whose
// node still says its progress; another film's are kept (the audit of 2026-10-25, C5).
func TestForgetFile_ForgetsAFilmsStoresButOnesMaking(t *testing.T) {
	t.Parallel()
	tracker := newRunTracker()
	idle, making, other := "1/chunks-a/g", "1/chunks-b/g", "2/chunks-a/g"
	for _, dir := range []string{idle, making, other} {
		tracker.served(dir, 0)
	}
	tracker.begin(making, 1, 4, 9, 1, time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC))
	tracker.forgetFile(1)
	if _, isKept := tracker.storeByDir[idle]; isKept {
		t.Errorf("file 1's idle store is kept; want it forgotten")
	}
	if !tracker.hasRun(making) || !tracker.isMadeFromThrough(other, 0, 0) {
		t.Errorf("forgetting file 1 lost the run on its way or file 2's chunks; want both kept")
	}
}

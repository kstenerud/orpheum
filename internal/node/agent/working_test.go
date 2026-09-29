// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import "testing"

func TestJobTracker_AJobCancelledBeforeItStartsStartsCancelledAndARunningOneStops(t *testing.T) {
	t.Parallel()
	var all jobTracker
	if all.cancel(5) {
		t.Error("cancelling job 5 before it started said it was running")
	}
	early, doneEarly := all.startJob(t.Context(), 5)
	defer doneEarly()
	if early.Err() == nil {
		t.Error("job 5, cancelled while it waited, started with a live context")
	}
	running, doneRunning := all.startJob(t.Context(), 6)
	defer doneRunning()
	if running.Err() != nil {
		t.Fatal("job 6 started cancelled, though no one cancelled it")
	}
	if !all.cancel(6) || running.Err() == nil {
		t.Error("cancelling running job 6 did not stop it")
	}
}

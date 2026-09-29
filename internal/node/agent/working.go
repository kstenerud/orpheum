// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"sync"
)

// jobTracker holds the jobs this node is working on, by the core's job id, each with the cancel that
// stops it (cut-a §5.3), and the jobs cancelled before they started: a cancel can arrive while
// its job still waits behind other work.
type jobTracker struct {
	mutex                sync.Mutex
	cancelByJobID        map[int64]context.CancelFunc // guarded by mutex
	cancelledBeforeStart map[int64]bool               // guarded by mutex: cancelled before they started
	waiting              map[int64]bool               // guarded by mutex: prepares waiting for a slot
}

// setWaiting marks a job waiting for a slot, or no longer waiting.
func (tracker *jobTracker) setWaiting(jobID int64, isWaiting bool) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	if tracker.waiting == nil {
		tracker.waiting = map[int64]bool{}
	}
	if isWaiting {
		tracker.waiting[jobID] = true
	} else {
		delete(tracker.waiting, jobID)
	}
}

// waitingJobIDs are the jobs waiting for a slot, in no order.
func (tracker *jobTracker) waitingJobIDs() []int64 {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	jobIDs := make([]int64, 0, len(tracker.waiting))
	for jobID := range tracker.waiting {
		jobIDs = append(jobIDs, jobID)
	}
	return jobIDs
}

// startJob is a job's context, which cancel ends, and finishJob, which forgets the job once it ends. A
// job cancelled before it started starts cancelled.
func (tracker *jobTracker) startJob(ctx context.Context, jobID int64) (context.Context, func()) {
	jobCtx, stop := context.WithCancel(ctx)
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	if tracker.cancelledBeforeStart[jobID] {
		delete(tracker.cancelledBeforeStart, jobID)
		stop()
	}
	if tracker.cancelByJobID == nil {
		tracker.cancelByJobID = map[int64]context.CancelFunc{}
	}
	tracker.cancelByJobID[jobID] = stop
	return jobCtx, func() {
		stop()
		tracker.mutex.Lock()
		defer tracker.mutex.Unlock()
		delete(tracker.cancelByJobID, jobID)
	}
}

// cancel stops a job, or, when it has not started, has it start cancelled. It reports whether the
// job was running.
func (tracker *jobTracker) cancel(jobID int64) bool {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	stop, isRunning := tracker.cancelByJobID[jobID]
	if isRunning {
		stop()
		return true
	}
	if tracker.cancelledBeforeStart == nil {
		tracker.cancelledBeforeStart = map[int64]bool{}
	}
	tracker.cancelledBeforeStart[jobID] = true
	return false
}

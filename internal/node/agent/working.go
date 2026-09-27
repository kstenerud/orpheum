// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"sync"
)

// working are the jobs this node is working on, by the core's job id, each with the cancel that
// stops it (cut-a §5.3), and the jobs cancelled before they started: a cancel can arrive while
// its job still waits behind other work.
type working struct {
	mutex     sync.Mutex
	jobs      map[int64]context.CancelFunc // guarded by mutex
	cancelled map[int64]bool               // guarded by mutex: cancelled before they started
	waiting   map[int64]bool               // guarded by mutex: prepares waiting for a slot
}

// wait marks a job waiting for a slot, or no longer waiting.
func (all *working) wait(job int64, isWaiting bool) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if all.waiting == nil {
		all.waiting = map[int64]bool{}
	}
	if isWaiting {
		all.waiting[job] = true
	} else {
		delete(all.waiting, job)
	}
}

// waitingJobs are the jobs waiting for a slot, in no order.
func (all *working) waitingJobs() []int64 {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	jobs := make([]int64, 0, len(all.waiting))
	for job := range all.waiting {
		jobs = append(jobs, job)
	}
	return jobs
}

// start is a job's context, which cancel ends, and done, which forgets the job once it ends. A
// job cancelled before it started starts cancelled.
func (all *working) start(ctx context.Context, job int64) (context.Context, func()) {
	jobCtx, stop := context.WithCancel(ctx)
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if all.cancelled[job] {
		delete(all.cancelled, job)
		stop()
	}
	if all.jobs == nil {
		all.jobs = map[int64]context.CancelFunc{}
	}
	all.jobs[job] = stop
	return jobCtx, func() {
		stop()
		all.mutex.Lock()
		defer all.mutex.Unlock()
		delete(all.jobs, job)
	}
}

// cancel stops a job, or, when it has not started, has it start cancelled. It reports whether the
// job was running.
func (all *working) cancel(job int64) bool {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	stop, isRunning := all.jobs[job]
	if isRunning {
		stop()
		return true
	}
	if all.cancelled == nil {
		all.cancelled = map[int64]bool{}
	}
	all.cancelled[job] = true
	return false
}

// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"sync"
)

// working are the jobs this node is working on, by the core's job id, each with the cancel that
// stops it (cut-a §5.3).
type working struct {
	mutex sync.Mutex
	jobs  map[int64]context.CancelFunc // guarded by mutex
}

// start is a job's context, which cancel ends, and done, which forgets the job once it ends.
func (all *working) start(ctx context.Context, job int64) (context.Context, func()) {
	jobCtx, stop := context.WithCancel(ctx)
	all.mutex.Lock()
	defer all.mutex.Unlock()
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

// cancel stops a job, and reports whether it was one this node is working on.
func (all *working) cancel(job int64) bool {
	all.mutex.Lock()
	stop, found := all.jobs[job]
	all.mutex.Unlock()
	if found {
		stop()
	}
	return found
}

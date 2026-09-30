// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Sweep derives one kind's work, or part of it, from library.db and queues it (D231 §3): the
// queue is a cache of the work the library's state implies, so a job lost with cache.db,
// cancelled, or parked and forgotten is queued again by the next sweep. A request to queue a job
// is only a hint to run it sooner.
type Sweep = jobqueue.Sweep

// AddSweep adds a sweep. Add them before Sweeping runs.
func (queue *Queue) AddSweep(sweep Sweep) { queue.queue.AddSweep(sweep) }

// RequestSweeps asks for the named sweeps to run soon, or all of them when none is named: after
// an identification pass, and whenever a destination comes back, which the queue asks itself.
func (queue *Queue) RequestSweeps(names ...string) { queue.queue.RequestSweeps(names...) }

// RunSweeps runs the named sweeps now, or all of them, and says how many jobs they queued.
func (queue *Queue) RunSweeps(ctx context.Context, names ...string) int {
	return queue.queue.RunSweeps(ctx, names...)
}

// Sweeping runs every sweep at start, each again on its own timer, checked on every tick, and
// those asked for, until ctx ends (D231 §3).
func (queue *Queue) Sweeping(ctx context.Context, tick <-chan time.Time) {
	queue.queue.Sweeping(ctx, tick)
}

// CancelWhere cancels every queued and running job that match says is the work of what is being
// retired (D231 §7, §9): a root's, a node's or a deleted library's. It says how many it cancelled.
func (queue *Queue) CancelWhere(
	ctx context.Context, match func(target Target, destination Destination) bool,
) (int, error) {
	return queue.queue.CancelWhere(ctx, func(row jobqueue.Row[Target]) bool {
		return match(row.Target, row.Destination)
	})
}

// Forget drops a retired destination: its work was cancelled, and nothing brings it back.
func (queue *Queue) Forget(destination Destination) { queue.queue.Forget(destination) }

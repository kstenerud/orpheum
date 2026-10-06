// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Sweep derives one kind's work, or part of it, from persistent.db and queues it (D231 §3): the
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

// CancelMatching cancels kind's queued and running jobs on target whose arguments match says: a
// run of chunks a seek went far past (D250 §5). It says how many it cancelled.
func (queue *Queue) CancelMatching(
	ctx context.Context, kind Kind, target Target, match func(argsJSON string) bool,
) (int, error) {
	return queue.queue.CancelWhere(ctx, func(row jobqueue.Row[Target]) bool {
		return row.Kind == kind && row.Target == target && match(row.ArgsJSON)
	})
}

// Forget drops a retired destination: its work was cancelled, and nothing brings it back.
func (queue *Queue) Forget(destination Destination) { queue.queue.Forget(destination) }

// RetiredWork is what a retire took out of the library (D231 §7): the items, files, streams and
// fetched subtitles whose jobs end, and the destinations nothing brings back.
type RetiredWork struct {
	ItemIDs            []int64
	FileIDs            []int64
	StreamIDs          []int64
	FetchedSubtitleIDs []int64
	// Destinations are the retired roots and the disks mounted inside them, and a retired node's
	// own and its prepared directory's.
	Destinations []Destination
}

// CancelRetired cancels every queued, parked and running job of what a retire took out, by its
// target or by the destination it waits on, then forgets those destinations. It says how many
// jobs it cancelled.
func (queue *Queue) CancelRetired(ctx context.Context, work RetiredWork) (int, error) {
	items, files := setOf(work.ItemIDs), setOf(work.FileIDs)
	streams, fetchedSubtitles := setOf(work.StreamIDs), setOf(work.FetchedSubtitleIDs)
	destinations := setOf(work.Destinations)
	cancelled, err := queue.CancelWhere(ctx, func(target Target, destination Destination) bool {
		return items[target.ItemID] || files[target.FileID] || streams[target.StreamID] ||
			fetchedSubtitles[target.FetchedSubtitleID] || destinations[destination]
	})
	if err != nil {
		return cancelled, err
	}
	for _, destination := range work.Destinations {
		queue.Forget(destination)
	}
	return cancelled, nil
}

// setOf is the set of values, never holding the zero value, which is a target's unset column.
func setOf[V comparable](values []V) map[V]bool {
	set := make(map[V]bool, len(values))
	var zero V
	for _, value := range values {
		if value != zero {
			set[value] = true
		}
	}
	return set
}

// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"sync"
)

// runTracker knows, for each store, the chunks known made and the runs making more: in memory,
// as a player's requests and the nodes' words arrive. It is what keeps a second run from being
// queued over chunks a run will make (D247 §10). A restart forgets it, and the node's answer for a
// chunk, made or not yet, is the truth it learns again from.
type runTracker struct {
	mutex sync.Mutex
	// storeByDir is each store's chunks and runs, by its directory (StoreDir); guarded by mutex.
	storeByDir map[string]*storeRuns
}

// storeRuns are one store's chunks known made and its runs making more, each by its first chunk.
type storeRuns struct {
	isMade     map[int]bool
	runByFirst map[int]*run
}

// run is one run of a store's chunks, first through last, made in order.
type run struct {
	// jobID is its job, once a handler gave it to its node; 0 while it waits in the queue.
	jobID       int64
	first, last int
	// madeThrough is the last chunk it has made, first-1 before it made any.
	madeThrough int
}

// willMake is whether the run is still to make chunk n.
func (each *run) willMake(n int) bool { return n > each.madeThrough && n <= each.last }

func newRunTracker() *runTracker {
	return &runTracker{storeByDir: map[string]*storeRuns{}}
}

// storeLocked is a store's chunks and runs, made empty the first time it is named.
func (tracker *runTracker) storeLocked(dir string) *storeRuns {
	known, found := tracker.storeByDir[dir]
	if !found {
		known = &storeRuns{isMade: map[int]bool{}, runByFirst: map[int]*run{}}
		tracker.storeByDir[dir] = known
	}
	return known
}

// isCoveredLocked is whether chunk n is made, or a run of the store will make it.
func (known *storeRuns) isCoveredLocked(n int) bool {
	if known.isMade[n] {
		return true
	}
	for _, each := range known.runByFirst {
		if each.willMake(n) {
			return true
		}
	}
	return false
}

// plan finds the run a player asking for chunk wanted of the store needs, and records it on its
// way: the first chunk of those that start within lookaheadMs of film from wanted's start that is
// neither made nor to be made by a run, through the earliest of runSpanMs of film, the chunk before
// the next that is made or to be made, and the grid's last. It reports false when every chunk in
// the look-ahead is made or on its way. The caller queues the run, and forgets it if it was not
// queued.
func (tracker *runTracker) plan(dir string, grid Grid, wanted int) (first, last int, isPlanned bool) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	lookUntilMs := grid.StartsMs[wanted] + lookaheadMs
	first = -1
	for n := wanted; n <= grid.lastChunk() && grid.StartsMs[n] < lookUntilMs; n++ {
		if !known.isCoveredLocked(n) {
			first = n
			break
		}
	}
	if first < 0 {
		return 0, 0, false
	}
	spanUntilMs := grid.StartsMs[first] + runSpanMs
	last = first
	for last < grid.lastChunk() && grid.StartsMs[last+1] < spanUntilMs && !known.isCoveredLocked(last+1) {
		last++
	}
	known.runByFirst[first] = &run{first: first, last: last, madeThrough: first - 1}
	return first, last, true
}

// begin records a run its handler is giving to its node: one planned, or one its job tried again
// after it ended.
func (tracker *runTracker) begin(dir string, first, last int, jobID int64) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	each, found := known.runByFirst[first]
	if !found {
		each = &run{first: first, madeThrough: first - 1}
		known.runByFirst[first] = each
	}
	each.jobID, each.last = jobID, last
}

// madeThrough records that the run of the store from first has made every chunk through last.
func (tracker *runTracker) madeThrough(dir string, first, last int) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	for chunk := first; chunk <= last; chunk++ {
		known.isMade[chunk] = true
	}
	if each, found := known.runByFirst[first]; found && last > each.madeThrough {
		each.madeThrough = last
	}
}

// forget forgets the run of the store from first: it ended, or was never queued.
func (tracker *runTracker) forget(dir string, first int) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	delete(tracker.storeLocked(dir).runByFirst, first)
}

// served records chunk n of the store made: its node served it.
func (tracker *runTracker) served(dir string, n int) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.storeLocked(dir).isMade[n] = true
}

// isMaking is whether a run of the store is on its way.
func (tracker *runTracker) isMaking(dir string) bool {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	return len(tracker.storeLocked(dir).runByFirst) > 0
}

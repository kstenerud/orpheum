// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"sync"
	"time"
)

// runTracker knows, for each store, the chunks known made and the runs making more: in memory,
// as a player's requests and the nodes' words arrive. It is what keeps a second run from being
// queued over chunks a run will make (D247 §10). A restart forgets it, and the node's answer for a
// chunk, made or not yet, is the truth it learns again from.
type runTracker struct {
	mutex sync.Mutex
	// storeByDir is each store's chunks and runs, by its directory (StoreDir); guarded by mutex.
	storeByDir map[string]*storeRuns
	// evictedOnAt is when a run was last evicted from each node, by its id (capacity.go); guarded by
	// mutex.
	evictedOnAt map[int64]time.Time
}

// storeRuns are one store's chunks known made and its runs making more, each by its first chunk.
type storeRuns struct {
	// storeID is its chunk_store row, which its runs' jobs target; 0 until a run of it is asked for.
	storeID    int64
	isMade     map[int]bool
	runByFirst map[int]*run
	// lastAskedAt is when a player last asked for one of its chunks: a store asked for lately is in
	// use (capacity.go).
	lastAskedAt time.Time
	// evictedAt is when a run of it was evicted to keep its node's others faster than playback; zero
	// unless it was, until a player's stall asks for it again (D250 §6).
	evictedAt time.Time
}

// run is one run of a store's chunks, first through last, made in order.
type run struct {
	// jobID is its job, once a handler gave it to its node; 0 while it waits in the queue.
	jobID       int64
	first, last int
	// madeThrough is the last chunk it has made, first-1 before it made any.
	madeThrough int
	// nodeID is the node making it, and startedAt when it was given to it; zero while it is queued.
	nodeID    int64
	startedAt time.Time
	// samples are the film it had made at moments of its progress, oldest first, for its speed.
	samples []speedSample
}

// willMake is whether the run is still to make chunk n.
func (each *run) willMake(n int) bool { return n > each.madeThrough && n <= each.last }

func newRunTracker() *runTracker {
	return &runTracker{storeByDir: map[string]*storeRuns{}, evictedOnAt: map[int64]time.Time{}}
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

// isCovered is whether chunk n of the store is made, or a run of it will make it.
func (tracker *runTracker) isCovered(dir string, n int) bool {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	return tracker.storeLocked(dir).isCoveredLocked(n)
}

// isMadeFromThrough is whether every chunk of the store from first through last is known made.
func (tracker *runTracker) isMadeFromThrough(dir string, first, last int) bool {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	for n := first; n <= last; n++ {
		if !known.isMade[n] {
			return false
		}
	}
	return true
}

// cutFar cuts every run of the store that wanted is more than beyondMs of film past: past the chunk
// it makes next. Such a request was sought to (D250 §5), since a player catching up never jumps that
// far, and no one follows those runs now: they are forgotten, and returned for the caller to cancel.
func (tracker *runTracker) cutFar(dir string, grid Grid, wanted int, beyondMs int64) []RunSpan {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	var cuts []RunSpan
	for first, each := range known.runByFirst {
		next := each.madeThrough + 1
		if wanted <= each.madeThrough || next > grid.lastChunk() ||
			grid.StartsMs[wanted]-grid.StartsMs[next] <= beyondMs {
			continue
		}
		cuts = append(cuts, RunSpan{First: each.first, Last: each.last, MadeThrough: each.madeThrough})
		delete(known.runByFirst, first)
	}
	return cuts
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

// RunSpan is a run of a store's chunks as the core knows it: its first and last chunk, and the last
// it has made, First-1 before it made any.
type RunSpan struct {
	First, Last, MadeThrough int
}

// covering is the run of the store whose chunks include chunk n, and whether there is one.
func (tracker *runTracker) covering(dir string, n int) (RunSpan, bool) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	for _, each := range tracker.storeLocked(dir).runByFirst {
		if n >= each.first && n <= each.last {
			return RunSpan{First: each.first, Last: each.last, MadeThrough: each.madeThrough}, true
		}
	}
	return RunSpan{}, false
}

// begin records a run its handler is giving to nodeID at now: one planned, or one its job tried
// again after it ended.
func (tracker *runTracker) begin(dir string, first, last int, jobID, nodeID int64, now time.Time) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	each, found := known.runByFirst[first]
	if !found {
		each = &run{first: first, madeThrough: first - 1}
		known.runByFirst[first] = each
	}
	each.jobID, each.last, each.nodeID = jobID, last, nodeID
	if each.startedAt.IsZero() {
		each.startedAt = now
	}
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

// making is the chunk_store row of the store, and whether a run of it is on its way.
func (tracker *runTracker) making(dir string) (storeID int64, isMaking bool) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	return known.storeID, len(known.runByFirst) > 0
}

// knowStoreID records the chunk_store row of the store.
func (tracker *runTracker) knowStoreID(dir string, storeID int64) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.storeLocked(dir).storeID = storeID
}

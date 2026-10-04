// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// attempt is the core's attempt at a job that the node holds: the job's id, and the token the core
// made for this sending of it, by which the node holds it and every result names it (D238 §2).
type attempt struct {
	jobID int64
	token string
	// work is the work the command asked for, without the bytes it carried, which every result
	// echoes.
	work nodeproto.Work
}

// attemptOf is the attempt a command names.
func attemptOf(command nodeproto.Command) attempt {
	return attempt{jobID: command.JobID, token: command.AttemptToken, work: command.WithoutBytes()}
}

// named is result naming the attempt it answers, and echoing its work.
func (held attempt) named(result nodeproto.Result) nodeproto.Result {
	result.JobID, result.AttemptToken = held.jobID, held.token
	work := held.work
	result.Work = &work
	return result
}

// heldWork is one attempt the node holds, by its token: waiting for a slot of its kind and a reader
// of its device, then running, and perhaps paused for play-driven work.
type heldWork struct {
	attempt
	priority int64
	slot     nodeproto.Slot
	// device is the key of the device its run reads, or "" for work that reads none of the media.
	device string
	// isPlayDriven is work a person waits for, to play: it pauses background work on its device to
	// have a reader there (D240 §6).
	isPlayDriven bool
	// saysHowFar is a prepare, which says how far it is while it runs, as FFmpeg moves on.
	saysHowFar bool
	// run does the work to its end and says how it ended to the core, on a context a cancel ends.
	run func(ctx context.Context)

	arrival int64 // the order it came in, among work of one priority
	// start runs it on the node's lifetime, which hold was given, and returns what stops it.
	start func() context.CancelFunc
	stop  context.CancelFunc
	// pausing pauses its FFmpeg, which runs on a context carrying it (execx.WithPausing).
	pausing *execx.Pausing
	// movement is when it last moved, which its run's context tells (execx.WithMoved).
	movement *movement
	// stalls, for work that reads in the node's own process, a listing or an examining, is told how
	// long its reading has not moved once that is its window (stallStillReads); nil for FFmpeg's
	// work, which execx watches.
	stalls chan time.Duration
}

func (work *heldWork) isRunning() bool { return work.stop != nil }

// isReading is whether it holds a reader of its device: running, not paused.
func (work *heldWork) isReading() bool { return work.isRunning() && !work.pausing.IsPaused() }

// workList is the work the node holds, in memory (D238 §3): each job waits until a slot of its kind
// is free and a reader of its device, and starts before the others waiting at a lower priority and,
// at one priority, before those that came after it (D234 §3). Play-driven work that finds its
// device's readers taken pauses background work reading it, which continues once there is room
// again (D240 §6). A cancel stops a running job, or drops a waiting one before it starts.
type workList struct {
	clock    clock.Clock
	mutex    sync.Mutex
	capacity nodeproto.Capacity     // how many of each kind run at once
	running  map[nodeproto.Slot]int // guarded by mutex: running and paused alike hold their slot
	held     []*heldWork            // guarded by mutex: waiting and running
	arrivals int64                  // guarded by mutex
	// readerLimits are how many may read each device at once, by its key; a device not named has
	// one. Guarded by mutex.
	readerLimits map[string]int
	// changed is closed and replaced at each change of what runs, for work waiting to be continued.
	// Guarded by mutex.
	changed chan struct{}
}

func newWorkList(capacity nodeproto.Capacity, clockSource clock.Clock) *workList {
	return &workList{
		clock: clockSource, capacity: capacity, running: map[nodeproto.Slot]int{}, readerLimits: map[string]int{},
		changed: make(chan struct{}),
	}
}

// setReaderLimits takes the core's limits on each device's readers, and starts what they make room
// for.
func (list *workList) setReaderLimits(limits map[string]int) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	list.readerLimits = maps.Clone(limits)
	list.startDue()
}

// hold takes a job and starts what is due. It reports false, holding nothing, for an attempt it
// holds already: a command the core sent again. It says, as isWaiting, whether the job waits for a
// slot or a reader.
func (list *workList) hold(ctx context.Context, work heldWork) (isHeld, isWaiting bool) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	if list.find(work.token) != nil {
		return false, false
	}
	list.arrivals++
	work.arrival = list.arrivals
	work.pausing, work.movement = &execx.Pausing{}, newMovement(list.clock)
	added := &work
	added.start = func() context.CancelFunc {
		runCtx, stop := context.WithCancel(execx.WithMoved(execx.WithPausing(ctx, added.pausing), added.movement.moved))
		go func() {
			defer list.finish(added)
			added.run(runCtx)
		}()
		return stop
	}
	list.held = append(list.held, added)
	list.startDue()
	return true, !added.isRunning()
}

// byPriority is held work, the highest priority first and, at one priority, the first to come.
func byPriority(first, second *heldWork) int {
	return cmp.Or(cmp.Compare(second.priority, first.priority), cmp.Compare(first.arrival, second.arrival))
}

// startDue continues the paused work there is room for again, then starts each waiting job whose
// kind has a free slot and whose device a free reader, the highest priority first; play-driven work
// whose device has none pauses background work there to have one. The caller holds the mutex.
func (list *workList) startDue() {
	ordered := slices.Clone(list.held)
	slices.SortFunc(ordered, byPriority)
	list.continuePaused(ordered)
	for _, work := range ordered {
		if work.isRunning() || !list.hasFreeSlot(work) {
			continue
		}
		if !list.hasFreeReader(work.device) && work.isPlayDriven {
			list.pauseBackgroundFor(work.device, ordered)
		}
		if !list.hasFreeReader(work.device) {
			continue
		}
		list.running[work.slot]++
		work.stop = work.start()
	}
	close(list.changed)
	list.changed = make(chan struct{})
}

// continuePaused continues the paused work, by priority, whose device has a reader free again and no
// play-driven work waiting for one there. The caller holds the mutex.
func (list *workList) continuePaused(ordered []*heldWork) {
	wantedForPlay := map[string]bool{}
	for _, work := range ordered {
		if !work.isRunning() && work.isPlayDriven && work.device != "" && list.hasFreeSlot(work) {
			wantedForPlay[work.device] = true
		}
	}
	for _, work := range ordered {
		if work.isRunning() && work.pausing.IsPaused() && !wantedForPlay[work.device] &&
			list.hasFreeReader(work.device) {
			work.pausing.Continue()
			work.movement.resume()
		}
	}
}

// hasFreeSlot is whether a slot of the work's kind is free: always, for work that holds none, a listing
// or an examining, which waits only for its device's reader. The caller holds the mutex.
func (list *workList) hasFreeSlot(work *heldWork) bool {
	return work.slot == "" || list.running[work.slot] < list.capacity.SlotCount(work.slot)
}

// hasFreeReader is whether a device has a reader free: always, for work that reads none. The caller
// holds the mutex.
func (list *workList) hasFreeReader(device string) bool {
	if device == "" {
		return true
	}
	limit, isNamed := list.readerLimits[device]
	if !isNamed {
		limit = 1
	}
	reading := 0
	for _, work := range list.held {
		if work.device == device && work.isReading() {
			reading++
		}
	}
	return reading < limit
}

// pauseBackgroundFor pauses the background work reading a device, the lowest priority first, until
// it has a reader free. The caller holds the mutex.
func (list *workList) pauseBackgroundFor(device string, ordered []*heldWork) {
	for _, work := range slices.Backward(ordered) {
		if list.hasFreeReader(device) {
			return
		}
		if work.device == device && !work.isPlayDriven && work.isReading() {
			work.pausing.Pause()
			work.movement.pause()
		}
	}
}

// waitWhilePaused waits while the work holding token is paused, for work that reads in the node's own
// process, between one file and the next, until ctx ends (D240 §6).
func (list *workList) waitWhilePaused(ctx context.Context, token string) error {
	for {
		list.mutex.Lock()
		work := list.find(token)
		isPaused := work != nil && work.pausing.IsPaused()
		changed := list.changed
		list.mutex.Unlock()
		if !isPaused {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting to be continued: %w", ctx.Err())
		case <-changed:
		}
	}
}

// finish forgets a job whose run ended, frees its slot and starts what is due.
func (list *workList) finish(work *heldWork) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	// stop is set under the mutex once the run has begun, which may end before then.
	work.stop()
	list.running[work.slot]--
	list.remove(work.token)
	list.startDue()
}

// cancel stops a running attempt, or drops a waiting one before it starts. It reports whether the
// attempt was held.
func (list *workList) cancel(token string) bool {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	work := list.find(token)
	if work == nil {
		return false
	}
	if work.isRunning() {
		work.stop()
	} else {
		list.remove(token)
		list.startDue()
	}
	return true
}

// reprioritize gives a held attempt its job's new priority, play-driven or not, and starts what is
// due: work no person waits for any more takes its place among the background work, and pauses
// nothing more (D234 §3).
func (list *workList) reprioritize(token string, priority int64, isPlayDriven bool) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	if work := list.find(token); work != nil {
		work.priority, work.isPlayDriven = priority, isPlayDriven
		list.startDue()
	}
}

// heldState is where an attempt the node holds is, and how long it has not moved.
type heldState struct {
	attempt
	isRunning, isPaused, saysHowFar bool
	sinceMoved                      time.Duration
}

// states are where the attempts the node holds are, in no order. Waiting work moves when the work
// running does, so a wait lasts only while something ahead of it moves (D233 §5); with nothing
// running, it has not moved since it was held.
func (list *workList) states() []heldState {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	states := make([]heldState, 0, len(list.held))
	runningSinceMoved, isAnyRunning := time.Duration(0), false
	for _, work := range list.held {
		if work.isRunning() {
			since := work.movement.sinceMoved()
			if !isAnyRunning || since < runningSinceMoved {
				runningSinceMoved = since
			}
			isAnyRunning = true
		}
	}
	for _, work := range list.held {
		since := work.movement.sinceMoved()
		if !work.isRunning() && isAnyRunning {
			since = min(since, runningSinceMoved)
		}
		states = append(states, heldState{
			attempt: work.attempt, isRunning: work.isRunning(), isPaused: work.isRunning() && work.pausing.IsPaused(),
			saysHowFar: work.saysHowFar, sinceMoved: since,
		})
	}
	return states
}

// stallStillReads tells each running job that reads in the node's own process, and is not paused,
// that it has stalled once it has not moved for window: its reading waits on its disk, which the
// node cannot stop (D233 §10).
func (list *workList) stallStillReads(window time.Duration) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	for _, work := range list.held {
		if work.stalls == nil || !work.isRunning() || work.pausing.IsPaused() {
			continue
		}
		if since := work.movement.sinceMoved(); since >= window {
			select {
			case work.stalls <- since:
			default:
			}
		}
	}
}

// find is the held attempt of a token, or nil. The caller holds the mutex.
func (list *workList) find(token string) *heldWork {
	for _, work := range list.held {
		if work.token == token {
			return work
		}
	}
	return nil
}

// remove forgets a held attempt. The caller holds the mutex.
func (list *workList) remove(token string) {
	list.held = slices.DeleteFunc(list.held, func(work *heldWork) bool { return work.token == token })
}

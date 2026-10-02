// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// slotKind names the slots a kind of work runs in (cut-a §5.3): the encodes and remuxes the node's
// config says (nodeproto.Capacity), and one each of subtitle extraction and loudness measurement,
// each of which reads a whole video, and of artwork re-encoding.
type slotKind string

const (
	slotEncode  slotKind = "encode"
	slotRemux   slotKind = "remux"
	slotExtract slotKind = "extract"
	slotMeasure slotKind = "measure"
	slotArtwork slotKind = "artwork"
)

// slotCounts is how many of each kind of work run at once on a node of capacity.
func slotCounts(capacity nodeproto.Capacity) map[slotKind]int {
	return map[slotKind]int{
		slotEncode: capacity.EncodeSlotCount, slotRemux: capacity.RemuxSlotCount,
		slotExtract: 1, slotMeasure: 1, slotArtwork: 1,
	}
}

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

// heldWork is one attempt the node holds, by its token: waiting for a slot of its kind, then
// running.
type heldWork struct {
	attempt
	priority int64
	slot     slotKind
	// saysHowFar is a prepare, which says how far it is while it runs, as FFmpeg moves on.
	saysHowFar bool
	// run does the work to its end and says how it ended to the core, on a context a cancel ends.
	run func(ctx context.Context)

	arrival int64 // the order it came in, among work of one priority
	// start runs it on the node's lifetime, which hold was given, and returns what stops it.
	start func() context.CancelFunc
	stop  context.CancelFunc
}

func (work *heldWork) isRunning() bool { return work.stop != nil }

// workList is the work the node holds, in memory (D238 §3): each job waits until a slot of its kind
// is free and starts before the others waiting at a lower priority and, at one priority, before
// those that came after it (D234 §3). A cancel stops a running job, or drops a waiting one before
// it starts.
type workList struct {
	mutex    sync.Mutex
	slots    map[slotKind]int // how many of each kind run at once
	running  map[slotKind]int // guarded by mutex
	held     []*heldWork      // guarded by mutex: waiting and running
	arrivals int64            // guarded by mutex
}

func newWorkList(capacity nodeproto.Capacity) *workList {
	return &workList{slots: slotCounts(capacity), running: map[slotKind]int{}}
}

// hold takes a job and starts what is due. It reports false, holding nothing, for an attempt it
// holds already: a command the core sent again. It says, as isWaiting, whether the job waits for a
// slot.
func (list *workList) hold(ctx context.Context, work heldWork) (isHeld, isWaiting bool) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	if list.find(work.token) != nil {
		return false, false
	}
	list.arrivals++
	work.arrival = list.arrivals
	added := &work
	added.start = func() context.CancelFunc {
		runCtx, stop := context.WithCancel(ctx)
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

// startDue starts each waiting job whose kind has a free slot, the highest priority first and, at
// one priority, the first to come. The caller holds the mutex.
func (list *workList) startDue() {
	waiting := make([]*heldWork, 0, len(list.held))
	for _, work := range list.held {
		if !work.isRunning() {
			waiting = append(waiting, work)
		}
	}
	slices.SortFunc(waiting, func(first, second *heldWork) int {
		return cmp.Or(cmp.Compare(second.priority, first.priority), cmp.Compare(first.arrival, second.arrival))
	})
	for _, work := range waiting {
		if list.running[work.slot] >= list.slots[work.slot] {
			continue
		}
		list.running[work.slot]++
		work.stop = work.start()
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
	}
	return true
}

// heldState is where an attempt the node holds is.
type heldState struct {
	attempt
	isRunning, saysHowFar bool
}

// states are where the attempts the node holds are, in no order.
func (list *workList) states() []heldState {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	states := make([]heldState, 0, len(list.held))
	for _, work := range list.held {
		states = append(states, heldState{
			attempt: work.attempt, isRunning: work.isRunning(), saysHowFar: work.saysHowFar,
		})
	}
	return states
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

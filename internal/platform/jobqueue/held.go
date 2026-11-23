// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// held are the running jobs a runner here holds: claimed, taken back from a machine, or with an
// outcome the storage could not write yet. A job its storage marks running that none holds was
// left so by a write that failed, and GiveBackUnheld returns it to the queue (D540).
type held struct {
	mutex sync.Mutex
	// countByID is how many holds each job has: a runner's, and its unwritten outcome's.
	countByID map[int64]int // guarded by mutex
	// unwritten are the outcomes the storage refused for want of room, oldest first, each written
	// once its probe finds room (S77). Each holds its job.
	unwritten []Outcome // guarded by mutex
	// unheldSince is when GiveBackUnheld first found each running job none held.
	unheldSince map[int64]time.Time // guarded by mutex
}

// GivenBackDelay is how long a job given back for want of a write waits before it is claimed again:
// a write that fails every time does not run its job in a loop, and its attempts still count, so
// it ends at its limit (D540).
const GivenBackDelay = 30 * time.Second

// UnheldRunningGrace is how long a running job no runner holds is left before GiveBackUnheld gives
// it back: a claim marks its job running a moment before its runner holds it (D540).
const UnheldRunningGrace = time.Minute

func (queue *Queue[T]) hold(id int64) {
	queue.held.mutex.Lock()
	defer queue.held.mutex.Unlock()
	queue.held.countByID[id]++
}

func (queue *Queue[T]) release(id int64) {
	queue.held.mutex.Lock()
	defer queue.held.mutex.Unlock()
	queue.held.countByID[id]--
	if queue.held.countByID[id] <= 0 {
		delete(queue.held.countByID, id)
	}
}

// keepUnwritten keeps an outcome the storage refused for want of room, holding its job until it
// is written.
func (queue *Queue[T]) keepUnwritten(outcomes ...Outcome) {
	queue.held.mutex.Lock()
	defer queue.held.mutex.Unlock()
	for _, outcome := range outcomes {
		queue.held.countByID[outcome.ID]++
	}
	queue.held.unwritten = append(outcomes, queue.held.unwritten...)
}

// takeUnwritten takes the oldest outcome left unwritten, if any is.
func (queue *Queue[T]) takeUnwritten() (Outcome, bool) {
	queue.held.mutex.Lock()
	defer queue.held.mutex.Unlock()
	if len(queue.held.unwritten) == 0 {
		return Outcome{}, false
	}
	outcome := queue.held.unwritten[0]
	queue.held.unwritten = queue.held.unwritten[1:]
	return outcome, true
}

func (queue *Queue[T]) hasUnwritten() bool {
	queue.held.mutex.Lock()
	defer queue.held.mutex.Unlock()
	return len(queue.held.unwritten) > 0
}

// writeUnwritten writes the outcomes the storage failed to write before, oldest first, when the
// storage's probe is due, and says whether nothing is left unwritten. Every runner asks, and any
// writes them: the one that holds the storage's probe.
func (queue *Queue[T]) writeUnwritten(ctx context.Context) (bool, error) {
	if !queue.hasUnwritten() {
		return true, nil
	}
	storage := queue.storageDestination
	queue.claimMutex.Lock()
	isProbe := queue.beginProbe(storage, queue.clock.Now())
	queue.claimMutex.Unlock()
	if !isProbe && queue.isParked(storage) {
		return false, nil
	}
	for {
		outcome, isLeft := queue.takeUnwritten()
		if !isLeft {
			break
		}
		isWritten, err := queue.storage.Record(ctx, outcome)
		if err != nil && queue.isStorageFailure(err) {
			queue.keepUnwritten(outcome)
			queue.release(outcome.ID)
			queue.Park(ctx, Parking{Destination: storage, Reason: storageReason(err), IsProbeFailure: isProbe})
			return false, nil
		}
		if err != nil {
			if isProbe {
				queue.endProbeUnanswered(ctx, storage)
			}
			err = fmt.Errorf("record job %d's outcome: %w", outcome.ID, err)
			err = errors.Join(err, queue.giveBack(ctx, outcome.ID, err))
			queue.release(outcome.ID)
			return false, err
		}
		queue.release(outcome.ID)
		if !isWritten {
			queue.tellCancelled(ctx, outcome.ID)
		}
	}
	queue.Unpark(ctx, storage)
	return true, nil
}

// giveBack returns a running job whose outcome could not be written to the queue, so nothing
// leaves it running (D540): written whatever became of the context its outcome came in, keeping
// the attempt its claim counted, so a job whose outcome is never written still ends at its limit.
// One the storage refuses for want of room parks the storage, and the job,
// once none holds it, is given back by GiveBackUnheld when there is room.
func (queue *Queue[T]) giveBack(ctx context.Context, id int64, cause error) error {
	ctx = context.WithoutCancel(ctx)
	at := queue.clock.Now().Add(queue.jittered(ctx, GivenBackDelay))
	_, err := queue.storage.GiveBack(ctx, id, at, "given back: "+cause.Error())
	if err != nil && queue.isStorageFailure(err) {
		queue.Park(ctx, Parking{Destination: queue.storageDestination, Reason: storageReason(err)})
		return nil
	}
	if err != nil {
		return fmt.Errorf("give job %d back to the queue: %w", id, err)
	}
	return nil
}

// tellCancelled tells the queue's cancel hook of a job whose outcome found it cancelled: the
// cancel's own word on it may have come while its handler still gave its work away, as to a node
// (D540).
func (queue *Queue[T]) tellCancelled(ctx context.Context, id int64) {
	queue.mutex.Lock()
	cancelled := queue.cancelled
	queue.mutex.Unlock()
	if cancelled != nil {
		cancelled(ctx, id)
	}
}

// GiveBackUnheld returns to the queue each job the storage marks running that no runner here has
// held since an earlier call UnheldRunningGrace or more before, and says how many it gave back: a
// job a failed write left running, which every request it covers waits on, until a restart
// (D540). It is a sweep's (internal/core/jobs).
func (queue *Queue[T]) GiveBackUnheld(ctx context.Context) (int, error) {
	ids, err := queue.storage.RunningIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the running jobs: %w", err)
	}
	now := queue.clock.Now()
	var due []int64
	queue.held.mutex.Lock()
	unheldSince := make(map[int64]time.Time, len(ids))
	for _, id := range ids {
		if queue.held.countByID[id] > 0 {
			continue
		}
		since, wasUnheld := queue.held.unheldSince[id]
		if !wasUnheld {
			since = now
		}
		unheldSince[id] = since
		if now.Sub(since) >= UnheldRunningGrace {
			due = append(due, id)
		}
	}
	queue.held.unheldSince = unheldSince
	queue.held.mutex.Unlock()
	givenBack := 0
	for _, id := range due {
		isGivenBack, err := queue.storage.GiveBack(ctx, id, now, "given back: no runner held it")
		if err != nil {
			return givenBack, fmt.Errorf("give job %d back to the queue: %w", id, err)
		}
		if isGivenBack {
			givenBack++
			queue.logger.WarnContext(ctx, "a running job no runner held is given back to the queue", "jobId", id)
		}
	}
	if givenBack > 0 {
		queue.Wake()
	}
	return givenBack, nil
}

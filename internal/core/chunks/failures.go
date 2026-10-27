// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"
)

// ErrCannotBeMade is a chunk whose runs FFmpeg failed on the source, at the same chunk, as often as
// runFailuresToGiveUp: another run would fail the same way, so a player asking for it is told the
// film cannot play rather than asked to ask again for as long as it waits (file 15102 on nas.example,
// 2026-10-27, its sound queued again every 4.4 s for as long as its player waited).
var ErrCannotBeMade = errors.New("FFmpeg failed on the film at this chunk each time it was made")

// runFailuresToGiveUp is how many runs of a store failing at the same chunk stop it being asked of
// its node again: the second, so a failure FFmpeg met once by chance, a read error over a network
// share, is tried again once.
const runFailuresToGiveUp = 2

// runFailure is how often runs of a store failed at one chunk, and FFmpeg's word on the last.
type runFailure struct {
	count int
	why   string
}

// failed records that a run of the store at dir failed at chunk, FFmpeg failing on the source for
// why, and returns how often runs of it have failed there.
func (tracker *runTracker) failed(dir string, chunk int, why string) int {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	if known.failureByChunk == nil {
		known.failureByChunk = map[int]*runFailure{}
	}
	failure, found := known.failureByChunk[chunk]
	if !found {
		failure = &runFailure{}
		known.failureByChunk[chunk] = failure
	}
	failure.count++
	failure.why = why
	return failure.count
}

// givenUp is FFmpeg's word on the last run of the store at dir that failed at chunk, and whether
// runs failed there often enough that no other is made (runFailuresToGiveUp).
func (tracker *runTracker) givenUp(dir string, chunk int) (why string, isGivenUp bool) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	failure, found := tracker.storeLocked(dir).failureByChunk[chunk]
	if !found || failure.count < runFailuresToGiveUp {
		return "", false
	}
	return failure.why, true
}

// recordFailure takes a run of the store at dir that FFmpeg failed on at chunk, for why, and logs
// it once its runs have failed there often enough that no other is made: what FFmpeg said is the
// reason its players are told it cannot play.
func (service *Service) recordFailure(ctx context.Context, dir string, chunk int, why string) {
	count := service.runs.failed(dir, chunk, why)
	if count == runFailuresToGiveUp {
		service.logger.WarnContext(ctx, "runs of chunks failed at the same chunk again; it is not made again "+
			"until the film is played anew", "storeDir", dir, "chunk", chunk, "failures", count, "why", why)
	}
}

// refuseGivenUp is ErrCannotBeMade for a run planned from chunk first of the store at dir whose runs
// failed there often enough (givenUp), forgetting the run; nil otherwise.
func (service *Service) refuseGivenUp(dir string, first int) error {
	why, isGivenUp := service.runs.givenUp(dir, first)
	if !isGivenUp {
		return nil
	}
	service.runs.forget(dir, first)
	return fmt.Errorf("%s chunk %d: %w: %s", dir, first, ErrCannotBeMade, why)
}

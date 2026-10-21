// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// How a run of chunks is watched to stay faster than playback, or the players waiting on it never
// catch up: the owner, "Live transcoding only works when it's faster than playback" (D250 §6).
// The window a run's speed is measured over is chunk_run_speed_window_seconds (chunkTiming); a run must
// have been sampled over half of it to be judged.

// ErrNoRoomToMake is a store whose run was evicted to keep its node's other runs faster than
// playback (D250 §6): its players are told why their playback stops, until a stall asks for it
// again.
var ErrNoRoomToMake = errors.New("its node has no room to make it as fast as it plays")

// speedSample is how much of the film a run had made at a moment: the end of its last chunk made.
type speedSample struct {
	at        time.Time
	madeEndMs int64
}

// sample records that the run of the store from first had made the film through madeEndMs at now,
// keeping the samples of the last two speed windows.
func (tracker *runTracker) sample(dir string, first int, now time.Time, madeEndMs int64) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	each, found := tracker.storeLocked(dir).runByFirst[first]
	if !found {
		return
	}
	each.samples = append(slices.DeleteFunc(each.samples, func(old speedSample) bool {
		return now.Sub(old.at) > 2*tracker.timing.speedWindow
	}), speedSample{at: now, madeEndMs: madeEndMs})
}

// speedAt is the run's speed at now, film made per wall time over the last window (6.2 is 6.2 times
// playback), and whether it has been sampled over half the window, long enough to say.
func (each *run) speedAt(now time.Time, window time.Duration) (float64, bool) {
	from := slices.IndexFunc(each.samples, func(sample speedSample) bool {
		return now.Sub(sample.at) <= window
	})
	if from < 0 {
		return 0, false
	}
	first, latest := each.samples[from], each.samples[len(each.samples)-1]
	if latest.at.Sub(first.at) < window/2 {
		return 0, false
	}
	return float64(latest.madeEndMs-first.madeEndMs) / float64(latest.at.Sub(first.at).Milliseconds()), true
}

// runningRun is a run a node is making, as the capacity watch sees it.
type runningRun struct {
	dir                 string
	first, last         int
	jobID               int64
	startedAt           time.Time
	speed               float64
	isMeasured, isInUse bool
}

// runningOn is every run nodeID is making at now, with its speed where it is measured and whether a
// player wants it, by interestsOf the film's (D309).
func (tracker *runTracker) runningOn(
	nodeID int64, now time.Time, interestsOf func(fileID int64) ([]Interest, bool), keptAheadMs int64,
) []runningRun {
	tracker.mutex.Lock()
	var running []runningRun
	for dir, known := range tracker.storeByDir {
		for _, each := range known.runByFirst {
			if each.nodeID != nodeID || each.jobID == 0 {
				continue
			}
			speed, isMeasured := each.speedAt(now, tracker.timing.speedWindow)
			running = append(running, runningRun{
				dir: dir, first: each.first, last: each.last, startedAt: each.startedAt, speed: speed,
				isMeasured: isMeasured, isInUse: true, jobID: each.jobID,
			})
		}
	}
	tracker.mutex.Unlock()
	// The interests are asked for outside the tracker's lock: the playbacks keep them under their own.
	for index, each := range running {
		if interests, isKnown := interestsOf(fileOfStore(each.dir)); isKnown {
			running[index].isInUse = tracker.isWanted(each.dir, each.first, interests, keptAheadMs)
		}
	}
	return running
}

// evict forgets the run of the store from first, evicted from nodeID at now, and marks its store
// evicted, so its players are told (ErrNoRoomToMake).
func (tracker *runTracker) evict(dir string, first int, nodeID int64, now time.Time) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	tracker.cancelLocked(known, first)
	known.evictedAt = now
	tracker.evictedOnAt[nodeID] = now
}

// drop forgets the run of the store from first, cancelled at now on nodeID since its players moved
// onto another quality: its store is not evicted, and the node is left to settle as after an
// eviction.
func (tracker *runTracker) drop(dir string, first int, nodeID int64, now time.Time) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.cancelLocked(tracker.storeLocked(dir), first)
	tracker.evictedOnAt[nodeID] = now
}

// settle leaves nodeID's runs to settle from now, as after an eviction: its other work was stopped.
func (tracker *runTracker) settle(nodeID int64, now time.Time) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.evictedOnAt[nodeID] = now
}

// lastEvictionOn is when a run was last evicted from nodeID; zero for never.
func (tracker *runTracker) lastEvictionOn(nodeID int64) time.Time {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	return tracker.evictedOnAt[nodeID]
}

// isEvicted is whether a run of the store was evicted, and not asked for again by a stall since.
func (tracker *runTracker) isEvicted(dir string) bool {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	return !tracker.storeLocked(dir).evictedAt.IsZero()
}

// unevict clears the store's eviction: a player's stall asks for it again, a new try.
func (tracker *runTracker) unevict(dir string) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.storeLocked(dir).evictedAt = time.Time{}
}

// keepRunsFast keeps the runs nodeID is making faster than playback (D250 §6): when one of two or
// more measures slower than chunk_run_slowest_speed_percent of it, one run is cancelled: the oldest no
// player wants, its store refusing no one; or, when players want every one, the newest, its players
// moved down a quality (D309 §6), or, when one cannot move, evicted, and its players told why their
// playback stops. After any the node's runs are left chunk_run_speed_window_seconds to settle.
func (service *Service) keepRunsFast(ctx context.Context, nodeID int64) error {
	now := service.clock.Now()
	timing := service.refreshChunkTiming(ctx)
	if last := service.runs.lastEvictionOn(nodeID); !last.IsZero() && now.Sub(last) < timing.speedWindow {
		return nil
	}
	slowestPercent, err := service.settings.Int(ctx, settings.ChunkRunSlowestSpeedPercent)
	if err != nil {
		return fmt.Errorf("read %s: %w", settings.ChunkRunSlowestSpeedPercent, err)
	}
	keptAhead, err := service.settings.Duration(ctx, settings.ChunkKeptAheadSeconds)
	if err != nil {
		return fmt.Errorf("read %s: %w", settings.ChunkKeptAheadSeconds, err)
	}
	running := service.runs.runningOn(nodeID, now, service.interestsOf, keptAhead.Milliseconds())
	isTooSlow := func(each runningRun) bool { return each.isMeasured && each.speed*100 < float64(slowestPercent) }
	if service.holdBackFor(ctx, nodeID, running, isTooSlow, now.Add(timing.speedWindow)) {
		service.runs.settle(nodeID, now)
		return nil
	}
	if len(running) < 2 || !slices.ContainsFunc(running, isTooSlow) {
		return nil
	}
	victim := evictee(running)
	speeds := make([]string, 0, len(running))
	for _, each := range running {
		speeds = append(speeds, fmt.Sprintf("%s@%d:%.2f", each.dir, each.first, each.speed))
	}
	if !victim.isInUse {
		// No player is left without its chunks: its store refuses no one, and a player that comes to
		// them later starts a run of its own. Marked evicted, a quality a player went on to play was
		// refused it for good, the run its own chunks needed being the one no one had asked for yet
		// (the owner's friend, 2026-10-12).
		service.runs.drop(victim.dir, victim.first, nodeID, now)
		service.logger.WarnContext(ctx, "cancelled a run of chunks no one played from: its node could not "+
			"keep every run faster than playback", "nodeId", nodeID, "storeDir", victim.dir,
			"firstChunk", victim.first, "lastChunk", victim.last, "slowestPercent", slowestPercent,
			"runSpeeds", speeds)
		return service.queue.Cancel(ctx, victim.jobID)
	}
	if service.isMovedOff(victim) {
		service.runs.drop(victim.dir, victim.first, nodeID, now)
		service.logger.WarnContext(ctx, "moved a run's players down a quality: its node could not "+
			"keep every run faster than playback", "nodeId", nodeID, "storeDir", victim.dir,
			"firstChunk", victim.first, "lastChunk", victim.last, "slowestPercent", slowestPercent,
			"runSpeeds", speeds)
		return service.queue.Cancel(ctx, victim.jobID)
	}
	service.runs.evict(victim.dir, victim.first, nodeID, now)
	service.logger.WarnContext(ctx, "evicted a run of chunks: its node could not keep every run faster than playback",
		"nodeId", nodeID, "storeDir", victim.dir, "firstChunk", victim.first, "lastChunk", victim.last,
		"wasInUse", victim.isInUse, "slowestPercent", slowestPercent, "runSpeeds", speeds)
	return service.queue.Cancel(ctx, victim.jobID)
}

// holdBackFor holds back the node's other work, to the time until, when a run players want of a
// quality, an encode, is too slow on a node that encodes in software (D324 §5): the node stops the work no one
// plays and is given none, so the played encode has the CPU. It reports whether it stopped any, after
// which the node's runs are left to settle before any is evicted.
func (service *Service) holdBackFor(
	ctx context.Context, nodeID int64, running []runningRun, isTooSlow func(runningRun) bool, until time.Time,
) bool {
	if service.holdBack == nil || (service.encoders != nil && service.encoders.EncodesOnAGPU(nodeID)) {
		return false
	}
	isPlayedEncodeBehind := slices.ContainsFunc(running, func(each runningRun) bool {
		return each.isInUse && isQualityStore(each.dir) && isTooSlow(each)
	})
	if !isPlayedEncodeBehind {
		return false
	}
	stopped := service.holdBack.HoldBackFor(ctx, nodeID, until)
	if stopped > 0 {
		service.logger.WarnContext(ctx, "stopped a node's other work: the film it encodes for a player, in "+
			"software, fell behind playback", "nodeId", nodeID, "stoppedCount", stopped, "heldBackUntil", until)
	}
	return stopped > 0
}

// isMovedOff is whether every player of a run in use, of a quality made for players, was moved onto
// the quality below, so the run is no longer needed and is cancelled rather than evicted: an eviction
// stops its players (D286 §4, D309 §6).
func (service *Service) isMovedOff(victim runningRun) bool {
	fileID := fileOfStore(victim.dir)
	streamName, _ := streamOfStore(victim.dir, fileID)
	return victim.isInUse && service.movePlayers != nil && isQualityStore(victim.dir) &&
		service.movePlayers(fileID, streamName)
}

// isQualityStore is whether a store's directory is of a quality, a picture encoded for players
// (its stream "q…"), rather than the film's own picture copied or a sound.
func isQualityStore(dir string) bool {
	fileID := fileOfStore(dir)
	streamName, isStore := streamOfStore(dir, fileID)
	return isStore && fileID != 0 && strings.HasPrefix(streamName, "q")
}

// evictee is the run to evict of those running: the oldest no player wants, or, when players want
// every one, the newest (D250 §6).
func evictee(running []runningRun) runningRun {
	byAge := slices.Clone(running)
	slices.SortFunc(byAge, func(a, b runningRun) int { return a.startedAt.Compare(b.startedAt) })
	for _, each := range byAge {
		if !each.isInUse {
			return each
		}
	}
	return byAge[len(byAge)-1]
}

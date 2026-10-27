// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"time"
)

// How far a player may land ahead of a run before the run is no longer its own (D353, amending D309
// §2): waiting for the run to make the film between costs the gap over the run's speed; stopping it,
// for the player's ask to start one where it landed, costs a new run's start. A run is left where
// waiting costs less, and stopped where it costs more.

// unmeasuredJumpGapMs is the gap past which a player is taken to have jumped away from a run not yet
// measured, its speed or its store's start unknown: Jellyfin's rule, a new transcode once the segment
// asked for is 24 s of film past the one being made (segmentGapRequiringTranscodingChange in
// DynamicHlsController.cs). A constant: the measured rule replaces it as soon as the run has made two
// chunks, so it governs only a run's first seconds, where a setting would tune nothing.
const unmeasuredJumpGapMs = 24_000

// jumpFloorChunks is how many chunks past the one a run makes next a player may want without stopping
// it, however fast the run starts again, so a run is never stopped by its own stream's next asks, the
// restart loop the minute before D353 kept off: a chunk a player was served is known made at once,
// and the run's progress, which says what it makes next, follows on the node's next word, so a
// player's next ask may read a chunk past it (D355 §4). Two chunks, one for that and one of slack; a
// constant, as it follows how the node's word arrives, not a judgement an admin could make.
const jumpFloorChunks = 2

// wantRules are what a run's wantedness is read by (isWantedLocked): how far a stopped player keeps
// ahead, how far a run is planned ahead of a player, and, for a jump, now and the window its run's
// speed is measured over.
type wantRules struct {
	keptAheadMs, lookaheadMs int64
	now                      time.Time
	speedWindow              time.Duration
}

// rulesAt are the wantRules of the timing for a stopped player keeping keptAheadMs ahead, at now.
func (timing chunkTiming) rulesAt(keptAheadMs int64, now time.Time) wantRules {
	return wantRules{
		keptAheadMs: keptAheadMs, lookaheadMs: timing.lookaheadMs, now: now, speedWindow: timing.speedWindow,
	}
}

// jumpJudgement is how a run was judged against the first chunk a player needs past what it makes:
// the numbers a stop for a jump is logged with, so a report is explained from the log alone.
type jumpJudgement struct {
	// making is the chunk the run makes next, and needed the first its player needs not made.
	making, needed int
	// gapMs is the film between them; thresholdMs the gap past which the player jumped away.
	gapMs, thresholdMs int64
	// speed is the run's film made per wall time, and startupMs its store's last run's time from its
	// start to its first chunk; isMeasured whether both are known, or the threshold is Jellyfin's.
	speed      float64
	startupMs  int64
	isMeasured bool
}

// attributes are what a log says of a jump judged.
func (judged jumpJudgement) attributes() []any {
	rule := "measured: waiting gapMs/encoderSpeed costs more than a new run's startupMs"
	if !judged.isMeasured {
		rule = "not measured yet: Jellyfin's 24 s of film"
	}
	return []any{
		"makingChunk", judged.making, "neededChunk", judged.needed, "gapMs", judged.gapMs,
		"encoderSpeed", judged.speed, "startupMs", judged.startupMs, "thresholdMs", judged.thresholdMs,
		"isMeasured", judged.isMeasured, "rule", rule,
	}
}

// judgeJumpLocked is how the run is judged against unmade, the first chunk a player needs that is not
// made, and whether the player jumped away from it (D353): unmade is past what the run makes next by
// more than the threshold, the run's speed times its store's last start where both are measured,
// unmeasuredJumpGapMs where not, and never under jumpFloorChunks. The player's position is its own
// stream's (D355), so a picture loaded ahead of a late sound never moves the sound's player away
// from the sound's run. Locked by its caller.
func (known *storeRuns) judgeJumpLocked(grid Grid, each *run, unmade int, rules wantRules) (jumpJudgement, bool) {
	making := max(each.first, each.madeThrough+1)
	if making >= unmade || making > grid.lastChunk() {
		return jumpJudgement{}, false
	}
	floorMs := grid.endMs(min(making+jumpFloorChunks-1, grid.lastChunk())) - grid.StartsMs[making]
	judged := jumpJudgement{
		making: making, needed: unmade, gapMs: grid.StartsMs[unmade] - grid.StartsMs[making],
		startupMs: known.startupMs, thresholdMs: unmeasuredJumpGapMs,
	}
	speed, hasSpeed := each.speedOver(rules.now, rules.speedWindow, 0)
	judged.speed = speed
	if hasSpeed && known.hasStartup {
		judged.isMeasured = true
		judged.thresholdMs = int64(speed * float64(known.startupMs))
	}
	judged.thresholdMs = max(judged.thresholdMs, floorMs)
	return judged, judged.gapMs > judged.thresholdMs
}

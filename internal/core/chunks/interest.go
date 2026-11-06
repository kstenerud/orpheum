// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// Interest is what one player wants of a film's chunks: one stream, from a position, and, once it
// stopped, paused or gone from its page, only chunk_kept_ahead_seconds after it (D309 §1, §7). A
// player wants its picture and its sound, each an interest; a stall's offer is another, the quality
// made ready for it (D247 §2).
type Interest struct {
	// StreamName is the stream's name, as its store names it (Stream.Name).
	StreamName string
	// PositionMs is where in the film the player wants it from.
	PositionMs int64
	// IsStopped is whether the player paused or left its page, and wants only chunk_kept_ahead_seconds
	// of film after PositionMs made.
	IsStopped bool
}

// Interests is every interest the players of a film hold now: internal/core/playback keeps them,
// each lapsing when its player falls silent (D309 §3).
type Interests func(fileID int64) []Interest

// InterestsBy is where the runs learn who wants them (D309). Set once, before the server serves;
// unset, every run is wanted.
func (service *Service) InterestsBy(interests Interests) { service.interests = interests }

// interestsOf is every interest held in fileID's chunks; nil when nothing says, and every run is
// wanted.
func (service *Service) interestsOf(fileID int64) ([]Interest, bool) {
	if service.interests == nil {
		return nil, false
	}
	return service.interests(fileID), true
}

// stoppedRun is a run cutUnwanted forgot, for its caller to cancel: its store and its chunks.
type stoppedRun struct {
	RunSpan
	dir     string
	storeID int64
	// interests are what each player wanting its stream wanted as it stopped (describeInterests), and
	// filmInterests what every player of its film wanted of each stream (describeFilmInterests); nil
	// for a run not stopped for want of one.
	interests, filmInterests []string
	// jumps are how it was judged against each player that jumped away from it (D353).
	jumps []jumpJudgement
}

// StopUnwanted stops every run of the film's chunks no player wants (D309 §2): a quality its players
// moved off, a place they sought away from, what a stopped player keeps ahead already made, an
// interest lapsed. Each is cancelled, queued or running; what it made is kept (§5). It goes on when
// the request that asked ends, as a paused player's does at once: the tracker forgets a run before
// its job is cancelled, and a stop cut short between left the job making what no one wanted.
func (service *Service) StopUnwanted(ctx context.Context, fileID int64) error {
	ctx = context.WithoutCancel(ctx)
	interests, isKnown := service.interestsOf(fileID)
	if !isKnown {
		return nil
	}
	keptAhead, err := service.settings.Duration(ctx, settings.ChunkKeptAheadSeconds)
	if err != nil {
		return fmt.Errorf("read %s: %w", settings.ChunkKeptAheadSeconds, err)
	}
	now := service.clock.Now()
	for _, stopped := range service.runs.cutUnwanted(fileID, interests, keptAhead.Milliseconds(), now) {
		cancelled, err := service.cancelRun(ctx, stopped)
		if err != nil {
			return err
		}
		service.logger.InfoContext(ctx, "stopped a run of chunks no player wants",
			append(stopped.attributes(cancelled), slogx.EventKey, "unwantedChunkRunStopped")...)
		for _, jump := range stopped.jumps {
			service.logger.DebugContext(ctx, "stopped a run of chunks for a player that jumped away from it",
				append([]any{
					"storeDir", stopped.dir, "firstChunk", stopped.First, slogx.EventKey, "chunkRunJumpedAway",
				}, jump.attributes()...)...)
		}
	}
	return nil
}

// cancelRun cancels a run the tracker forgot, queued or running, and says how many jobs it was: to
// the end, whether or not the request that asked is still there, since nothing else will.
func (service *Service) cancelRun(ctx context.Context, stopped stoppedRun) (int, error) {
	cancelled, err := service.queue.CancelMatching(context.WithoutCancel(ctx), jobs.KindMakeChunks,
		jobs.Target{ChunkStoreID: stopped.storeID}, func(argsJSON string) bool {
			var args runArgs
			return json.Unmarshal([]byte(argsJSON), &args) == nil && args.FirstChunk == stopped.First
		})
	if err != nil {
		return 0, fmt.Errorf("cancel the run of %s from chunk %d: %w", stopped.dir, stopped.First, err)
	}
	return cancelled, nil
}

// attributes are what a log says of a run cancelled with its jobs.
func (stopped stoppedRun) attributes(cancelled int) []any {
	attributes := []any{
		"storeDir", stopped.dir, "firstChunk", stopped.First, "lastChunk", stopped.Last,
		"madeThrough", stopped.MadeThrough, "cancelledJobs", cancelled,
	}
	if stopped.interests != nil {
		attributes = append(attributes, "streamInterests", stopped.interests, "filmInterests", stopped.filmInterests)
	}
	return attributes
}

// describeInterests says, for a log, what each player wanting stream wants: where it plays from, or
// where it stopped and the film it keeps ahead; empty when none wants it. A run stopped as no player
// wanted it said nothing of whose want had ended, and a pause read as a playing player's run cut
// (the owner's film, 2026-10-26).
func describeInterests(stream string, interests []Interest, keptAheadMs int64) []string {
	described := []string{}
	for _, interest := range interests {
		if interest.StreamName == stream {
			described = append(described, interest.describe(keptAheadMs))
		}
	}
	return described
}

// describeFilmInterests says, for a log, what every player of a film wants of each of its streams,
// each from its own position (D355): a stream's run judged from where its player is beside where the
// player's other stream is, so a picture loaded ahead of its sound reads as such.
func describeFilmInterests(interests []Interest, keptAheadMs int64) []string {
	described := make([]string, 0, len(interests))
	for _, interest := range interests {
		described = append(described, interest.StreamName+": "+interest.describe(keptAheadMs))
	}
	return described
}

// describe says where the interest wants its stream from: where it plays from, or where it stopped
// and the film it keeps ahead.
func (interest Interest) describe(keptAheadMs int64) string {
	if interest.IsStopped {
		return fmt.Sprintf("stopped at %d ms, keeping to %d ms", interest.PositionMs, interest.PositionMs+keptAheadMs)
	}
	return fmt.Sprintf("playing from %d ms", interest.PositionMs)
}

// cutQueuedAhead forgets a run of the store still waiting in the queue that begins within
// the timing's lookaheadMs of film after chunk wanted, which is not made and no run will make, with nothing made
// or on its way between: the run planned from wanted then makes its chunks too, as one run from where
// the player is. Left, the queue folded wanted's run into it, which keeps one job of a store queued,
// and a jump that landed just before it waited for it to make chunks after the jump's (D286 §1).
func (tracker *runTracker) cutQueuedAhead(dir string, grid Grid, wanted int) (stoppedRun, bool) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	if known.isCoveredLocked(wanted) {
		return stoppedRun{}, false
	}
	next := wanted + 1
	for next <= grid.lastChunk() && grid.StartsMs[next]-grid.StartsMs[wanted] <= tracker.timing.lookaheadMs &&
		!known.isCoveredLocked(next) {
		next++
	}
	queued, found := known.runByFirst[next]
	if !found || queued.jobID != 0 {
		return stoppedRun{}, false
	}
	delete(known.runByFirst, next)
	return stoppedRun{
		RunSpan: RunSpan{First: queued.first, Last: queued.last, MadeThrough: queued.madeThrough},
		dir:     dir, storeID: known.storeID,
	}, true
}

// cutUnwanted forgets every run of fileID's stores that no interest wants (isWantedLocked), and
// returns them for the caller to cancel. A store whose grid is not known yet keeps its runs.
func (tracker *runTracker) cutUnwanted(
	fileID int64, interests []Interest, keptAheadMs int64, now time.Time,
) []stoppedRun {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	rules := tracker.timing.rulesAt(keptAheadMs, now)
	var stopped []stoppedRun
	for dir, known := range tracker.storeByDir {
		stream, isOfFile := streamOfStore(dir, fileID)
		if !isOfFile {
			continue
		}
		for first, each := range known.runByFirst {
			isWanted, jumps := known.isWantedLocked(each, stream, interests, rules)
			if isWanted {
				continue
			}
			stopped = append(stopped, stoppedRun{
				RunSpan: RunSpan{First: each.first, Last: each.last, MadeThrough: each.madeThrough},
				dir:     dir, storeID: known.storeID, interests: describeInterests(stream, interests, keptAheadMs),
				filmInterests: describeFilmInterests(interests, keptAheadMs), jumps: jumps,
			})
			tracker.cancelLocked(known, first)
		}
	}
	return stopped
}

// isWanted is whether an interest wants the run of the store from first at now (isWantedLocked); true
// for a run it no longer knows, which nothing then stops.
func (tracker *runTracker) isWanted(
	dir string, first int, interests []Interest, keptAheadMs int64, now time.Time,
) bool {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	known := tracker.storeLocked(dir)
	each, found := known.runByFirst[first]
	if !found {
		return true
	}
	stream, _ := streamOfStore(dir, fileOfStore(dir))
	isWanted, _ := known.isWantedLocked(each, stream, interests, tracker.timing.rulesAt(keptAheadMs, now))
	return isWanted
}

// isWantedLocked is whether any interest in the store's stream wants the run, by the rules: one
// playing from a chunk the run has not gone past, the run starting within lookaheadMs of film of the
// first chunk after it not made, as a run is planned (runTracker.plan); or one stopped, whose
// keptAheadMs after it the run is still to make some of; never one that jumped away from it
// (judgeJumpLocked), each said with the numbers it was judged by. A store whose grid is not known yet
// is wanted: nothing can say.
func (known *storeRuns) isWantedLocked(
	each *run, stream string, interests []Interest, rules wantRules,
) (bool, []jumpJudgement) {
	if known.grid == nil {
		return true, nil
	}
	var jumps []jumpJudgement
	grid := *known.grid
	for _, interest := range interests {
		if interest.StreamName != stream {
			continue
		}
		from := grid.chunkAt(interest.PositionMs)
		if each.last < from {
			continue
		}
		// Chunks made between the player and the run are no distance: the run makes what follows them
		// (the owner's test, 2026-10-12, where a jump onto made chunks cut the run after them).
		unmade := from
		for unmade < each.first && known.isMade[unmade] {
			unmade++
		}
		// A run the first chunk its player needs is too far past is a place the player sought away from
		// (D309 §2, D353), though its chunks reach past the player: wanted for covering a jump's chunk,
		// it kept the jump from starting a run there, and the player waited for it to make the film
		// between (the owner's film, 2026-10-26). Nearer, it is the player's place.
		if jump, isAway := known.judgeJumpLocked(grid, each, unmade, rules); isAway {
			jumps = append(jumps, jump)
			continue
		}
		if interest.IsStopped {
			end := grid.chunkAt(interest.PositionMs + rules.keptAheadMs - 1)
			if each.first <= end && each.madeThrough < end {
				return true, nil
			}
			continue
		}
		if grid.StartsMs[each.first]-grid.StartsMs[unmade] <= rules.lookaheadMs {
			return true, nil
		}
	}
	return false, jumps
}

// wantedness says whether the players' interests want a run of a store, its tracker locked.
type wantedness func(known *storeRuns, each *run) bool

// wantsEvery is the wantedness of the server's own start of a run, and where nothing says.
func wantsEvery(*storeRuns, *run) bool { return true }

// wantednessOf is what the players of fileID want of stream's runs now, for a player's ask, isAsked;
// wantsEvery for the server's own start, or where nothing says.
func (service *Service) wantednessOf(
	ctx context.Context, fileID int64, stream Stream, isAsked bool,
) (wantedness, error) {
	interests, isKnown := service.interestsOf(fileID)
	if !isAsked || !isKnown {
		return wantsEvery, nil
	}
	keptAhead, err := service.settings.Duration(ctx, settings.ChunkKeptAheadSeconds)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", settings.ChunkKeptAheadSeconds, err)
	}
	name := stream.Name()
	// Read now: the closure runs under the tracker's lock.
	rules := service.runs.timingNow().rulesAt(keptAhead.Milliseconds(), service.clock.Now())
	return func(known *storeRuns, each *run) bool {
		isWanted, _ := known.isWantedLocked(each, name, interests, rules)
		return isWanted
	}, nil
}

// knowGrid records the grid a store's chunks are cut on, which its runs' interests are read on.
func (tracker *runTracker) knowGrid(dir string, grid Grid) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.storeLocked(dir).grid = &grid
}

// streamOfStore is the name of the stream a store's directory (StoreDir) holds, and whether the store
// is fileID's.
func streamOfStore(dir string, fileID int64) (string, bool) {
	fileText, rest, _ := strings.Cut(dir, "/")
	streamDir, _, _ := strings.Cut(rest, "/")
	stream, isStore := strings.CutPrefix(streamDir, "chunks-")
	return stream, isStore && fileText == strconv.FormatInt(fileID, 10)
}

// fileOfStore is the file a store's directory (StoreDir) is of; 0 for one that names none.
func fileOfStore(dir string) int64 {
	fileText, _, _ := strings.Cut(dir, "/")
	fileID, err := strconv.ParseInt(fileText, 10, 64)
	if err != nil {
		return 0
	}
	return fileID
}

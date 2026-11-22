// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// Refusals a rescan can meet.
var (
	// ErrNotRescannable is an extra, rescanned with the film or series it is an extra of: every other
	// video, and every library, is rescanned (D535 §3, §4, amending D357 §1).
	ErrNotRescannable = errors.New("an extra is rescanned with the film or series it belongs to")
)

// RescanScans are how a rescan has its scope's files scanned as a manual scan scans them, and sees
// whether that scan is over: the library admin (library.Admin).
type RescanScans interface {
	// ScanReadingAgain asks for a scan of every root of a library that reads every file again,
	// changed on disk or not (D392 §2); isDiscarding, throwing out everything made of each, whatever
	// differs (D535 §6).
	ScanReadingAgain(ctx context.Context, libraryID int64, isDiscarding bool) (int, error)
	// ScanWork asks for a scan of the folders that hold an item's files in a library, reading them
	// again, changed on disk or not, and, isDiscarding, throwing out everything made of them.
	ScanWork(ctx context.Context, libraryID, itemID int64, isDiscarding bool) (int, error)
	// IsScanning is whether a scan of any root of a library is in progress.
	IsScanning(ctx context.Context, libraryID int64) (bool, error)
}

// A rescan is of one kind alone, D392's full one (D535 §5, amending D357 and D392 §1): it reads
// every file in its scope again, changed on disk or not, throws out what the differences from what
// was kept touch, asks again for the images its films and series are missing, then matches each
// again. The light rescan, a match again without reading the files, is gone.

// Rescans are an admin's rescans (D357): of every library of films or series, of one, or of one
// film or series. A rescan is a scan, then a match again of every film and series in its scope,
// from what its files say, as of one never matched. It is kept as a scan is, in cache.db (rescan,
// rematch), from its request until its last film or series is matched again; each of those is the
// fetch_metadata job its item's fetch runs, whichever, and Advance, the rescans' sweep, lists them
// once the scan is over and queues them.
type Rescans struct {
	matcher *Matcher
	queue   Queue
	scans   RescanScans
	// onRequested asks for the rescans' sweep soon (OnRequested); nil asks for none.
	onRequested func()
	// givenUp forgets what another package keeps of the attempts given up on in a rescan's scope
	// (SetGivenUpForgetters, D535 §8).
	givenUp GivenUpForgetters
	// waits is how many sweeps in a row found each rescan's library still changing, by rescan id;
	// guarded by waitsMutex. A restart begins the count again.
	waits      map[int64]int
	waitsMutex sync.Mutex
}

// rescanWaitsBeforeGivingUp is how many sweeps in a row may find a rescan's library still changing,
// each having filed it filesSayReads times, before the rescan gives up: a library no pass settles
// is a bug, which waiting longer does not fix (the owner, 2026-11-13). The sweep runs about every
// ten seconds, so a minute or so.
const rescanWaitsBeforeGivingUp = 6

// waited counts one more sweep that found a rescan's library still changing, and says whether the
// rescan has waited as long as it may.
func (rescans *Rescans) waited(rescanID int64) bool {
	rescans.waitsMutex.Lock()
	defer rescans.waitsMutex.Unlock()
	if rescans.waits == nil {
		rescans.waits = map[int64]int{}
	}
	rescans.waits[rescanID]++
	if rescans.waits[rescanID] < rescanWaitsBeforeGivingUp {
		return false
	}
	delete(rescans.waits, rescanID)
	return true
}

// stoppedWaiting forgets the sweeps a rescan waited, its library read.
func (rescans *Rescans) stoppedWaiting(rescanID int64) {
	rescans.waitsMutex.Lock()
	defer rescans.waitsMutex.Unlock()
	delete(rescans.waits, rescanID)
}

// NewRescans returns the rescans, matching through matcher and queuing on queue.
func NewRescans(matcher *Matcher, queue Queue, scans RescanScans) *Rescans {
	return &Rescans{matcher: matcher, queue: queue, scans: scans}
}

// OnRequested has a rescan, once asked for, ask for the rescans' sweep: the job queue's, set by the
// server's wiring.
func (rescans *Rescans) OnRequested(ask func()) { rescans.onRequested = ask }

// RescanEverything rescans every live library, whatever it holds (D357 §1, D535 §4), and says how
// many rescans it started: a library already being rescanned starts none. With isDiscarding, each
// throws out what was made, as RescanLibrary says.
func (rescans *Rescans) RescanEverything(ctx context.Context, isDiscarding bool) (int, error) {
	libraryIDs, err := rescans.matcher.database.Read().ListLiveLibraryIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the libraries: %w", err)
	}
	started := 0
	for _, libraryID := range libraryIDs {
		isStarted, err := rescans.RescanLibrary(ctx, libraryID, isDiscarding)
		if err != nil {
			return started, err
		}
		if isStarted {
			started++
		}
	}
	return started, nil
}

// RescanLibrary rescans a library, and says whether it started one: a second press while one runs
// adds nothing (D357 §5, D392 §6). It reads every file again (D535 §5); a library of other videos has
// nothing in it matched, other videos never are (D535 §4). It forgets every attempt given up on in it,
// discarding or not (D535 §8). With isDiscarding, it throws out everything
// the app made or fetched for its files, whatever differs: what was made of each file, its films' and
// series' kept search results and images, and their records fetched again; never what a person
// chose, edited or watched (D535 §6).
func (rescans *Rescans) RescanLibrary(ctx context.Context, libraryID int64, isDiscarding bool) (bool, error) {
	_, err := store.OrNotFound(rescans.matcher.database.Read().GetLibraryForIdentify(ctx, libraryID))
	if errors.Is(err, store.ErrNotFound) {
		return false, fmt.Errorf("library %d: %w", libraryID, library.ErrNoLibrary)
	}
	if err != nil {
		return false, fmt.Errorf("read library %d: %w", libraryID, err)
	}
	isStarted, err := rescans.start(ctx, libraryID, nil, isDiscarding)
	if err != nil || !isStarted {
		return false, err
	}
	if err := rescans.forgetGivenUp(ctx, libraryID, nil); err != nil {
		return true, err
	}
	if _, err := rescans.scans.ScanReadingAgain(ctx, libraryID, isDiscarding); err != nil {
		return true, fmt.Errorf("scan library %d to rescan it: %w", libraryID, err)
	}
	rescans.askForTheSweep()
	return true, nil
}

// RescanItem rescans one film, series, episode or other video, and says whether it started one: none
// while its own rescan or its library's runs (D357 §5, D392 §6). It reads its files again, a series'
// every episode's (D535 §5); an episode or an other video is not matched: an episode is matched
// through its series, and an other video never (D535 §3). It forgets every attempt given up on for it
// and its files (D535 §8). With isDiscarding, it throws out what was
// made of its files and for it, as RescanLibrary says (D535 §6).
func (rescans *Rescans) RescanItem(ctx context.Context, itemID int64, isDiscarding bool) (bool, error) {
	item, err := rescans.matcher.readRescanned(ctx, itemID)
	if err != nil {
		return false, err
	}
	libraryID := ptr.ValueOrZero(item.libraryID())
	running, err := rescans.matcher.cache.Read().CountRescansOfLibraryOrItem(ctx,
		cachegen.CountRescansOfLibraryOrItemParams{LibraryID: libraryID, ItemID: &itemID})
	if err != nil {
		return false, fmt.Errorf("read item %d's rescans: %w", itemID, err)
	}
	if _, isWaiting, rematchErr := rescans.matcher.pendingRematch(ctx, itemID); rematchErr != nil || isWaiting ||
		running > 0 {
		return false, rematchErr
	}
	isStarted, err := rescans.start(ctx, libraryID, &itemID, isDiscarding)
	if err != nil || !isStarted {
		return false, err
	}
	if err := rescans.forgetGivenUp(ctx, libraryID, &itemID); err != nil {
		return true, err
	}
	if _, err := rescans.scans.ScanWork(ctx, libraryID, itemID, isDiscarding); err != nil {
		return true, fmt.Errorf("scan item %d's folders to rescan it: %w", itemID, err)
	}
	rescans.askForTheSweep()
	return true, nil
}

// IsRescanning says whether a rescan covers a film, series, episode or other video: its own, its
// library's or every library's, or, given an episode's series, the series'. It does from the press
// until the rescan ends, matched again; its page says so while it lasts, in place of a notice that
// came seconds after the press (the owner, 2026-11-22). An extra is never rescanned.
func (rescans *Rescans) IsRescanning(ctx context.Context, itemID int64, seriesID *int64) (bool, error) {
	if isRunning, err := rescans.matcher.isRescanRunning(ctx); err != nil || !isRunning {
		return false, err
	}
	item, err := rescans.matcher.readRescanned(ctx, itemID)
	if errors.Is(err, ErrNotRescannable) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	libraryID := ptr.ValueOrZero(item.libraryID())
	for _, covered := range []*int64{&itemID, seriesID} {
		if covered == nil {
			continue
		}
		running, err := rescans.matcher.cache.Read().CountRescansOfLibraryOrItem(ctx,
			cachegen.CountRescansOfLibraryOrItemParams{LibraryID: libraryID, ItemID: covered})
		if err != nil {
			return false, fmt.Errorf("read item %d's rescans: %w", *covered, err)
		}
		if running > 0 {
			return true, nil
		}
	}
	return false, nil
}

// readRescanned reads the film, series, episode or other video a rescan names: never an extra.
func (matcher *Matcher) readRescanned(ctx context.Context, itemID int64) (target, error) {
	item, err := matcher.readTarget(ctx, itemID)
	if err != nil {
		return target{}, err
	}
	if !item.isSeries && item.title.Kind == library.TitleKindExtra {
		return target{}, fmt.Errorf("item %d, a %s: %w", itemID, item.title.Kind, ErrNotRescannable)
	}
	if item.libraryID() == nil {
		return target{}, fmt.Errorf("item %d: %w", itemID, ErrNoItem)
	}
	return item, nil
}

// start records a rescan of a scope, a library or one film or series in it, unless one of that
// scope runs, and says whether it did; isDiscarding, one that discards what was made (D535 §6).
func (rescans *Rescans) start(
	ctx context.Context, libraryID int64, itemID *int64, isDiscarding bool,
) (bool, error) {
	count := &rescans.matcher.rescans
	count.mutex.Lock()
	defer count.mutex.Unlock()
	if err := rescans.matcher.countRescansLocked(ctx); err != nil {
		return false, err
	}
	discarding := int64(0)
	if isDiscarding {
		discarding = 1
	}
	isStarted := false
	err := rescans.matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		_, insertErr := store.OrNotFound(queries.InsertRescan(ctx, cachegen.InsertRescanParams{
			LibraryID: libraryID, ItemID: itemID, RequestedAt: rescans.matcher.clock.Now().UnixMilli(),
			IsFull: 1, IsDiscarding: discarding,
		}))
		if errors.Is(insertErr, store.ErrNotFound) {
			return nil // one of the scope runs
		}
		if insertErr != nil {
			return fmt.Errorf("record it: %w", insertErr)
		}
		isStarted = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("record library %d's rescan: %w", libraryID, err)
	}
	if isStarted {
		count.running++
		rescans.matcher.logger.InfoContext(ctx, "a rescan was asked for", "libraryId", libraryID,
			"itemId", ptr.ValueOrZero(itemID), "isDiscarding", isDiscarding)
	}
	return isStarted, nil
}

func (rescans *Rescans) askForTheSweep() {
	if rescans.onRequested != nil {
		rescans.onRequested()
	}
}

// rescanReason is why a rescan's match is queued, as the job view says it.
const rescanReason = "a rescan matches it again from its files"

// Advance is the rescans' sweep: it lists the films and series of each rescan whose scan is over,
// with what their files say they are, read as a pass reads them, and queues the fetch of every film
// and series a rescan waits to match again. It says how many fetches it queued.
func (rescans *Rescans) Advance(ctx context.Context) (int, error) {
	running, err := rescans.matcher.cache.Read().ListRescans(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the rescans: %w", err)
	}
	saidByLibrary := map[int64]map[int64]filesQuery{}
	for _, rescan := range running {
		if rescan.ListedAt != nil {
			continue
		}
		isScanning, err := rescans.scans.IsScanning(ctx, rescan.LibraryID)
		if err != nil {
			return 0, fmt.Errorf("see whether library %d is scanning: %w", rescan.LibraryID, err)
		}
		if isScanning {
			continue
		}
		said, err := rescans.saidFor(ctx, rescan, saidByLibrary)
		if err != nil {
			return 0, err
		}
		if said == nil {
			continue // its library still changes
		}
		rescans.stoppedWaiting(rescan.ID)
		if err := rescans.list(ctx, rescan, said); err != nil {
			return 0, err
		}
	}
	return rescans.queueRematches(ctx)
}

// saidFor is what the files a rescan matches again say: nothing for one that matches nothing, of an
// episode, an other video or a library of other videos (D535 §3, §4), whose files' scan is all it
// is; else filesSayFor's.
func (rescans *Rescans) saidFor(
	ctx context.Context, rescan cachegen.ListRescansRow, saidByLibrary map[int64]map[int64]filesQuery,
) (map[int64]filesQuery, error) {
	queries := rescans.matcher.database.Read()
	var kind string
	var err error
	if rescan.ItemID != nil {
		kind, err = store.OrNotFound(queries.GetItemKind(ctx, *rescan.ItemID))
	} else {
		var found sqlcgen.GetLibraryForIdentifyRow
		found, err = store.OrNotFound(queries.GetLibraryForIdentify(ctx, rescan.LibraryID))
		kind = found.Kind
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("read what rescan %d matches again: %w", rescan.ID, err)
	}
	if kind != KindMovie && kind != KindSeries {
		return map[int64]filesQuery{}, nil // matches nothing, or nothing is left of it: its list is empty
	}
	return rescans.filesSayFor(ctx, rescan, saidByLibrary)
}

// filesSayFor is what the files of a rescan's library say, read once a sweep for all its rescans;
// nil for a library that changed each time it was read, whose rescan waits or gives up.
func (rescans *Rescans) filesSayFor(
	ctx context.Context, rescan cachegen.ListRescansRow, saidByLibrary map[int64]map[int64]filesQuery,
) (map[int64]filesQuery, error) {
	if said, known := saidByLibrary[rescan.LibraryID]; known {
		return said, nil
	}
	said, err := rescans.matcher.pass.filesSayOf(ctx, rescan.LibraryID)
	if errors.Is(err, errLibraryKeptChanging) {
		return nil, rescans.wait(ctx, rescan, err)
	}
	if err != nil {
		return nil, err
	}
	saidByLibrary[rescan.LibraryID] = said
	return said, nil
}

// wait leaves a rescan whose library changed each time the sweep read it for the next sweep, or,
// once it has waited as long as it may, ends it given up, logging the write that kept changing the
// library, which names the item it was filing.
func (rescans *Rescans) wait(ctx context.Context, rescan cachegen.ListRescansRow, keptChanging error) error {
	if rescans.waited(rescan.ID) {
		return rescans.matcher.endRescan(ctx, rescan.ID, keptChanging)
	}
	rescans.matcher.logger.InfoContext(ctx, "a rescan waits for its library to stop changing",
		slogx.EventKey, "rescanWaiting", "libraryId", rescan.LibraryID, "cause", keptChanging)
	return nil
}

// list records the films and series a rescan matches again, with what their files say, once its
// scan is over: its one item, or every film and series of its library. One that another rescan
// already matches again is that one's.
func (rescans *Rescans) list(ctx context.Context, rescan cachegen.ListRescansRow, said map[int64]filesQuery) error {
	itemIDs := slices.Sorted(maps.Keys(said))
	if rescan.ItemID != nil {
		itemIDs = []int64{*rescan.ItemID}
	}
	var listed int64
	err := rescans.matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		listed = 0
		for _, itemID := range itemIDs {
			query, isFilmOrSeries := said[itemID]
			if !isFilmOrSeries {
				continue // no file of it left to say
			}
			idsJSON, err := json.Marshal(query.ids)
			if err != nil {
				return fmt.Errorf("encode item %d's ids: %w", itemID, err)
			}
			added, err := queries.InsertRematch(ctx, cachegen.InsertRematchParams{
				ItemID: itemID, RescanID: rescan.ID, Name: query.name, Year: int64(query.year),
				IdsJSON: string(idsJSON),
			})
			if err != nil {
				return fmt.Errorf("list item %d: %w", itemID, err)
			}
			listed += added
		}
		return queries.SetRescanListed(ctx, cachegen.SetRescanListedParams{
			ListedAt: new(rescans.matcher.clock.Now().UnixMilli()), ItemCount: listed, ID: rescan.ID,
		})
	})
	if err != nil {
		return fmt.Errorf("list rescan %d's films and series: %w", rescan.ID, err)
	}
	rescans.matcher.logger.InfoContext(ctx, "a rescan's scan is over; its films and series are matched again",
		"libraryId", rescan.LibraryID, "itemId", ptr.ValueOrZero(rescan.ItemID), "itemCount", listed,
		"isFull", rescan.IsFull != 0)
	if rescan.IsDiscarding != 0 {
		if err := rescans.matcher.discardMade(ctx, itemIDs, rescan.ItemID != nil); err != nil {
			return err
		}
	}
	if rescan.IsFull != 0 {
		if err := rescans.askForMissingImages(ctx, itemIDs, said); err != nil {
			return err
		}
	}
	if listed == 0 {
		return rescans.matcher.endRescanIfDone(ctx, rescan.ID)
	}
	return nil
}

// askForMissingImages asks again, for a full rescan, for the images its films and series, a series'
// episodes with it, are missing or were refused, which the pass at its end queues (D392 §4).
func (rescans *Rescans) askForMissingImages(ctx context.Context, itemIDs []int64, said map[int64]filesQuery) error {
	for _, itemID := range itemIDs {
		if _, isFilmOrSeries := said[itemID]; !isFilmOrSeries {
			continue
		}
		kind, err := store.OrNotFound(rescans.matcher.database.Read().GetItemKind(ctx, itemID))
		switch {
		case errors.Is(err, store.ErrNotFound):
			continue
		case err != nil:
			return fmt.Errorf("read item %d: %w", itemID, err)
		case kind == KindSeries:
			err = rescans.matcher.retrySeriesImages(ctx, itemID)
		default:
			err = rescans.matcher.retryImages(ctx, itemID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// queueRematches queues the fetch of every film and series a rescan waits to match again: one an
// admin asked for alone at a person's priority, a library's in the background. The fetch reads its
// rematch whichever fetch of the item it is.
func (rescans *Rescans) queueRematches(ctx context.Context) (int, error) {
	waiting, err := rescans.matcher.cache.Read().ListRematches(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the films and series rescans wait on: %w", err)
	}
	queued := 0
	for _, rematch := range waiting {
		priority := jobs.PriorityBackground
		if rematch.RescanItemID != nil {
			priority = jobs.PriorityPersonsRequest
		}
		added, err := rescans.queue.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: rematch.ItemID}, Destination: FetchDestination,
			Reason: rescanReason, Priority: priority, IsDerived: true,
		})
		if err != nil {
			return queued, fmt.Errorf("queue item %d's rematch: %w", rematch.ItemID, err)
		}
		if added {
			queued++
		}
	}
	return queued, nil
}

// RescanProgress is how far the rescans of one library are (D357 §5): whether one waits for its
// scan, and of the films and series listed, how many were matched again.
type RescanProgress struct {
	IsScanning   bool
	ItemCount    int64
	MatchedCount int64
}

// Progress is each library's rescans in progress, by library; a library with none is absent.
func (rescans *Rescans) Progress(ctx context.Context) (map[int64]RescanProgress, error) {
	running, err := rescans.matcher.cache.Read().ListRescans(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the rescans: %w", err)
	}
	progress := map[int64]RescanProgress{}
	for _, rescan := range running {
		held := progress[rescan.LibraryID]
		held.IsScanning = held.IsScanning || rescan.ListedAt == nil
		held.ItemCount += rescan.ItemCount
		held.MatchedCount += rescan.ItemCount - rescan.WaitingCount
		progress[rescan.LibraryID] = held
	}
	return progress, nil
}

// RescanGivenUp is a library whose last rescan gave up, its files changing every time they were
// read, and when.
type RescanGivenUp struct {
	LibraryID   int64
	LibraryName string
	GivenUpAt   int64 // Unix milliseconds
}

// GivenUp are the libraries whose rescan gave up and no later rescan of which has finished, which
// Needs attention lists (the owner, 2026-11-13), by library id; a library deleted since is not.
func (rescans *Rescans) GivenUp(ctx context.Context) ([]RescanGivenUp, error) {
	rows, err := rescans.matcher.cache.Read().ListRescansGivenUp(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the rescans given up: %w", err)
	}
	given := make([]RescanGivenUp, 0, len(rows))
	for _, row := range rows {
		named, err := store.OrNotFound(rescans.matcher.database.Read().GetLibrary(ctx, row.LibraryID))
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read library %d: %w", row.LibraryID, err)
		}
		given = append(given, RescanGivenUp{
			LibraryID: row.LibraryID, LibraryName: named.Name, GivenUpAt: row.GivenUpAt,
		})
	}
	return given, nil
}

// pendingRematch is what an item's files say it is, when a rescan waits to match it again.
func (matcher *Matcher) pendingRematch(ctx context.Context, itemID int64) (cachegen.GetRematchRow, bool, error) {
	if isRunning, err := matcher.isRescanRunning(ctx); err != nil || !isRunning {
		return cachegen.GetRematchRow{}, false, err
	}
	row, err := store.OrNotFound(matcher.cache.Read().GetRematch(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return cachegen.GetRematchRow{}, false, nil
	}
	if err != nil {
		return cachegen.GetRematchRow{}, false, fmt.Errorf("read item %d's rematch: %w", itemID, err)
	}
	return row, true, nil
}

// handleRematch is a fetch_metadata job of an item a rescan waits on: it matches the item again
// from what its files say, logs what changed, and counts it in its rescan. An id its files give that
// its provider does not have is the item's own failure (RecordFailure), which ends its rematch too;
// ids kept in cache.db that cannot be read say nothing of the item, and are tried again (D508 §2).
func (matcher *Matcher) handleRematch(ctx context.Context, itemID int64, row cachegen.GetRematchRow) error {
	said := filesQuery{name: row.Name, year: int(row.Year)}
	if err := strictjson.Unmarshal([]byte(row.IdsJSON), &said.ids); err != nil {
		matcher.logger.WarnContext(ctx, "a rematch's ids could not be read; it is tried again",
			"itemId", itemID, "err", err)
		return fmt.Errorf("item %d's rematch: %w", itemID, err)
	}
	result, err := matcher.rematch(ctx, itemID, said)
	if errors.Is(err, ErrNoMatch) {
		return fmt.Errorf("rematch item %d: %w", itemID, jobs.MarkPermanent(err))
	}
	if err != nil {
		return err
	}
	if result.outcome == rematchChanged {
		if err := matcher.settle(ctx, itemID, 0); err != nil {
			return err
		}
	}
	if err := matcher.fetchDiscarded(ctx, itemID, row, result); err != nil {
		return err
	}
	matcher.logRematch(ctx, itemID, result)
	return matcher.endRematch(ctx, itemID, row.RescanID, result.outcome)
}

// logRematch logs a rescan's match that changed what an item is matched to: the item, its old match
// and its new one, and what settled it (D357 §6), so a report of a wrong match after a rescan is
// explained from the log alone.
func (matcher *Matcher) logRematch(ctx context.Context, itemID int64, result rematched) {
	switch result.outcome {
	case rematchChanged:
		matcher.logger.InfoContext(ctx, "a rescan matched an item to another work",
			slogx.EventKey, "rescanRematched",
			"itemId", itemID,
			"before", result.before, "beforeName", result.beforeName, "after", result.after,
			"afterName", result.afterName, "settledBy", result.settledBy, "isGuess", result.isGuess)
	case rematchUnsettled:
		if result.before != "" {
			matcher.logger.InfoContext(ctx, "a rescan left an item unmatched: its search settled on none",
				"itemId", itemID, "before", result.before, "beforeName", result.beforeName,
				"settledBy", result.settledBy)
		}
	case rematchUnchanged, rematchLeftAlone, rematchFailed:
		matcher.logger.DebugContext(ctx, "a rescan matched an item again", "itemId", itemID,
			"outcome", string(result.outcome), "after", result.after, "leftAloneBecause", result.leftAloneBecause)
	}
}

// endRematch counts an item matched again in its rescan, and ends the rescan once it was the last.
func (matcher *Matcher) endRematch(ctx context.Context, itemID, rescanID int64, outcome rematchOutcome) error {
	count := cachegen.CountRematchedInRescanParams{ID: rescanID}
	switch outcome {
	case rematchChanged:
		count.Changed = 1
	case rematchUnchanged:
		count.Unchanged = 1
	case rematchLeftAlone:
		count.LeftAlone = 1
	case rematchUnsettled:
		count.Unsettled = 1
	case rematchFailed:
		count.Failed = 1
	}
	err := matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if err := queries.DeleteRematch(ctx, itemID); err != nil {
			return fmt.Errorf("forget it: %w", err)
		}
		return queries.CountRematchedInRescan(ctx, count)
	})
	if err != nil {
		return fmt.Errorf("count item %d's rematch: %w", itemID, err)
	}
	return matcher.endRescanIfDone(ctx, rescanID)
}

// endRescanIfDone ends a rescan whose films and series were all matched again, and logs its totals
// (D357 §6).
func (matcher *Matcher) endRescanIfDone(ctx context.Context, rescanID int64) error {
	return matcher.endRescan(ctx, rescanID, nil)
}

// endRescan ends a rescan that is done, or, given why it gave up, one that is not, and logs its
// totals: a rescan given up as an error, with why.
func (matcher *Matcher) endRescan(ctx context.Context, rescanID int64, givenUp error) error {
	matcher.rescans.mutex.Lock()
	if err := matcher.countRescansLocked(ctx); err != nil {
		matcher.rescans.mutex.Unlock()
		return err
	}
	var ended cachegen.Rescan
	isEnded := false
	err := matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var err error
		ended, isEnded, err = matcher.deleteEndedRescan(ctx, queries, rescanID, givenUp)
		return err
	})
	if err == nil && isEnded {
		matcher.rescans.running--
	}
	matcher.rescans.mutex.Unlock()
	if err != nil {
		return fmt.Errorf("end rescan %d: %w", rescanID, err)
	}
	if isEnded {
		matcher.logRescanEnded(ctx, ended, givenUp)
	}
	return nil
}

// deleteEndedRescan deletes the rescan if it is ended, and answers it and true, or false while it still waits.
// One given up puts its library on Needs attention until a later rescan of it finishes.
func (matcher *Matcher) deleteEndedRescan(
	ctx context.Context, queries *cachegen.Queries, rescanID int64, givenUp error,
) (cachegen.Rescan, bool, error) {
	rescan, err := store.OrNotFound(queries.GetRescan(ctx, rescanID))
	if errors.Is(err, store.ErrNotFound) {
		return cachegen.Rescan{}, false, nil
	}
	if err != nil {
		return cachegen.Rescan{}, false, fmt.Errorf("read it: %w", err)
	}
	waiting, err := queries.CountRematchesOfRescan(ctx, rescanID)
	if err != nil {
		return cachegen.Rescan{}, false, fmt.Errorf("count what it waits on: %w", err)
	}
	// A rescan gives up before it is listed, so waits on nothing.
	if givenUp == nil && (waiting > 0 || rescan.ListedAt == nil) {
		return cachegen.Rescan{}, false, nil
	}
	if err = queries.DeleteRescan(ctx, rescanID); err != nil {
		return cachegen.Rescan{}, false, fmt.Errorf("delete it: %w", err)
	}
	if givenUp != nil {
		err = queries.UpsertRescanGivenUp(ctx, cachegen.UpsertRescanGivenUpParams{
			LibraryID: rescan.LibraryID, GivenUpAt: matcher.clock.Now().UnixMilli(),
		})
	} else {
		err = queries.DeleteRescanGivenUp(ctx, rescan.LibraryID)
	}
	if err != nil {
		return cachegen.Rescan{}, false, fmt.Errorf("record its library's give-up: %w", err)
	}
	return rescan, true, nil
}

// logRescanEnded logs an ended rescan's totals, and owes a full one's library its pass.
func (matcher *Matcher) logRescanEnded(ctx context.Context, ended cachegen.Rescan, givenUp error) {
	if givenUp != nil {
		matcher.logger.ErrorContext(ctx, "a rescan gave up: its library changed every time it was read",
			append([]any{slogx.EventKey, "rescanGaveUp", "cause", givenUp.Error()}, rescanTotals(ended)...)...)
		return
	}
	matcher.logger.InfoContext(ctx, "a rescan ended",
		append([]any{slogx.EventKey, "rescanEnded"}, rescanTotals(ended)...)...)
	if ended.IsFull != 0 {
		matcher.pass.OwePass(ended.LibraryID) // which queues the images asked for again (D392 §4)
	}
}

// rescanTotals are a rescan's totals as its log line says them.
func rescanTotals(rescan cachegen.Rescan) []any {
	return []any{
		slog.Int64("libraryId", rescan.LibraryID), slog.Int64("itemId", ptr.ValueOrZero(rescan.ItemID)),
		slog.Int64("itemCount", rescan.ItemCount), slog.Int64("changedCount", rescan.ChangedCount),
		slog.Int64("unchangedCount", rescan.UnchangedCount), slog.Int64("leftAloneCount", rescan.LeftAloneCount),
		slog.Int64("unsettledCount", rescan.UnsettledCount), slog.Int64("failedCount", rescan.FailedCount),
		slog.Bool("isFull", rescan.IsFull != 0),
	}
}

// rescanCount is how many rescans cache.db keeps, so a fetch reads its item's rematch only while one
// runs: a rematch is kept only with its rescan (D357). It is counted from cache.db when first needed,
// which finds the rescans a restart left running, and then kept by start and endRescanIfDone, the only
// writers of a rescan, which change it under mutex with their writes, so a count is never short of
// what cache.db holds.
type rescanCount struct {
	mutex sync.Mutex
	// running is the rescans kept, once isCounted; guarded by mutex.
	running   int64
	isCounted bool
}

// countRescansLocked counts the rescans cache.db keeps, unless they were counted. The caller holds
// matcher.rescans.mutex.
func (matcher *Matcher) countRescansLocked(ctx context.Context) error {
	if matcher.rescans.isCounted {
		return nil
	}
	running, err := matcher.cache.Read().ListRescans(ctx)
	if err != nil {
		return fmt.Errorf("count the rescans: %w", err)
	}
	matcher.rescans.running, matcher.rescans.isCounted = int64(len(running)), true
	return nil
}

// isRescanRunning is whether cache.db keeps a rescan, which a rematch waits on.
func (matcher *Matcher) isRescanRunning(ctx context.Context) (bool, error) {
	matcher.rescans.mutex.Lock()
	defer matcher.rescans.mutex.Unlock()
	if err := matcher.countRescansLocked(ctx); err != nil {
		return false, err
	}
	return matcher.rescans.running > 0, nil
}

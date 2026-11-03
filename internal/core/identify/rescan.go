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

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// Refusals a rescan can meet.
var (
	// ErrNotRescannable is a library of neither films nor series: an other one, whose videos are never
	// matched (D357 §1).
	ErrNotRescannable = errors.New("only a library of films or of series is rescanned whole")
	// ErrNotAWork is an episode, an extra or an other video: a rescan matches a film or a series,
	// an episode through its series (D209, D357 §1).
	ErrNotAWork = errors.New("only a film or a series is rescanned; an episode is rescanned through its series")
)

// RescanScans are how a rescan has its scope's files scanned as a manual scan scans them, and sees
// whether that scan is over: the library admin (library.Admin).
type RescanScans interface {
	// Scan asks for a scan of every root of a library.
	Scan(ctx context.Context, libraryID int64) (int, error)
	// ScanReadingAgain asks for a scan of every root of a library that reads every file again,
	// changed on disk or not (D392 §2).
	ScanReadingAgain(ctx context.Context, libraryID int64) (int, error)
	// ScanWork asks for a scan of the folders that hold a film's or series' files in a library, one
	// that reads its files again when isReadingAgain.
	ScanWork(ctx context.Context, libraryID, itemID int64, isReadingAgain bool) (int, error)
	// IsScanning is whether a scan of any root of a library is in progress.
	IsScanning(ctx context.Context, libraryID int64) (bool, error)
}

// RescanKind is how deep a rescan looks.
type RescanKind int

const (
	// Rescan scans its scope as a manual scan does, then matches each film and series in it again
	// (D357).
	Rescan RescanKind = iota
	// FullRescan reads every file in its scope again, changed on disk or not, throws out what the
	// differences from what was kept touch, asks again for the images its films and series are
	// missing, then matches each again as a rescan does (D392).
	FullRescan
)

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
}

// NewRescans returns the rescans, matching through matcher and queuing on queue.
func NewRescans(matcher *Matcher, queue Queue, scans RescanScans) *Rescans {
	return &Rescans{matcher: matcher, queue: queue, scans: scans}
}

// OnRequested has a rescan, once asked for, ask for the rescans' sweep: the job queue's, set by the
// server's wiring.
func (rescans *Rescans) OnRequested(ask func()) { rescans.onRequested = ask }

// RescanEverything rescans every live library of films or of series (D357 §1), and says how many
// rescans it started: a library already being rescanned starts none.
func (rescans *Rescans) RescanEverything(ctx context.Context, kind RescanKind) (int, error) {
	libraryIDs, err := rescans.matcher.database.Read().ListLiveLibraryIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the libraries: %w", err)
	}
	started := 0
	for _, libraryID := range libraryIDs {
		isStarted, err := rescans.RescanLibrary(ctx, libraryID, kind)
		if errors.Is(err, ErrNotRescannable) {
			continue
		}
		if err != nil {
			return started, err
		}
		if isStarted {
			started++
		}
	}
	return started, nil
}

// RescanLibrary rescans a library of films or of series, and says whether it started one: a second
// press while one runs adds nothing (D357 §5, D392 §6).
func (rescans *Rescans) RescanLibrary(ctx context.Context, libraryID int64, kind RescanKind) (bool, error) {
	found, err := store.OrNotFound(rescans.matcher.database.Read().GetLibraryForIdentify(ctx, libraryID))
	if errors.Is(err, store.ErrNotFound) {
		return false, fmt.Errorf("library %d: %w", libraryID, library.ErrNoLibrary)
	}
	if err != nil {
		return false, fmt.Errorf("read library %d: %w", libraryID, err)
	}
	if found.Kind != library.LibraryKindMovies && found.Kind != library.LibraryKindShows {
		return false, fmt.Errorf("library %d, of %s: %w", libraryID, found.Kind, ErrNotRescannable)
	}
	isStarted, err := rescans.start(ctx, libraryID, nil, kind)
	if err != nil || !isStarted {
		return false, err
	}
	scan := rescans.scans.Scan
	if kind == FullRescan {
		scan = rescans.scans.ScanReadingAgain
	}
	if _, err := scan(ctx, libraryID); err != nil {
		return true, fmt.Errorf("scan library %d to rescan it: %w", libraryID, err)
	}
	rescans.askForTheSweep()
	return true, nil
}

// RescanItem rescans one film or series, and says whether it started one: none while its own
// rescan or its library's runs (D357 §5, D392 §6).
func (rescans *Rescans) RescanItem(ctx context.Context, itemID int64, kind RescanKind) (bool, error) {
	item, err := rescans.matcher.readWork(ctx, itemID)
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
	isStarted, err := rescans.start(ctx, libraryID, &itemID, kind)
	if err != nil || !isStarted {
		return false, err
	}
	if _, err := rescans.scans.ScanWork(ctx, libraryID, itemID, kind == FullRescan); err != nil {
		return true, fmt.Errorf("scan item %d's folders to rescan it: %w", itemID, err)
	}
	rescans.askForTheSweep()
	return true, nil
}

// readWork reads the film or series a rescan names: never an episode, an extra or an other video.
func (matcher *Matcher) readWork(ctx context.Context, itemID int64) (target, error) {
	item, err := matcher.readTarget(ctx, itemID)
	if err != nil {
		return target{}, err
	}
	if !item.isSeries && item.title.Kind != KindMovie {
		return target{}, fmt.Errorf("item %d, a %s: %w", itemID, item.title.Kind, ErrNotAWork)
	}
	if item.libraryID() == nil {
		return target{}, fmt.Errorf("item %d: %w", itemID, ErrNoItem)
	}
	return item, nil
}

// start records a rescan of a scope, a library or one film or series in it, unless one of that
// scope runs, and says whether it did.
func (rescans *Rescans) start(ctx context.Context, libraryID int64, itemID *int64, kind RescanKind) (bool, error) {
	isStarted := false
	err := rescans.matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		_, insertErr := store.OrNotFound(queries.InsertRescan(ctx, cachegen.InsertRescanParams{
			LibraryID: libraryID, ItemID: itemID, RequestedAt: rescans.matcher.clock.Now().UnixMilli(),
			IsFull: boolInt(kind == FullRescan),
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
		rescans.matcher.logger.InfoContext(ctx, "a rescan was asked for", "libraryId", libraryID,
			"itemId", ptr.ValueOrZero(itemID), "isFull", kind == FullRescan)
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
		said, known := saidByLibrary[rescan.LibraryID]
		if !known {
			said, err = rescans.matcher.pass.filesSayOf(ctx, rescan.LibraryID)
			if errors.Is(err, errLibraryKeptChanging) {
				rescans.matcher.logger.InfoContext(ctx, "a rescan waits for its library to stop changing",
					"libraryId", rescan.LibraryID, "cause", err)
				continue
			}
			if err != nil {
				return 0, err
			}
			saidByLibrary[rescan.LibraryID] = said
		}
		if err := rescans.list(ctx, rescan, said); err != nil {
			return 0, err
		}
	}
	return rescans.queueRematches(ctx)
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
		case kind == itemKindCollection:
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

// pendingRematch is what an item's files say it is, when a rescan waits to match it again.
func (matcher *Matcher) pendingRematch(ctx context.Context, itemID int64) (cachegen.GetRematchRow, bool, error) {
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
// its provider does not have is the item's own failure (RecordFailure), which ends its rematch too.
func (matcher *Matcher) handleRematch(ctx context.Context, itemID int64, row cachegen.GetRematchRow) error {
	said := filesQuery{name: row.Name, year: int(row.Year)}
	if err := json.Unmarshal([]byte(row.IdsJSON), &said.ids); err != nil {
		return fmt.Errorf("item %d's rematch: %w", itemID, jobs.MarkPermanent(err))
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
	matcher.logRematch(ctx, itemID, result)
	return matcher.endRematch(ctx, itemID, row.RescanID, result.outcome)
}

// logRematch logs a rescan's match that changed what an item is matched to: the item, its old match
// and its new one, and what settled it (D357 §6), so a report of a wrong match after a rescan is
// explained from the log alone.
func (matcher *Matcher) logRematch(ctx context.Context, itemID int64, result rematched) {
	switch result.outcome {
	case rematchChanged:
		matcher.logger.InfoContext(ctx, "a rescan matched an item to another work", "itemId", itemID,
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
	var ended cachegen.Rescan
	isEnded := false
	err := matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		rescan, err := store.OrNotFound(queries.GetRescan(ctx, rescanID))
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read it: %w", err)
		}
		waiting, err := queries.CountRematchesOfRescan(ctx, rescanID)
		if err != nil {
			return fmt.Errorf("count what it waits on: %w", err)
		}
		if waiting > 0 || rescan.ListedAt == nil {
			return nil
		}
		ended, isEnded = rescan, true
		return queries.DeleteRescan(ctx, rescanID)
	})
	if err != nil {
		return fmt.Errorf("end rescan %d: %w", rescanID, err)
	}
	if isEnded {
		matcher.logger.InfoContext(ctx, "a rescan ended", rescanTotals(ended)...)
		if ended.IsFull != 0 {
			matcher.pass.OwePass(ended.LibraryID) // which queues the images asked for again (D392 §4)
		}
	}
	return nil
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

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

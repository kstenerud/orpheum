// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// refreshArgs are a fetch_metadata job's arguments when it refreshes a matched item rather than matching
// it (D213).
type refreshArgs struct {
	IsRefresh bool `json:"isRefresh"`
	// ShouldRetryImages asks again for the item's images that were missing or refused.
	ShouldRetryImages bool `json:"shouldRetryImages,omitempty"`
}

// RefreshJob is the fetch_metadata job that refreshes an item from the source it follows, from runnableAt
// (now when zero); with shouldRetryImages it also asks again for the images it is missing (D213 §1, §2).
func RefreshJob(
	itemID int64, reason string, runnableAt time.Time, shouldRetryImages bool, priority int64,
) (jobs.Request, error) {
	// Encoded from refreshArgs itself, so the job reads the names it was written with.
	args, err := json.Marshal(refreshArgs{IsRefresh: true, ShouldRetryImages: shouldRetryImages})
	if err != nil {
		return jobs.Request{}, fmt.Errorf("encode item %d's refresh: %w", itemID, err)
	}
	return jobs.Request{
		Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: itemID}, ArgsJSON: string(args), Reason: reason,
		RunnableAt: runnableAt, Priority: priority, Destination: FetchDestination,
	}, nil
}

// FetchDestination is what a fetch_metadata job needs that can be down: TMDB's API, the one
// provider a fetch calls in Cut A. Named when the job is queued, so no job is claimed while it
// is parked, and a person's refresh looks at it at once (D231 §6).
const FetchDestination = jobs.DestinationTMDBAPI

// FetchArgsMerge is how a fetch_metadata request folds into the job open for its item (D231 §8).
// A refresh behind a queued refresh asks for the images again if either does. A match and a
// refresh do different things (a refresh fetches again only what is stored, and a match fetches
// a season listing a new episode needs), so one never folds into the other: the queued job keeps
// its own arguments. A person's refresh is a fact the job reads when it runs (refresh_request,
// D231 §4), so whichever job runs does what they asked. A running fetch covers a request of its
// own mode that asks for no images it does not; any other request is queued to follow it.
func FetchArgsMerge() jobs.ArgsMerge {
	return jobs.ArgsMerge{
		Merge: func(queued, requested string) (string, error) {
			queuedArgs, err := decodeFetchArgs(queued)
			if err != nil {
				return "", err
			}
			requestedArgs, err := decodeFetchArgs(requested)
			if err != nil || !queuedArgs.IsRefresh || !requestedArgs.IsRefresh {
				return queued, err
			}
			queuedArgs.ShouldRetryImages = queuedArgs.ShouldRetryImages || requestedArgs.ShouldRetryImages
			merged, err := json.Marshal(queuedArgs)
			if err != nil {
				return "", fmt.Errorf("encode merged fetch arguments: %w", err)
			}
			return string(merged), nil
		},
		Covers: func(running, requested string) (bool, error) {
			runningArgs, err := decodeFetchArgs(running)
			if err != nil {
				return false, err
			}
			requestedArgs, err := decodeFetchArgs(requested)
			if err != nil {
				return false, err
			}
			return runningArgs.IsRefresh == requestedArgs.IsRefresh &&
				(runningArgs.ShouldRetryImages || !requestedArgs.ShouldRetryImages), nil
		},
	}
}

// decodeFetchArgs reads a fetch_metadata job's arguments.
func decodeFetchArgs(argsJSON string) (refreshArgs, error) {
	var args refreshArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return refreshArgs{}, fmt.Errorf("read fetch arguments %q: %w", argsJSON, err)
	}
	return args, nil
}

// OutcomeKept is a refresh that found the item gone from its provider: our copy is kept (D213 §4).
const OutcomeKept Outcome = "kept: gone from its provider"

// rollingPriority puts the rolling refresh behind the queue's other work; a person's refresh is
// jobs.PriorityPersonsRequest, ahead of it.
const rollingPriority = -10

// refreshPeriods are the settings that say, per provider, how old its data may get (D213 §3).
func refreshPeriods() map[string]settings.Key {
	return map[string]settings.Key{"tmdb": settings.TMDBRefreshAfterDays}
}

// refresh fetches an item's record again from the source it follows and re-derives it: a film,
// or a series with every season listing it keeps. An episode is refreshed through its series.
// One the provider no longer has keeps our copy. An item not matched yet is matched.
func (matcher *Matcher) refresh(ctx context.Context, itemID int64, shouldRetryImages bool) (Outcome, error) {
	queries := matcher.database.Read()
	kind, err := store.OrNotFound(queries.GetItemKind(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return OutcomeNothingToDo, nil
	}
	if err != nil {
		return "", fmt.Errorf("read item %d: %w", itemID, err)
	}
	if kind == KindSeries {
		return matcher.refreshSeries(ctx, itemID, shouldRetryImages)
	}
	title, err := store.OrNotFound(queries.GetWorkForMatch(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return OutcomeNothingToDo, nil
	}
	if err != nil {
		return "", fmt.Errorf("read title %d: %w", itemID, err)
	}
	if title.Kind == library.TitleKindEpisode {
		place, placeErr := store.OrNotFound(queries.GetEpisodePlace(ctx, itemID))
		if placeErr != nil {
			return OutcomeNothingToDo, nil //nolint:nilerr // an episode with no series has nothing to refresh
		}
		return matcher.refreshSeries(ctx, place.SequenceID, shouldRetryImages)
	}
	provider, known := matcher.providers[ptr.ValueOrZero(title.FollowedSource)]
	if !known || title.FollowedSourceID == nil {
		return matcher.Match(ctx, itemID)
	}
	if shouldRetryImages {
		if retryErr := matcher.retryImages(ctx, itemID); retryErr != nil {
			return "", retryErr
		}
	}
	err = matcher.accept(ctx, provider, title, *title.FollowedSourceID, ptr.ValueOrZero(title.IdentificationState))
	if errors.Is(err, ErrNoMatch) {
		gone := goneResponsePart{itemID, provider.Name(), ""}
		return OutcomeKept, matcher.keepGoneCopy(ctx, title.FilingLibraryID, gone)
	}
	if err != nil {
		return "", err
	}
	return OutcomeMatched, nil
}

// refreshSeries fetches a matched series' record and each season listing and episode group it keeps,
// its list of groups taken again from its new record where that carries it, and derives its
// orderings from them.
func (matcher *Matcher) refreshSeries(ctx context.Context, seriesID int64, shouldRetryImages bool) (Outcome, error) {
	queries := matcher.database.Read()
	series, err := store.OrNotFound(queries.GetSeriesForMatch(ctx, seriesID))
	if errors.Is(err, store.ErrNotFound) {
		return OutcomeNothingToDo, nil
	}
	if err != nil {
		return "", fmt.Errorf("read series %d: %w", seriesID, err)
	}
	provider, known := matcher.providers[providerOf(series.MetadataSource)]
	if !known || !isSeriesMatched(series) {
		return matcher.matchSeries(ctx, seriesID)
	}
	if shouldRetryImages {
		if retryErr := matcher.retrySeriesImages(ctx, seriesID); retryErr != nil {
			return "", retryErr
		}
	}
	parts, err := queries.ListResponseParts(ctx, sqlcgen.ListResponsePartsParams{
		ItemID: seriesID, Provider: provider.Name(),
	})
	if err != nil {
		return "", fmt.Errorf("list series %d's stored parts: %w", seriesID, err)
	}
	// The list of episode groups is taken again from the record, after it.
	keepsList := slices.Contains(parts, groupListPart)
	parts = slices.DeleteFunc(parts, func(part string) bool { return part == groupListPart })
	outcome := OutcomeMatched
	for _, part := range parts {
		body, fetchErr := matcher.fetchPart(ctx, provider, ptr.ValueOrZero(series.FollowedSourceID), part)
		if errors.Is(fetchErr, ErrNoMatch) {
			outcome = OutcomeKept
			if err := matcher.noteRefreshAttempt(ctx, goneResponsePart{seriesID, provider.Name(), part}); err != nil {
				return "", err
			}
			continue
		}
		if fetchErr != nil {
			return "", fmt.Errorf("refresh series %d's %q: %w", seriesID, part, fetchErr)
		}
		if err := matcher.storePart(ctx, provider, series, part, body); err != nil {
			return "", err
		}
	}
	if err := matcher.refreshOrderings(ctx, provider, series, keepsList); err != nil {
		return "", err
	}
	return outcome, matcher.recordSeriesAndRederive(ctx, series, nil)
}

// refreshOrderings takes a refreshed series' list of episode groups again, when it keeps one, from its
// new record where that carries it, and derives its orderings from what is now stored.
func (matcher *Matcher) refreshOrderings(
	ctx context.Context, provider Provider, series sqlcgen.GetSeriesForMatchRow, keepsList bool,
) error {
	if keepsList {
		list, err := matcher.groupList(ctx, provider, series.ID, ptr.ValueOrZero(series.FollowedSourceID))
		if err != nil {
			return err
		}
		if err := matcher.storePartBody(ctx, provider, series.ID, groupListPart, list); err != nil {
			return err
		}
	}
	return keepOrderingsDerived(ctx, matcher.database, provider, series.ID)
}

// fetchPart fetches one stored part of a series again: its own record, an episode group, or a
// season's listing.
func (matcher *Matcher) fetchPart(ctx context.Context, provider Provider, sourceID, part string) ([]byte, error) {
	if part == "" {
		return provider.Fetch(ctx, KindSeries, sourceID) //nolint:wrapcheck // wrapped by the caller
	}
	if groupID, isGroup := groupOfPart(part); isGroup {
		return provider.FetchEpisodeGroup(ctx, groupID) //nolint:wrapcheck // wrapped by the caller
	}
	season, err := seasonOfPart(part)
	if err != nil {
		return nil, err
	}
	return provider.FetchSeason(ctx, sourceID, season) //nolint:wrapcheck // wrapped by the caller
}

// storePart stores a part fetched again: a series' record with its match as it was, an episode
// group, or a season.
func (matcher *Matcher) storePart(
	ctx context.Context, provider Provider, series sqlcgen.GetSeriesForMatchRow, part string, body []byte,
) error {
	if _, isGroup := groupOfPart(part); isGroup {
		return matcher.storePartBody(ctx, provider, series.ID, part, body)
	}
	if part != "" {
		season, err := seasonOfPart(part)
		if err != nil {
			return err
		}
		return matcher.storeSeason(ctx, provider, series.ID, season, body)
	}
	metadata, err := provider.Parse(KindSeries, body)
	if err != nil {
		return fmt.Errorf("read series %d's record: %w", series.ID, err)
	}
	bodyGzip, err := bounded.Gzip(body)
	if err != nil {
		return fmt.Errorf("keep the provider's answer: %w", err)
	}
	found := seriesMatchFound{
		provider: provider.Name(), seriesID: series.ID, metadata: metadata,
		identification: ptr.ValueOrZero(series.IdentificationState),
	}
	now := matcher.clock.Now().UnixMilli()
	return matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error { //nolint:wrapcheck // recordSeries wraps
		return recordSeries(ctx, queries, found, bodyGzip, now)
	})
}

// goneResponsePart names one stored part a refresh found gone from its provider.
type goneResponsePart struct {
	itemID   int64
	provider string
	part     string
}

// keepGoneCopy keeps a title's copy of a record its provider no longer has, notes the attempt, and
// marks its library as owing a pass, so images asked for again are owed.
func (matcher *Matcher) keepGoneCopy(ctx context.Context, libraryID *int64, gone goneResponsePart) error {
	matcher.logger.InfoContext(ctx, "a refresh found an item gone from its provider; our copy is kept",
		"itemId", gone.itemID, "provider", gone.provider)
	if err := matcher.noteRefreshAttempt(ctx, gone); err != nil {
		return err
	}
	if libraryID == nil {
		return nil
	}
	matcher.pass.OwePass(*libraryID)
	return nil
}

// noteRefreshAttempt stamps a stored part's LastAskedAt with now: the provider was asked, and the
// rolling refresh waits a full period before asking again. Its FetchedAt stays the stored body's
// age, which a season listing's staleness is judged by.
func (matcher *Matcher) noteRefreshAttempt(ctx context.Context, gone goneResponsePart) error {
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.MarkProviderResponseAsked(ctx, sqlcgen.MarkProviderResponseAskedParams{
			LastAskedAt: matcher.clock.Now().UnixMilli(), ItemID: gone.itemID, Provider: gone.provider, Part: gone.part,
		})
	})
	if err != nil {
		return fmt.Errorf("note item %d's refresh: %w", gone.itemID, err)
	}
	return nil
}

// ImageRetrier makes items' missing and refused images owed again, for their re-derivation to
// find (D213 §1, §2): the artwork service, which writes the artwork table and which the server's
// wiring hands in through SetImageRetrier, so this package never imports it. A refresh asks it
// when its job's arguments or the person's stored request (refresh_request) say to.
type ImageRetrier interface {
	MarkImagesOwedAgain(ctx context.Context, itemIDs ...int64) error
}

// SetImageRetrier sets what a refresh that asks for images again calls. Until it is set, a
// refresh asks for no image again. Set it before any job runs.
func (matcher *Matcher) SetImageRetrier(retrier ImageRetrier) {
	matcher.imageRetrier = retrier
}

// retryImages makes an item's missing and refused images owed again, for its re-derivation to
// report (D213 §1, §2).
func (matcher *Matcher) retryImages(ctx context.Context, itemIDs ...int64) error {
	if matcher.imageRetrier == nil {
		return nil
	}
	//nolint:wrapcheck // the artwork service's, wrapped there
	return matcher.imageRetrier.MarkImagesOwedAgain(ctx, itemIDs...)
}

func (matcher *Matcher) retrySeriesImages(ctx context.Context, seriesID int64) error {
	episodes, err := matcher.database.Read().ListLiveSeriesEpisodeSources(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("list series %d's episodes: %w", seriesID, err)
	}
	ids := []int64{seriesID}
	for _, episode := range episodes {
		ids = append(ids, episode.WorkID)
	}
	return matcher.retryImages(ctx, ids...)
}

// Queue is where refreshes are queued: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
}

// Refresher queues fetches: a person's refresh, of one title or series whenever they like, the
// rolling refresh of every item whose data is older than its provider's setting (D213 §2, §3),
// and, as the fetch sweeps, what persistent.db says is owed (D231 §3).
type Refresher struct {
	database *store.Database
	queue    Queue
	settings RefreshSettings
	clock    clock.Clock
	logger   *slog.Logger
}

// RefreshSettings are what the refresher reads: each provider's period.
type RefreshSettings interface {
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// NewRefresher returns the refresher.
func NewRefresher(
	database *store.Database, queue Queue, refreshSettings RefreshSettings, clockSource clock.Clock,
	logger *slog.Logger,
) *Refresher {
	return &Refresher{database: database, queue: queue, settings: refreshSettings, clock: clockSource, logger: logger}
}

// Request records a person's refresh of a title or series, with its missing images asked for
// again, clearing a failure of its own ("try again"), and queues its fetch at their priority; it
// reports whether a job was queued rather than one already waiting. The request is a fact the
// refresh sweep queues again until a fetch answers it (D231 §4), so it outlives cache.db.
func (refresher *Refresher) Request(ctx context.Context, itemID int64) (bool, error) {
	if _, err := store.OrNotFound(refresher.database.Read().GetItemKind(ctx, itemID)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, fmt.Errorf("item %d: %w", itemID, ErrNoItem)
		}
		return false, fmt.Errorf("read item %d: %w", itemID, err)
	}
	err := refresher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if clearErr := queries.ClearFetchFailure(ctx, itemID); clearErr != nil {
			return fmt.Errorf("clear its failure: %w", clearErr)
		}
		requestErr := queries.RequestRefresh(ctx, sqlcgen.RequestRefreshParams{
			ItemID: itemID, RequestedAt: refresher.clock.Now().UnixMilli(), ShouldFetchImages: 1,
		})
		if requestErr != nil {
			return fmt.Errorf("keep the request: %w", requestErr)
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("record item %d's refresh: %w", itemID, err)
	}
	job, err := RefreshJob(itemID, personsRefreshReason, time.Time{}, true, jobs.PriorityPersonsRequest)
	if err != nil {
		return false, err
	}
	queued, err := refresher.queue.Enqueue(ctx, job)
	if err != nil {
		return false, fmt.Errorf("queue item %d's refresh: %w", itemID, err)
	}
	return queued, nil
}

// personsRefreshReason is why a person's refresh is queued, as the job view says it.
const personsRefreshReason = "a person asked for it to be refreshed"

// QueueRequested queues the fetch of every live item a person asked to refresh that no fetch has
// answered yet, at their priority (D231 §4): the refresh sweep. It says how many it queued.
func (refresher *Refresher) QueueRequested(ctx context.Context) (int, error) {
	requests, err := refresher.database.Read().ListRefreshRequests(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the refreshes people asked for: %w", err)
	}
	queued := 0
	for _, request := range requests {
		job, err := RefreshJob(request.ItemID, personsRefreshReason, time.Time{}, request.ShouldFetchImages != 0,
			jobs.PriorityPersonsRequest)
		if err != nil {
			return queued, err
		}
		added, err := refresher.enqueueDerived(ctx, job)
		if err != nil {
			return queued, err
		}
		queued += added
	}
	return queued, nil
}

// FirstMatchJob is the fetch_metadata job that matches a film or series no source has answered
// for yet (D170).
func FirstMatchJob(itemID int64) jobs.Request {
	return jobs.Request{
		Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: itemID}, Destination: FetchDestination,
		Reason: "the first match for a newly identified title or series",
	}
}

// QueueFirstMatches queues the first match of every film and series in a library matched at a
// provider that has no answer yet: not matched, not "not found", and no failure of its own. The
// match is owed until then, however often an earlier job for it was lost (D231 §3, which
// replaces D170's "once per item"). It says how many it queued.
func (refresher *Refresher) QueueFirstMatches(ctx context.Context) (int, error) {
	queries := refresher.database.Read()
	libraryIDs, err := queries.ListProviderMatchedLibraryIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the libraries matched at a provider: %w", err)
	}
	queued := 0
	for _, libraryID := range libraryIDs {
		films, err := queries.ListWorksAwaitingMatch(ctx, &libraryID)
		if err != nil {
			return queued, fmt.Errorf("list library %d's films awaiting a match: %w", libraryID, err)
		}
		series, err := queries.ListSeriesAwaitingMatch(ctx, &libraryID)
		if err != nil {
			return queued, fmt.Errorf("list library %d's series awaiting a match: %w", libraryID, err)
		}
		for _, itemID := range slices.Concat(films, series) {
			added, err := refresher.enqueueDerived(ctx, FirstMatchJob(itemID))
			if err != nil {
				return queued, err
			}
			queued += added
		}
	}
	return queued, nil
}

// RecheckReason is why a recheck of a missing image is queued, as the job view says it.
const RecheckReason = "an image it names is missing; its record is looked at again once"

// QueueRechecks queues the refresh of each item with an image its source said was missing and
// that is due its one recheck (D213 §1), with the images it is missing asked for again: derived
// from the artwork's refusal, which the artwork service reads (DueRechecks), never held as a job
// dated ahead (D231 §3). It says how many it queued.
func (refresher *Refresher) QueueRechecks(ctx context.Context, itemIDs []int64) (int, error) {
	queued := 0
	for _, itemID := range itemIDs {
		job, err := RefreshJob(itemID, RecheckReason, time.Time{}, true, 0)
		if err != nil {
			return queued, err
		}
		added, err := refresher.enqueueDerived(ctx, job)
		if err != nil {
			return queued, err
		}
		queued += added
	}
	return queued, nil
}

// enqueueDerived queues a job a sweep derived, and says whether it added one (1) or merged (0).
func (refresher *Refresher) enqueueDerived(ctx context.Context, job jobs.Request) (int, error) {
	job.IsDerived = true
	added, err := refresher.queue.Enqueue(ctx, job)
	if err != nil {
		return 0, fmt.Errorf("queue item %d's fetch: %w", job.ItemID, err)
	}
	if added {
		return 1, nil
	}
	return 0, nil
}

// Sweep queues a refresh of every item whose data from a provider is due: older than the
// provider's period, each item falling due somewhere in its last quarter, spread by its id, so a
// library fetched at once is not refreshed at once. It returns how many it queued.
func (refresher *Refresher) Sweep(ctx context.Context) (int, error) {
	queued := 0
	now := refresher.clock.Now()
	for provider, key := range refreshPeriods() {
		period, err := refresher.settings.Duration(ctx, key)
		if err != nil {
			return queued, fmt.Errorf("read %s: %w", key, err)
		}
		if period == 0 {
			continue // never, as a person chose
		}
		due, err := refresher.database.Read().ListResponsesAskedBefore(ctx, sqlcgen.ListResponsesAskedBeforeParams{
			Provider: provider, Before: now.Add(-period * 3 / 4).UnixMilli(),
		})
		if err != nil {
			return queued, fmt.Errorf("list %s data due: %w", provider, err)
		}
		for _, item := range due {
			if now.Before(time.UnixMilli(item.OldestAskedAt).Add(dueAfter(period, item.ItemID))) {
				continue
			}
			added, err := refresher.queueDue(ctx, item.ItemID, provider)
			if err != nil {
				return queued, err
			}
			if added {
				queued++
			}
		}
	}
	return queued, nil
}

// queueDue queues the rolling refresh of one item whose data from provider is due, and says
// whether it was queued rather than already waiting.
func (refresher *Refresher) queueDue(ctx context.Context, itemID int64, provider string) (bool, error) {
	job, err := RefreshJob(itemID, "its "+provider+" data is due to be fetched again", time.Time{}, false,
		rollingPriority)
	if err != nil {
		return false, err
	}
	added, err := refresher.enqueueDerived(ctx, job)
	return added > 0, err
}

// dueAfter is how old an item's data gets before it is refreshed: between three quarters of the
// period and all of it, by its id, the same every time.
func dueAfter(period time.Duration, itemID int64) time.Duration {
	const spread = 1024
	slot := (uint64(itemID) * 2654435761) % spread //nolint:gosec // an id, hashed for spreading
	return period*3/4 + period/4*time.Duration(slot)/spread
}

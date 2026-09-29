// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
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

// refreshArgs are a fetch job's arguments when it refreshes a matched item rather than matching
// it (D213).
type refreshArgs struct {
	IsRefresh bool `json:"isRefresh"`
	// ShouldRetryImages asks again for the item's images that were missing or refused.
	ShouldRetryImages bool `json:"shouldRetryImages,omitempty"`
}

// RefreshJob is the fetch job that refreshes an item from the source it follows, from runnableAt
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
		Kind: jobs.KindFetch, Target: jobs.Target{ItemID: itemID}, ArgsJSON: string(args), Reason: reason,
		RunnableAt: runnableAt, Priority: priority,
	}, nil
}

// OutcomeKept is a refresh that found the item gone from its provider: our copy is kept (D213 §4).
const OutcomeKept Outcome = "kept: gone from its provider"

// personRefreshPriority puts a person's refresh ahead of the queue's own work; the rolling refresh
// comes after it.
const (
	personRefreshPriority = 20
	rollingPriority       = -10
)

// refreshPeriods are the settings that say, per provider, how old its data may get (D213 §3).
func refreshPeriods() map[string]settings.Key {
	return map[string]settings.Key{"tmdb": settings.TMDBRefreshAfterD}
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
	if kind == itemKindCollection {
		return matcher.refreshSeries(ctx, itemID, shouldRetryImages)
	}
	title, err := store.OrNotFound(queries.GetTitleForMatch(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return OutcomeNothingToDo, nil
	}
	if err != nil {
		return "", fmt.Errorf("read title %d: %w", itemID, err)
	}
	if title.Kind == library.TitleKindEpisode {
		seriesID, seriesErr := store.OrNotFound(queries.GetSeriesOfTitle(ctx, itemID))
		if seriesErr != nil {
			return OutcomeNothingToDo, nil //nolint:nilerr // an episode with no series has nothing to refresh
		}
		return matcher.refreshSeries(ctx, seriesID, shouldRetryImages)
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
	err = matcher.accept(ctx, provider, title, *title.FollowedSourceID, title.IdentificationState)
	if errors.Is(err, ErrNoMatch) {
		return OutcomeKept, matcher.keepGoneCopy(ctx, title.LibraryID, goneResponsePart{itemID, provider.Name(), ""})
	}
	if err != nil {
		return "", err
	}
	return OutcomeMatched, nil
}

// refreshSeries fetches a matched series' record and each season listing it keeps.
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
	return outcome, matcher.recordSeriesAndRederive(ctx, series, nil)
}

// fetchPart fetches one stored part of a series again: its own record, or a season's listing.
func (matcher *Matcher) fetchPart(ctx context.Context, provider Provider, sourceID, part string) ([]byte, error) {
	if part == "" {
		return provider.Fetch(ctx, KindSeries, sourceID) //nolint:wrapcheck // wrapped by the caller
	}
	season, err := strconv.Atoi(strings.TrimPrefix(part, "season/"))
	if err != nil {
		return nil, fmt.Errorf("stored part %q: %w", part, err)
	}
	return provider.FetchSeason(ctx, sourceID, season) //nolint:wrapcheck // wrapped by the caller
}

// storePart stores a part fetched again: a series' record with its match as it was, or a season.
func (matcher *Matcher) storePart(
	ctx context.Context, provider Provider, series sqlcgen.GetSeriesForMatchRow, part string, body []byte,
) error {
	if part != "" {
		season, err := strconv.Atoi(strings.TrimPrefix(part, "season/"))
		if err != nil {
			return fmt.Errorf("stored part %q: %w", part, err)
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
// re-derives its library, so images asked for again are owed.
func (matcher *Matcher) keepGoneCopy(ctx context.Context, libraryID *int64, gone goneResponsePart) error {
	matcher.logger.InfoContext(ctx, "a refresh found an item gone from its provider; our copy is kept",
		"itemId", gone.itemID, "provider", gone.provider)
	if err := matcher.noteRefreshAttempt(ctx, gone); err != nil {
		return err
	}
	if libraryID == nil {
		return nil
	}
	_, err := matcher.pass.IdentifyLibrary(ctx, *libraryID)
	return err
}

// noteRefreshAttempt stamps a stored part's FetchedAt with now: the provider was asked, and the
// rolling refresh waits a full period before asking again.
func (matcher *Matcher) noteRefreshAttempt(ctx context.Context, gone goneResponsePart) error {
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.MarkProviderResponseAsked(ctx, sqlcgen.MarkProviderResponseAskedParams{
			FetchedAt: matcher.clock.Now().UnixMilli(), ItemID: gone.itemID, Provider: gone.provider, Part: gone.part,
		})
	})
	if err != nil {
		return fmt.Errorf("note item %d's refresh: %w", gone.itemID, err)
	}
	return nil
}

// retryImages makes an item's missing and refused images owed again, for its re-derivation to
// report (D213 §1, §2).
func (matcher *Matcher) retryImages(ctx context.Context, itemIDs ...int64) error {
	now := matcher.clock.Now().UnixMilli()
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, itemID := range itemIDs {
			err := queries.RetryItemArtwork(ctx, sqlcgen.RetryItemArtworkParams{RemovedAt: &now, ItemID: itemID})
			if err != nil {
				return err //nolint:wrapcheck // wrapped below
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ask again for the images of %v: %w", itemIDs, err)
	}
	return nil
}

func (matcher *Matcher) retrySeriesImages(ctx context.Context, seriesID int64) error {
	episodes, err := matcher.database.Read().ListLiveSeriesEpisodeSources(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("list series %d's episodes: %w", seriesID, err)
	}
	ids := []int64{seriesID}
	for _, episode := range episodes {
		ids = append(ids, episode.TitleID)
	}
	return matcher.retryImages(ctx, ids...)
}

// Queue is where refreshes are queued: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
}

// Refresher queues refreshes: a person's, of one title or series whenever they like, and the
// rolling refresh of every item whose data is older than its provider's setting (D213 §2, §3).
type Refresher struct {
	database *store.Database
	queue    Queue
	settings RefreshSettings
	clock    clock.Clock
	logger   *slog.Logger
}

// RefreshSettings are what the refresher reads: each provider's period.
type RefreshSettings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// NewRefresher returns the refresher.
func NewRefresher(
	database *store.Database, queue Queue, refreshSettings RefreshSettings, clockSource clock.Clock,
	logger *slog.Logger,
) *Refresher {
	return &Refresher{database: database, queue: queue, settings: refreshSettings, clock: clockSource, logger: logger}
}

// Request queues a person's refresh of a title or series, with its missing images asked for
// again, and reports whether one was queued rather than already waiting.
func (refresher *Refresher) Request(ctx context.Context, itemID int64) (bool, error) {
	if _, err := store.OrNotFound(refresher.database.Read().GetItemKind(ctx, itemID)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, fmt.Errorf("item %d: %w", itemID, ErrNoItem)
		}
		return false, fmt.Errorf("read item %d: %w", itemID, err)
	}
	job, err := RefreshJob(itemID, "a person asked for it to be refreshed", time.Time{}, true, personRefreshPriority)
	if err != nil {
		return false, err
	}
	queued, err := refresher.queue.Enqueue(ctx, job)
	if err != nil {
		return false, fmt.Errorf("queue item %d's refresh: %w", itemID, err)
	}
	return queued, nil
}

// Sweep queues a refresh of every item whose data from a provider is due: older than the
// provider's period, each item falling due somewhere in its last quarter, spread by its id, so a
// library fetched at once is not refreshed at once. It returns how many it queued.
func (refresher *Refresher) Sweep(ctx context.Context) (int, error) {
	queued := 0
	now := refresher.clock.Now()
	for provider, key := range refreshPeriods() {
		days, err := refresher.settings.Int(ctx, key)
		if err != nil {
			return queued, fmt.Errorf("read %s: %w", key, err)
		}
		if days == 0 {
			continue // never, as a person chose
		}
		period := time.Duration(days) * 24 * time.Hour
		due, err := refresher.database.Read().ListResponsesFetchedBefore(ctx, sqlcgen.ListResponsesFetchedBeforeParams{
			Provider: provider, Before: now.Add(-period * 3 / 4).UnixMilli(),
		})
		if err != nil {
			return queued, fmt.Errorf("list %s data due: %w", provider, err)
		}
		for _, item := range due {
			if now.Before(time.UnixMilli(item.Oldest).Add(dueAfter(period, item.ItemID))) {
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
	added, err := refresher.queue.Enqueue(ctx, job)
	if err != nil {
		return false, fmt.Errorf("queue item %d's refresh: %w", itemID, err)
	}
	return added, nil
}

// dueAfter is how old an item's data gets before it is refreshed: between three quarters of the
// period and all of it, by its id, the same every time.
func dueAfter(period time.Duration, itemID int64) time.Duration {
	const spread = 1024
	slot := (uint64(itemID) * 2654435761) % spread //nolint:gosec // an id, hashed for spreading
	return period*3/4 + period/4*time.Duration(slot)/spread
}

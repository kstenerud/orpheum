// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// claimSeriesByProviderID finds a series by a provider id its files give, as claimTitleByProviderID
// finds a film: in this library, or in another where it has no live files left, a deleted
// library's among them, when it follows its files here with its history (D208 section 2, D231
// section 7). Its episodes come back one by one, as their files are filed.
func (filer *libraryFiler) claimSeriesByProviderID(ctx context.Context, ids map[string]string) (int64, error) {
	for _, provider := range []string{sourceTMDB, sourceIMDb, sourceTVDB} {
		providerID := ids[provider]
		if providerID == "" {
			continue
		}
		rows, err := filer.queries.ListSeriesByProviderID(ctx, sqlcgen.ListSeriesByProviderIDParams{
			Provider: provider, ProviderID: providerID,
		})
		if err != nil {
			return 0, fmt.Errorf("look up the series %s %s: %w", provider, providerID, err)
		}
		for _, row := range rows {
			if row.LibraryID != nil && *row.LibraryID == filer.library.ID && row.RemovedAt == nil {
				return row.ID, nil
			}
		}
		for _, row := range rows {
			liveFileCount, countErr := filer.queries.CountLiveFilesOfSeries(ctx, row.ID)
			if countErr != nil {
				return 0, fmt.Errorf("count the files of series %d: %w", row.ID, countErr)
			}
			if liveFileCount > 0 {
				continue // its files are still there: that library keeps its series
			}
			return row.ID, filer.bringSeriesBack(ctx, row.ID)
		}
	}
	return 0, nil
}

// bringSeriesBack moves a series into this library, live, and lets its episodes be found at their
// places, each hidden one brought back when a file of it is filed.
func (filer *libraryFiler) bringSeriesBack(ctx context.Context, seriesID int64) error {
	libraryID := filer.library.ID
	err := filer.queries.MoveItemToLibrary(ctx, sqlcgen.MoveItemToLibraryParams{LibraryID: &libraryID, ID: seriesID})
	if err != nil {
		return fmt.Errorf("bring series %d back: %w", seriesID, err)
	}
	return filer.knowEpisodesOf(ctx, seriesID)
}

// bringEpisodeBack brings an episode hidden with a deleted library back into this one, live, as a
// file of it is filed; any other title is left as it is.
func (filer *libraryFiler) bringEpisodeBack(ctx context.Context, titleID int64) error {
	if !filer.returningEpisodes[titleID] {
		return nil
	}
	libraryID := filer.library.ID
	err := filer.queries.MoveItemToLibrary(ctx, sqlcgen.MoveItemToLibraryParams{LibraryID: &libraryID, ID: titleID})
	if err != nil {
		return fmt.Errorf("bring episode %d back: %w", titleID, err)
	}
	delete(filer.returningEpisodes, titleID)
	return nil
}

// joinDeletedSeries joins a series just matched to a deleted library's series the match names, as
// joinExisting joins a film: that series comes back into this library with its history, each
// episode at a place it had takes this one's files and comes back too, and every other episode
// moves into it; this one leaves a tombstone pointing there (D208 section 2, D231 section 7). It
// returns the series that stays: this one when no deleted library's series has the id.
func joinDeletedSeries(
	ctx context.Context, queries *sqlcgen.Queries, provider, providerID string, series sqlcgen.GetSeriesForMatchRow,
	now int64,
) (int64, error) {
	if series.LibraryID == nil {
		return series.ID, nil
	}
	rows, err := queries.ListSeriesByProviderID(ctx, sqlcgen.ListSeriesByProviderIDParams{
		Provider: provider, ProviderID: providerID,
	})
	if err != nil {
		return 0, fmt.Errorf("look up the series %s %s: %w", provider, providerID, err)
	}
	for _, row := range rows {
		if row.ID == series.ID || row.RemovedAt == nil {
			continue
		}
		if joinErr := joinEpisodes(ctx, queries, series.ID, row.ID, *series.LibraryID, now); joinErr != nil {
			return 0, joinErr
		}
		err = queries.MergeItem(ctx, sqlcgen.MergeItemParams{MergedInto: &row.ID, RemovedAt: &now, ID: series.ID})
		if err != nil {
			return 0, fmt.Errorf("merge series %d into %d: %w", series.ID, row.ID, err)
		}
		return row.ID, nil
	}
	return series.ID, nil
}

// episodePlace is an episode's season and number in its series' aired order.
type episodePlace [2]int64

func placeOf(place sqlcgen.ListSeriesEpisodePlacesRow) episodePlace {
	return episodePlace{ptr.ValueOrZero(place.GroupNumber), ptr.ValueOrZero(place.NumberInGroup)}
}

// seriesJoin is one series' episodes joining another's, which is kept, in a library.
type seriesJoin struct {
	queries                    *sqlcgen.Queries
	fromSeries, keptSeries     int64
	fromOrdering, keptOrdering int64
	libraryID, now             int64
}

// joinEpisodes brings the series kept back into a library and moves another series' episodes into
// it: an episode at a place the kept series has one gives it its versions and leaves a tombstone
// pointing there; any other moves, place and all.
func joinEpisodes(ctx context.Context, queries *sqlcgen.Queries, fromSeries, keptSeries, libraryID, now int64) error {
	join := seriesJoin{queries: queries, fromSeries: fromSeries, keptSeries: keptSeries, libraryID: libraryID, now: now}
	if err := join.bringBack(ctx, keptSeries); err != nil {
		return err
	}
	keptPlaces, err := queries.ListSeriesEpisodePlaces(ctx, keptSeries)
	if err != nil {
		return fmt.Errorf("list the episodes of series %d: %w", keptSeries, err)
	}
	keptAt := make(map[episodePlace]int64, len(keptPlaces))
	for _, place := range keptPlaces {
		keptAt[placeOf(place)] = place.TitleID
	}
	fromPlaces, err := queries.ListSeriesEpisodePlaces(ctx, fromSeries)
	if err != nil {
		return fmt.Errorf("list the episodes of series %d: %w", fromSeries, err)
	}
	if join.fromOrdering, err = airedOrderingOf(ctx, queries, fromSeries); err != nil {
		return err
	}
	if join.keptOrdering, err = airedOrderingOf(ctx, queries, keptSeries); err != nil {
		return err
	}
	for _, place := range fromPlaces {
		if kept, isAtAKeptPlace := keptAt[placeOf(place)]; isAtAKeptPlace {
			err = join.mergeEpisode(ctx, place.TitleID, kept)
		} else {
			err = join.moveEpisode(ctx, place.TitleID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (join seriesJoin) bringBack(ctx context.Context, itemID int64) error {
	err := join.queries.MoveItemToLibrary(ctx, sqlcgen.MoveItemToLibraryParams{LibraryID: &join.libraryID, ID: itemID})
	if err != nil {
		return fmt.Errorf("bring item %d back: %w", itemID, err)
	}
	return nil
}

// mergeEpisode gives an episode's versions to the kept series' episode at its place, which comes
// back, and leaves a tombstone pointing there.
func (join seriesJoin) mergeEpisode(ctx context.Context, episode, kept int64) error {
	move := sqlcgen.MoveVersionsBetweenTitlesParams{ToTitle: kept, FromTitle: episode}
	err := join.queries.MoveVersionsBetweenTitles(ctx, move)
	if err == nil {
		err = join.bringBack(ctx, kept)
	}
	if err == nil {
		err = join.queries.MergeItem(ctx, sqlcgen.MergeItemParams{MergedInto: &kept, RemovedAt: &join.now, ID: episode})
	}
	if err != nil {
		return fmt.Errorf("join episode %d to %d: %w", episode, kept, err)
	}
	return nil
}

// moveEpisode moves an episode into the kept series, at the same place in its aired order.
func (join seriesJoin) moveEpisode(ctx context.Context, episode int64) error {
	err := join.queries.MoveEpisodeToSeries(ctx, sqlcgen.MoveEpisodeToSeriesParams{
		ToSeries: join.keptSeries, FromSeries: join.fromSeries, TitleID: &episode,
	})
	if err == nil {
		err = join.queries.MovePositionToOrdering(ctx, sqlcgen.MovePositionToOrderingParams{
			ToOrdering: join.keptOrdering, FromOrdering: join.fromOrdering, TitleID: episode,
		})
	}
	if err != nil {
		return fmt.Errorf("move episode %d into series %d: %w", episode, join.keptSeries, err)
	}
	return nil
}

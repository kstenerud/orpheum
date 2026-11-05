// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// withProgress gives a page's entries how far userID is through each (the owner, 2026-10-25: "for
// series, the progress bar at the bottom shows how far along they have watched"): a film the percent
// of its file Continue Watching would resume it at, by the same rule; a series how many of its
// episodes they watched, of how many it has, in the ordering they watch it in; and whether they
// watched each (D370). A constant number of statements, in one read, whatever the page.
func (browser *Browser) withProgress(ctx context.Context, reach auth.Reach, userID int64, page Page) (Page, error) {
	if len(page.Entries) == 0 {
		return page, nil
	}
	thresholds, err := ReadResumeThresholds(ctx, browser.settings)
	if err != nil {
		return Page{}, err
	}
	var series, titles []Entry
	for _, entry := range page.Entries {
		if entry.Kind == ListSeries {
			series = append(series, entry)
		} else {
			titles = append(titles, entry)
		}
	}
	seriesIDs, titleIDs := idsBySeries(page.Entries)
	err = browser.database.ReadTx(ctx, func(queries *sqlcgen.Queries) error {
		if len(series) > 0 {
			viewer := Viewer{UserID: userID, Reach: reach}
			seriesErr := seriesProgress(ctx, queries, viewer, seriesIDs, series)
			if seriesErr != nil {
				return seriesErr
			}
		}
		if len(titles) == 0 {
			return nil
		}
		return filmsProgress(ctx, queries, thresholds, userID, titleIDs, titles)
	})
	if err != nil {
		return Page{}, err
	}
	progressed := map[int64]Entry{}
	for _, entry := range slices.Concat(series, titles) {
		progressed[entry.ID] = entry
	}
	for index := range page.Entries {
		page.Entries[index] = progressed[page.Entries[index].ID]
	}
	return page, nil
}

// filmsProgress sets each film's WatchedPercent: of its positions, most recent first, the first
// Continue Watching resumes at, when its file's length is known; and whether its person watched it
// (D370 §1).
func filmsProgress(
	ctx context.Context, queries *sqlcgen.Queries, thresholds ResumeThresholds, userID int64, ids []int64,
	entries []Entry,
) error {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("encode %d title ids: %w", len(ids), err)
	}
	rows, err := queries.ListWorksPlayPositions(ctx, sqlcgen.ListWorksPlayPositionsParams{
		WorkIdsJSON: string(encoded), UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("read user %d's positions in %d films: %w", userID, len(ids), err)
	}
	latestWatchedAt := map[int64]int64{} // a film's most recent watching, in any of its files
	decided := map[int64]bool{}
	percent := map[int64]int64{}
	for _, row := range rows {
		if IsBeforeMark(row.PositionReadAt, row.MarkedAt) {
			continue // marked by hand since: a fresh start (D370 §4)
		}
		if _, seen := latestWatchedAt[row.WorkID]; !seen {
			latestWatchedAt[row.WorkID] = row.PositionReadAt
		}
		if decided[row.WorkID] ||
			!thresholds.resumesAt(row.PositionMs, row.DurationMs, row.HiddenAt, latestWatchedAt[row.WorkID]) {
			continue
		}
		decided[row.WorkID] = true
		if row.DurationMs > 0 {
			percent[row.WorkID] = row.PositionMs * 100 / row.DurationMs
		}
	}
	watched, err := watchedTitles(ctx, queries, userID, ids)
	if err != nil {
		return err
	}
	for index := range entries {
		entries[index].WatchedPercent = percent[entries[index].ID]
		entries[index].IsWatched = watched[entries[index].ID]
	}
	return nil
}

// seriesProgress sets each series' EpisodeCount and FinishedEpisodeCount, in the ordering its viewer
// watches it in (ViewingOrderings): its members with a file at a place they reach, and those of them
// they watched, by playing or by hand (D370 §5); and IsWatched once they watched every one (§3).
func seriesProgress(
	ctx context.Context, queries *sqlcgen.Queries, viewer Viewer, ids []int64, entries []Entry,
) error {
	reach, userID := viewer.Reach, viewer.UserID
	orderings, err := ViewingOrderings(ctx, queries, userID, ids)
	if err != nil {
		return err
	}
	named := make([][2]any, 0, len(ids))
	for _, id := range ids {
		named = append(named, [2]any{id, orderings[id]})
	}
	encoded, err := json.Marshal(named)
	if err != nil {
		return fmt.Errorf("encode %d series' orderings: %w", len(ids), err)
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	counts, err := queries.CountOrderingMembers(ctx, sqlcgen.CountOrderingMembersParams{
		UserID: userID, OrderingsJSON: string(encoded), IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	if err != nil {
		return fmt.Errorf("count the episodes of %d series: %w", len(ids), err)
	}
	byID := map[int64]sqlcgen.CountOrderingMembersRow{}
	for _, row := range counts {
		byID[row.SequenceID] = row
	}
	for index := range entries {
		count := byID[entries[index].ID]
		entries[index].EpisodeCount, entries[index].FinishedEpisodeCount = count.MemberCount, count.WatchedCount
		entries[index].IsWatched = count.MemberCount > 0 && count.WatchedCount == count.MemberCount
	}
	return nil
}

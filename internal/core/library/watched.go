// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNotWatchable is a title no person marks watched: an extra, which is a different work and never
// shows the mark (D370 §2, D162).
var ErrNotWatchable = errors.New("an extra is never marked watched")

// IsBeforeMark is whether a position watched at readAt was watched at or before its title's last
// mark by hand, markedAt, nil for none: a mark is a fresh start, so such a position is not where the
// title resumes, nor partway through it (D370 §4).
func IsBeforeMark(readAt int64, markedAt *int64) bool {
	return markedAt != nil && readAt <= *markedAt
}

// watchedTitles are those of titleIDs userID has watched (D370 §1), read in one statement.
func watchedTitles(ctx context.Context, queries *sqlcgen.Queries, userID int64, titleIDs []int64) (
	map[int64]bool, error,
) {
	watched := map[int64]bool{}
	if len(titleIDs) == 0 {
		return watched, nil
	}
	encoded, err := json.Marshal(titleIDs)
	if err != nil {
		return nil, fmt.Errorf("encode %d title ids: %w", len(titleIDs), err)
	}
	rows, err := queries.ListWorksWatched(ctx, sqlcgen.ListWorksWatchedParams{
		WorkIdsJSON: string(encoded), UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("read user %d's watched marks of %d titles: %w", userID, len(titleIDs), err)
	}
	for _, row := range rows {
		watched[row.WorkID] = row.WatchedAt != nil
	}
	return watched, nil
}

// WatchedTitles are those of titleIDs userID has watched, by playing or by hand (D370 §1).
func (browser *Browser) WatchedTitles(ctx context.Context, userID int64, titleIDs []int64) (map[int64]bool, error) {
	return watchedTitles(ctx, browser.database.Read(), userID, titleIDs)
}

// TitleToMark is the one title a person marks watched or unwatched on its page (D370 §4): a film, an
// episode or an other video reach reaches. ErrNoItem for one it does not reach or that is not a title,
// ErrNotWatchable for an extra.
func (browser *Browser) TitleToMark(ctx context.Context, reach auth.Reach, titleID int64) error {
	queries := browser.database.Read()
	title, err := store.OrNotFound(queries.GetWorkDetail(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("title %d: %w", titleID, ErrNoItem)
	}
	if err != nil {
		return fmt.Errorf("read title %d: %w", titleID, err)
	}
	if err := ReachItem(ctx, queries, reach, titleID); err != nil {
		return err
	}
	if title.Kind == TitleKindExtra {
		return fmt.Errorf("title %d: %w", titleID, ErrNotWatchable)
	}
	return nil
}

// EpisodesToMark are the episodes a person marks watched or unwatched on a series' page, or on one
// season's, a group of the ordering they watch it in (D370 §4): those with a file at a place they
// reach, as its progress counts them, present now; ones that arrive later are not marked. ErrNoItem
// for a series reach does not reach.
func (browser *Browser) EpisodesToMark(
	ctx context.Context, reach auth.Reach, userID, seriesID int64, season *int64,
) ([]int64, error) {
	if _, err := browser.reachedSeries(ctx, reach, seriesID); err != nil {
		return nil, err
	}
	queries := browser.database.Read()
	ordering, err := ViewingOrdering(ctx, queries, userID, seriesID)
	if err != nil {
		return nil, err
	}
	members, err := ReadMembers(ctx, queries, browser.settings, Viewer{UserID: userID, Reach: reach}, seriesID,
		ordering)
	if err != nil {
		return nil, err
	}
	if season != nil {
		members = MembersOfGroup(members, *season)
	}
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.TitleID)
	}
	return ids, nil
}

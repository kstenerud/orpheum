// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNoItem is an item that is not there to hide: never known, or removed.
var ErrNoItem = errors.New("no such item")

// Hide takes a work out of the person's Continue Watching until it next has activity (D188).
// Hiding is not watching: nothing is marked finished, and the position stays.
func (service *Service) Hide(ctx context.Context, userID, itemID int64) error {
	if _, err := store.OrNotFound(service.database.Read().GetWorkDetail(ctx, itemID)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNoItem
		}
		return fmt.Errorf("read item %d: %w", itemID, err)
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.HideProgress(ctx, sqlcgen.HideProgressParams{
			UserID: userID, ItemID: itemID, HiddenAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("hide item %d: %w", itemID, err)
	}
	return nil
}

// Unhide undoes Hide: the work is back in Continue Watching if it is still partway through.
func (service *Service) Unhide(ctx context.Context, userID, itemID int64) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UnhideProgress(ctx, sqlcgen.UnhideProgressParams{UserID: userID, ItemID: itemID})
	})
	if err != nil {
		return fmt.Errorf("unhide item %d: %w", itemID, err)
	}
	return nil
}

// WatchStats are what a person has watched, for their page.
type WatchStats struct {
	// WatchedMs is the time in their spans of watching.
	WatchedMs     int64
	TitlesWatched int64
	// LastWatchedAt is when their latest span ended, or the zero time when they watched nothing.
	LastWatchedAt time.Time
}

// WatchStatsOf reads a person's watching from their spans, and from what deleted files left on their
// versions (D165, D306 §8).
func (service *Service) WatchStatsOf(ctx context.Context, userID int64) (WatchStats, error) {
	row, err := service.database.Read().GetWatchStats(ctx, userID)
	if err != nil {
		return WatchStats{}, fmt.Errorf("read user %d's watching: %w", userID, err)
	}
	stats := WatchStats{WatchedMs: row.WatchedMs, TitlesWatched: row.WorksWatchedCount}
	if row.LastWatchedAt != 0 {
		stats.LastWatchedAt = time.UnixMilli(row.LastWatchedAt)
	}
	return stats, nil
}

// MoveHidesOf gives the title that stays each person's hiding of a merged title's progress, the later
// where both were hidden, as the merge gives it the versions (D188): left on the tombstone, it would
// no longer hide them. It writes through queries, the transaction of the merge (identify's
// joinExisting and mergeEpisode).
func MoveHidesOf(ctx context.Context, queries *sqlcgen.Queries, fromItem, toItem int64) error {
	move := sqlcgen.MoveProgressHidesToItemParams{ToItem: toItem, FromItem: fromItem}
	err := queries.MoveProgressHidesToItem(ctx, move)
	if err == nil {
		err = queries.DeleteProgressHidesOfItem(ctx, fromItem)
	}
	if err != nil {
		return fmt.Errorf("move title %d's hidden progress to %d: %w", fromItem, toItem, err)
	}
	return nil
}

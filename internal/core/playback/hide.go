// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNoItem is an item that is not there to hide: never known, or removed.
var ErrNoItem = errors.New("no such item")

// Hide takes a work out of the person's Continue Watching until it next has activity (D188).
// Hiding is not watching: nothing is marked finished, and the position stays.
func (service *Service) Hide(ctx context.Context, userID, itemID int64) error {
	if _, err := store.One(service.database.Read().GetTitleDetail(ctx, itemID)); err != nil {
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

// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ReachFile is nil when reach reaches the file's library, and ErrNoFile otherwise, as for a file
// that does not exist: a library a person may not reach is absent from every answer (D243 §4).
func ReachFile(ctx context.Context, queries *sqlcgen.Queries, reach auth.Reach, fileID int64) error {
	if reach.IsEveryLibrary() {
		return nil
	}
	libraryID, err := store.OrNotFound(queries.GetLibraryOfFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && !reach.Reaches(&libraryID)) {
		return fmt.Errorf("file %d: %w", fileID, ErrNoFile)
	}
	if err != nil {
		return fmt.Errorf("read file %d's library: %w", fileID, err)
	}
	return nil
}

// ReachItem is nil when reach reaches the item's library, and ErrNoItem otherwise, as for an item
// that does not exist (D243 §4).
func ReachItem(ctx context.Context, queries *sqlcgen.Queries, reach auth.Reach, itemID int64) error {
	if reach.IsEveryLibrary() {
		return nil
	}
	libraryID, err := store.OrNotFound(queries.GetLibraryOfItem(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && !reach.Reaches(libraryID)) {
		return fmt.Errorf("item %d: %w", itemID, ErrNoItem)
	}
	if err != nil {
		return fmt.Errorf("read item %d's library: %w", itemID, err)
	}
	return nil
}

// ReachFile is ReachFile on the browser's database: the check a request about one file makes
// before anything else (D243 §4).
func (browser *Browser) ReachFile(ctx context.Context, reach auth.Reach, fileID int64) error {
	return ReachFile(ctx, browser.database.Read(), reach, fileID)
}

// ReachItem is ReachItem on the browser's database.
func (browser *Browser) ReachItem(ctx context.Context, reach auth.Reach, itemID int64) error {
	return ReachItem(ctx, browser.database.Read(), reach, itemID)
}

// ReachArt is whether reach reaches an item that holds the image whose source has this SHA-256:
// an image is served to whoever may see something that shows it (D243 §4).
func (browser *Browser) ReachArt(ctx context.Context, reach auth.Reach, sourceSha256 []byte) (bool, error) {
	if reach.IsEveryLibrary() {
		return true, nil
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	isReached, err := browser.database.Read().IsArtworkReached(ctx, sqlcgen.IsArtworkReachedParams{
		SourceSha256: sourceSha256, IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	if err != nil {
		return false, fmt.Errorf("read who reaches an image: %w", err)
	}
	return isReached, nil
}

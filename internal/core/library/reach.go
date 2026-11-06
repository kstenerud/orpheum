// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ReachFile is nil when a library reach reaches covers a place of the file, and ErrNoFile otherwise,
// as for a file that does not exist: a library a person may not reach, and a place none they reach
// covers, is absent from every answer (D243 §4, D363 §8); but a file of the system library, which any
// signed-in person may play (D435 §6).
func ReachFile(ctx context.Context, queries *sqlcgen.Queries, reach auth.Reach, fileID int64) error {
	if reach.IsEveryLibrary() {
		return nil
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	isReached, err := queries.IsFileReached(ctx, sqlcgen.IsFileReachedParams{
		FileID: fileID, IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	if err != nil {
		return fmt.Errorf("read who reaches file %d: %w", fileID, err)
	}
	if isReached != 0 {
		return nil
	}
	// The one exemption: a file of the system library is the server's own, which a feature plays for
	// any signed-in person who asked for it by what it is (D435 §6). No role reaches that library.
	isSystem, err := queries.IsFileInSystemLibrary(ctx, fileID)
	if err != nil {
		return fmt.Errorf("read whether file %d is the server's own: %w", fileID, err)
	}
	if isSystem == 0 {
		return fmt.Errorf("file %d: %w", fileID, ErrNoFile)
	}
	return nil
}

// ReachItem is nil when a library reach reaches lists the item, and ErrNoItem otherwise, as for an
// item that does not exist (D243 §4, D363 §8).
func ReachItem(ctx context.Context, queries *sqlcgen.Queries, reach auth.Reach, itemID int64) error {
	if reach.IsEveryLibrary() {
		return nil
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	isReached, err := queries.IsItemReached(ctx, sqlcgen.IsItemReachedParams{
		ItemID: itemID, IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	if err != nil {
		return fmt.Errorf("read who reaches item %d: %w", itemID, err)
	}
	if isReached == 0 {
		return fmt.Errorf("item %d: %w", itemID, ErrNoItem)
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
	return isReached == 1, nil
}

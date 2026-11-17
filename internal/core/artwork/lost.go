// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// SweepLost is the sweep that finds the images the art directory lost while the core runs (D510
// §2): it lists the directory afresh, since what the holdings in memory say it holds is only what
// the core wrote and removed itself, and retires the records of each image recorded as stored that
// it holds no WebP of, as a lookup that finds one gone does. It queues nothing itself: each library
// whose images it retired owes a pass, which does.
func (service *Service) SweepLost(ctx context.Context) (int, error) {
	// Read before the listing: an image's sizes are written before its record, so one stored while
	// the directory is listed is never taken for lost.
	stored, err := service.parts.Database.Read().ListStoredArtworkImages(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the images recorded as stored: %w", err)
	}
	files, err := service.parts.ArtFiles.ListArt()
	if err != nil {
		return 0, fmt.Errorf("list the art directory: %w", err)
	}
	return 0, service.retireLost(ctx, lostOf(stored, files)...)
}

// lostOf are the images, by hash in hex, recorded as stored that the art directory's files hold no
// WebP of: nothing left to serve or to cut a size from. A .none note alone is not the image.
func lostOf(stored [][]byte, files []instancedir.ArtFile) []string {
	held := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file.Name, webpSuffix) {
			held[file.SourceSha256Hex] = true
		}
	}
	var lost []string
	for _, sourceSha256 := range stored {
		if sourceSha256Hex := hex.EncodeToString(sourceSha256); !held[sourceSha256Hex] {
			lost = append(lost, sourceSha256Hex)
		}
	}
	return lost
}

// retireLost puts images recorded as stored, whose sizes the art directory lost, back to not stored
// (D510 §1): every record of each, of any source, a person's choice among them, is retired, as a
// refresh that looks again retires a refusal (RetryItemArtwork); what is left under its hash (a
// .none note made when nothing was there to cut from) goes; and each library that files their items
// owes a pass. The pass is the ordinary planning: it names the provider's and the media's images an
// item does not hold, which are fetched or read again, and plans the frame stills, which are taken
// again, each at the background's priority, where a look at its tile raises it (D324). The image
// stored again for the same source keeps a person's choice (record).
func (service *Service) retireLost(ctx context.Context, sourceSha256Hexes ...string) error {
	if len(sourceSha256Hexes) == 0 {
		return nil
	}
	var itemIDs []int64
	now := service.parts.Clock.Now().UnixMilli()
	err := service.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		itemIDs = nil
		for _, sourceSha256Hex := range sourceSha256Hexes {
			sourceSha256, err := hex.DecodeString(sourceSha256Hex)
			if err != nil {
				return fmt.Errorf("art %q is named by no hash: %w", sourceSha256Hex, err)
			}
			retired, err := queries.RetireArtworkOfLostImage(ctx, sqlcgen.RetireArtworkOfLostImageParams{
				RemovedAt: &now, SourceSha256: sourceSha256,
			})
			if err != nil {
				return fmt.Errorf("retire the records of art %s: %w", sourceSha256Hex, err)
			}
			itemIDs = append(itemIDs, retired...)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(itemIDs) > 0 {
		service.parts.Logger.WarnContext(ctx, "images recorded as stored have no image in the art directory; "+
			"each is made again", "count", len(sourceSha256Hexes), "items", len(itemIDs))
	}
	for _, sourceSha256Hex := range sourceSha256Hexes {
		if forgetErr := service.forgetImage(sourceSha256Hex); forgetErr != nil {
			return forgetErr
		}
	}
	libraryIDs, err := service.filingLibraries(ctx, itemIDs)
	if err != nil {
		return err
	}
	for _, libraryID := range libraryIDs {
		if service.parts.OwePass != nil {
			service.parts.OwePass(ctx, libraryID)
		}
	}
	return nil
}

// forgetImage removes every file the art directory holds of an image, and forgets it.
func (service *Service) forgetImage(sourceSha256Hex string) error {
	held := service.holdings
	held.mutex.Lock()
	var names []string
	for name := range held.sizeBytesByNameBySourceSha256[sourceSha256Hex] {
		names = append(names, name)
	}
	held.mutex.Unlock()
	for _, name := range names {
		if err := service.evict(candidate{sourceSha256Hex: sourceSha256Hex, name: name}); err != nil {
			return err
		}
	}
	return nil
}

// filingLibraries are the libraries whose passes file the items, each once, in order.
func (service *Service) filingLibraries(ctx context.Context, itemIDs []int64) ([]int64, error) {
	var libraryIDs []int64
	for _, itemID := range itemIDs {
		libraryID, err := store.OrNotFound(service.parts.Database.Read().GetFilingLibraryOfItem(ctx, itemID))
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("find the library of item %d: %w", itemID, err)
		}
		if libraryID != nil && !slices.Contains(libraryIDs, *libraryID) {
			libraryIDs = append(libraryIDs, *libraryID)
		}
	}
	slices.Sort(libraryIDs)
	return libraryIDs, nil
}

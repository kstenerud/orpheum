// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// retakeStillsLostAtStart takes again each frame still recorded as taken whose image the art
// directory, as LoadHoldings listed it, does not hold: a restore from backup carries no cache, and a
// still cannot be fetched again as a provider's image is (D507 §3). It logs a failure: the core
// starts as it would with every still held, slower to show them, not wrong.
func (service *Service) retakeStillsLostAtStart(ctx context.Context) {
	hashes, err := service.parts.Database.Read().ListTakenFrameStillImages(ctx)
	if err != nil {
		service.parts.Logger.ErrorContext(ctx, "could not check the frame stills against the art directory", "err", err)
		return
	}
	var lost []string
	for _, hash := range hashes {
		if sourceSha256Hex := hex.EncodeToString(hash); service.heldBaseName(sourceSha256Hex) == "" {
			lost = append(lost, sourceSha256Hex)
		}
	}
	if err := service.retakeLostStills(ctx, lost...); err != nil {
		service.parts.Logger.ErrorContext(ctx, "could not take again the frame stills the art directory lost",
			"count", len(lost), "err", err)
	}
}

// retakeLostStills puts frame stills recorded as taken, whose images the art directory lost, back to
// not taken (D507): their rows are retired as never taken, what is left under their hashes (a .none
// note made when nothing was there to cut from) is removed, and the frame-still planning of each
// library they were of runs, so the background takes them again and a look at their tiles raises
// them (D324), at once rather than at the library's next scan, a day away.
func (service *Service) retakeLostStills(ctx context.Context, sourceSha256Hexes ...string) error {
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
			retired, err := queries.RetireFrameStillsOfImage(ctx, sqlcgen.RetireFrameStillsOfImageParams{
				RemovedAt: &now, SourceSha256: sourceSha256,
			})
			if err != nil {
				return fmt.Errorf("retire the frame stills of art %s: %w", sourceSha256Hex, err)
			}
			itemIDs = append(itemIDs, retired...)
		}
		return nil
	})
	if err != nil {
		return err
	}
	service.parts.Logger.WarnContext(ctx, "frame stills recorded as taken have no image in the art directory; "+
		"each is taken again", "count", len(sourceSha256Hexes), "items", len(itemIDs))
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
		service.planFrameStills(ctx, libraryID, Found{}, nil)
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

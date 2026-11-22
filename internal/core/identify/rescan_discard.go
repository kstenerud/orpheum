// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// discardMade throws out, for a rescan that discards what was made (D535 §6), what the app fetched
// for its films and series whatever differs: each one's kept search results, candidates and the
// runtimes fitted to them (D172, D365), and every image of it and, a series, of its episodes, that a
// person did not choose, so each is fetched or made again. Its match, a person's choice, edits and
// watch state are a person's and stay (D357 §3); its record is fetched again as it is matched again
// (fetchDiscarded).
func (matcher *Matcher) discardMade(ctx context.Context, itemIDs []int64) error {
	for _, itemID := range itemIDs {
		kind, err := store.OrNotFound(matcher.database.Read().GetItemKind(ctx, itemID))
		switch {
		case errors.Is(err, store.ErrNotFound):
			continue
		case err != nil:
			return fmt.Errorf("read item %d: %w", itemID, err)
		}
		if err := matcher.dropCandidates(ctx, itemID); err != nil {
			return err
		}
		imagesOf := []int64{itemID}
		if kind == KindSeries {
			episodes, err := matcher.database.Read().ListLiveSeriesEpisodeSources(ctx, itemID)
			if err != nil {
				return fmt.Errorf("list series %d's episodes: %w", itemID, err)
			}
			for _, episode := range episodes {
				imagesOf = append(imagesOf, episode.WorkID)
			}
		}
		if matcher.imageRetrier == nil {
			continue
		}
		if err := matcher.imageRetrier.DiscardImages(ctx, imagesOf...); err != nil {
			return fmt.Errorf("discard item %d's images: %w", itemID, err)
		}
	}
	return nil
}

// fetchDiscarded fetches again, for a rescan that discards what was made (D535 §6), the record of a
// film or series its match again kept to the same work, or left alone as a person or its NFO set it:
// what was kept of it is thrown out and fetched again as the work it follows. A match that changed
// fetched the new work's already; one gone, unmatched or failed has none to fetch.
func (matcher *Matcher) fetchDiscarded(
	ctx context.Context, itemID int64, row cachegen.GetRematchRow, result rematched,
) error {
	if row.IsDiscarding == 0 {
		return nil
	}
	isKept := result.outcome == rematchUnchanged || result.outcome == rematchLeftAlone &&
		(result.leftAloneBecause == leftAloneChosen || result.leftAloneBecause == leftAloneByNFO)
	if !isKept {
		return nil
	}
	if _, err := matcher.refresh(ctx, itemID, false); err != nil {
		return fmt.Errorf("fetch item %d again: %w", itemID, err)
	}
	return nil
}

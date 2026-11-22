// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// discardMade throws out, for a rescan that discards what was made (D535 §6), what the app fetched
// for its films and series whatever differs: each one's kept search results, candidates and the
// runtimes fitted to them (D172, D365), every image of it and, a series, of its episodes, that a
// person did not choose, so each is fetched or made again, their failures to be fetched, and what is
// kept of the subtitles fetched for their files (FetchedDiscarder): a video page's, isVideoPage, throws
// out the subtitles too. Its match, a person's choice, edits and watch state are a person's and stay
// (D357 §3); its record is fetched again as it is matched again (fetchDiscarded).
func (matcher *Matcher) discardMade(ctx context.Context, itemIDs []int64, isVideoPage bool) error {
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
		if err := matcher.discardFetchedOf(ctx, itemID, imagesOf, isVideoPage); err != nil {
			return err
		}
	}
	return nil
}

// discardFetchedOf throws out, for discardMade, item itemID's images and its works' failures to be
// fetched, works naming it and, a series, its episodes, and what is kept of the subtitles fetched
// for its files.
func (matcher *Matcher) discardFetchedOf(ctx context.Context, itemID int64, works []int64, isVideoPage bool) error {
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, work := range works {
			if err := queries.ClearFetchFailure(ctx, work); err != nil {
				return fmt.Errorf("clear work %d's failure: %w", work, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("clear item %d's failures to be fetched: %w", itemID, err)
	}
	if matcher.imageRetrier != nil {
		if err = matcher.imageRetrier.DiscardImages(ctx, works...); err != nil {
			return fmt.Errorf("discard item %d's images: %w", itemID, err)
		}
	}
	if matcher.fetchedDiscarder == nil {
		return nil
	}
	files, err := matcher.database.Read().ListFilesOfWork(ctx,
		sqlcgen.ListFilesOfWorkParams{ItemID: itemID, MaxRows: math.MaxInt32})
	if err != nil {
		return fmt.Errorf("list item %d's files: %w", itemID, err)
	}
	fileIDs := make([]int64, 0, len(files))
	for _, file := range files {
		if !slices.Contains(fileIDs, file.ID) {
			fileIDs = append(fileIDs, file.ID)
		}
	}
	if err = matcher.fetchedDiscarder.DiscardFetched(ctx, fileIDs, isVideoPage); err != nil {
		return fmt.Errorf("discard the subtitles fetched for item %d: %w", itemID, err)
	}
	return nil
}

// FetchedDiscarder throws out, for a discarding rescan (D535 §6), what is kept of the subtitles
// fetched for files: with isThrowingOutDownloads the subtitles too, else only their text, converted
// again. The subtitles service, which the wiring hands in through SetFetchedDiscarder, so this
// package never imports it.
type FetchedDiscarder interface {
	DiscardFetched(ctx context.Context, fileIDs []int64, isThrowingOutDownloads bool) error
}

// SetFetchedDiscarder sets what a discarding rescan throws out fetched subtitles with. Until it is
// set, a discard leaves them be.
func (matcher *Matcher) SetFetchedDiscarder(discarder FetchedDiscarder) {
	matcher.fetchedDiscarder = discarder
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

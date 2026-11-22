// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// DiscardFetched throws out, for a rescan that discards what was made (D535 §6), what is kept of the
// subtitles fetched for files. A video page's, isThrowingOutDownloads, throws out the subtitles
// themselves, to be found again from the player, since a wrong pick is only undone so; a library's
// or everything's keeps each download, which cost one of the provider's quota, and throws out only
// its text, converted again from the kept download in the background.
func (service *Service) DiscardFetched(ctx context.Context, fileIDs []int64, isThrowingOutDownloads bool) error {
	if len(fileIDs) == 0 {
		return nil
	}
	idsJSON, err := json.Marshal(fileIDs)
	if err != nil {
		return fmt.Errorf("name the files whose fetched subtitles to discard: %w", err)
	}
	fetched, err := service.database.Read().ListFetchedSubtitlesOfFiles(ctx, string(idsJSON))
	if err != nil {
		return fmt.Errorf("list the subtitles fetched for files %v: %w", fileIDs, err)
	}
	for _, each := range fetched {
		key := fetchedKey(each.BodyGzip)
		if isThrowingOutDownloads {
			err = service.discardFetched(ctx, each.ID, key)
		} else {
			err = service.convertAgain(ctx, each.ID, key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// convertAgain throws out the text made of fetched subtitle id, kept where key says, and queues its
// conversion again from its kept download, in the background.
func (service *Service) convertAgain(ctx context.Context, id int64, key sqlcgen.GetSubtitleTextParams) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, err := queries.DeleteSubtitleText(ctx, sqlcgen.DeleteSubtitleTextParams(key))
		return err
	})
	if err != nil {
		return fmt.Errorf("throw out fetched subtitle %d's text: %w", id, err)
	}
	_, err = service.queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindConvertFetchedSubtitle, Target: jobs.Target{FetchedSubtitleID: id},
		Priority: jobs.PriorityBackground, Reason: "a discarding rescan threw out its text",
	})
	if err != nil {
		return fmt.Errorf("queue the conversion of fetched subtitle %d again: %w", id, err)
	}
	return nil
}

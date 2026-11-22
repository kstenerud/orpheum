// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetTextFailures forgets, for every rescan, discarding or not (D535 §8), why the text of a
// subtitle of files could not be made: of each file's own tracks, and of each subtitle fetched for
// it, so each is made again when it is next wanted. A text that was made stays (D535 §5).
func (service *Service) ForgetTextFailures(ctx context.Context, fileIDs []int64) error {
	if len(fileIDs) == 0 {
		return nil
	}
	idsJSON, err := json.Marshal(fileIDs)
	if err != nil {
		return fmt.Errorf("name the files whose subtitles' failures to forget: %w", err)
	}
	fetched, err := service.database.Read().ListFetchedSubtitlesOfFiles(ctx, string(idsJSON))
	if err != nil {
		return fmt.Errorf("list the subtitles fetched for files %v: %w", fileIDs, err)
	}
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, ownErr := queries.ForgetSubtitleTextFailuresOfFiles(ctx, string(idsJSON)); ownErr != nil {
			return ownErr
		}
		for _, each := range fetched {
			key := fetchedKey(each.BodyGzip)
			if fetchedErr := queries.ForgetSubtitleTextFailure(ctx,
				sqlcgen.ForgetSubtitleTextFailureParams(key)); fetchedErr != nil {
				return fmt.Errorf("fetched subtitle %d: %w", each.ID, fetchedErr)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("forget the subtitles' failures of files %v: %w", fileIDs, err)
	}
	return nil
}

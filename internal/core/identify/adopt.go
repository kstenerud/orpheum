// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// adopt has a library's pass file, from now on, the titles of the videos it governs whose filing
// library governs none of their files any more, and their series: a library deleted, or another of
// its kind that now covers them from a folder above (D363 §5, §10). They keep their ids and their
// history, so a library that covers what another did finds it as it was. Read first, it takes the
// writer only when there is one to adopt, and logs each.
func (pass *Pass) adopt(ctx context.Context, libraryID int64) error {
	queries := pass.database.Read()
	titleIDs, err := queries.ListTitlesToAdopt(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("list the titles library %d adopts: %w", libraryID, err)
	}
	seriesIDs, err := queries.ListSeriesToAdopt(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list the series library %d adopts: %w", libraryID, err)
	}
	if len(titleIDs) == 0 && len(seriesIDs) == 0 {
		return nil
	}
	err = pass.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if adoptErr := setFilingLibrary(ctx, queries, libraryID, titleIDs); adoptErr != nil {
			return adoptErr
		}
		// A series follows its episodes once they are this library's.
		followed, listErr := queries.ListSeriesToAdopt(ctx, &libraryID)
		if listErr != nil {
			return fmt.Errorf("list the series library %d adopts: %w", libraryID, listErr)
		}
		if adoptErr := setFilingLibrary(ctx, queries, libraryID, followed); adoptErr != nil {
			return adoptErr
		}
		pass.logger.InfoContext(ctx, "a library's pass files titles another library filed, which files them no more",
			"libraryId", libraryID, "titleIds", titleIDs, "seriesIds", followed)
		return nil
	})
	if err != nil {
		return fmt.Errorf("adopt titles into library %d: %w", libraryID, err)
	}
	return nil
}

// setFilingLibrary has libraryID's pass file the items named.
func setFilingLibrary(ctx context.Context, queries *sqlcgen.Queries, libraryID int64, itemIDs []int64) error {
	if len(itemIDs) == 0 {
		return nil
	}
	encoded, err := json.Marshal(itemIDs)
	if err != nil {
		return fmt.Errorf("encode %d item ids: %w", len(itemIDs), err)
	}
	err = queries.SetItemsFilingLibrary(ctx, sqlcgen.SetItemsFilingLibraryParams{
		LibraryID: &libraryID, ItemIdsJSON: string(encoded),
	})
	if err != nil {
		return fmt.Errorf("adopt %d items into library %d: %w", len(itemIDs), libraryID, err)
	}
	return nil
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// D508 §2: a fetch whose own arguments cannot be read says nothing of its item, so it is tried
// again, never the item's own failure for good.
func TestHandle_AFetchWhoseArgumentsCannotBeReadIsTriedAgain(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	err := lib.matcher.Handle(t.Context(), jobs.Job{
		ID: 3, Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: titleID}, ArgsJSON: `{"isRefresh":`,
	})
	if err == nil {
		t.Fatal("a fetch whose arguments cannot be read succeeded; want it failed")
	}
	if jobs.IsMarkedPermanent(err) {
		t.Errorf("the fetch failed for good (%v); want it tried again, its arguments saying nothing of the "+
			"film (D508 §2)", err)
	}
}

// D508 §2: a rematch whose ids kept in cache.db cannot be read says nothing of its film, so its fetch
// is tried again, never the film's own failure for good.
func TestRescan_ARematchWhoseIdsCannotBeReadIsTriedAgain(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	titleID := lib.titlesByPath()[crowdFile].WorkID
	err := lib.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		rescanID, err := queries.InsertRescan(t.Context(), cachegen.InsertRescanParams{
			LibraryID: lib.libraryID, RequestedAt: 1,
		})
		if err != nil {
			return fmt.Errorf("plant a rescan: %w", err)
		}
		_, err = queries.InsertRematch(t.Context(), cachegen.InsertRematchParams{
			ItemID: titleID, RescanID: rescanID, Name: "Crowd", Year: 1928, IdsJSON: `{"tmdb":`,
		})
		if err != nil {
			return fmt.Errorf("plant its rematch: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rescan := lib.rescanning() // counts the planted rescan running, as a restart would
	err = rescan.matcher.Handle(t.Context(), jobs.Job{
		Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: titleID},
	})
	if err == nil {
		t.Fatal("the fetch of a rematch whose ids cannot be read succeeded; want it failed")
	}
	if jobs.IsMarkedPermanent(err) {
		t.Errorf("the fetch failed for good (%v); want it tried again, the ids saying nothing of the film "+
			"(D508 §2)", err)
	}
}

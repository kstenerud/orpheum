// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// A raise takes the writer only when it changes a job: the interest raiser runs every 5 s, and
// repeats its raises for good each minute (the audit of 2026-10-25, C4).
func TestRaiseJobsOfInterest_WritesOnlyAChange(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	for itemID := int64(1); itemID <= 20; itemID++ {
		storage.insert("fetch_metadata", JobTarget{ItemID: itemID}, 0, jobStart)
	}
	storage.insert("read_through", JobTarget{FileID: 7}, 0, jobStart)
	raise := func() (*querytally.Tally, []RaisedJob) {
		ctx, tally := querytally.New(t.Context())
		raised, err := storage.jobs.RaiseJobsOfInterest(ctx, []int64{3}, []int64{7}, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return tally, raised
	}
	first, raised := raise()
	if len(raised) != 2 || !raised[0].HasRisen || !raised[1].HasRisen || first.Writes() != 1 {
		t.Errorf("the first raise = %+v in %d writes; want item 3's and file 7's jobs raised, in one", raised,
			first.Writes())
	}
	again, raised := raise()
	if len(raised) != 2 || raised[0].HasRisen || raised[0].Priority != 50 || again.Writes() != 0 {
		t.Errorf("the same raise again = %+v in %d writes; want both at 50 already, and no writer taken",
			raised, again.Writes())
	}
}

// The interest's reads and releases look up the open jobs of the items and files they are about by
// their own indexes, never by reading every open job (the audit of 2026-10-25, C4).
func TestInterestQueries_LookUpEachItemsAndFilesOpenJobs(t *testing.T) {
	t.Parallel()
	cache := openCacheAt(t, filepath.Join(t.TempDir(), "plans.db"))
	for _, query := range queriesIn(t, filepath.Join("cache", "queries")) {
		if query.name != "ListOpenJobsOfItemsAndFiles" && query.name != "ReleaseJobsOfItemsAndFiles" {
			continue
		}
		plan := fmt.Sprint(planOf(t, cache.database.readers, query))
		if !strings.Contains(plan, "job_open_item") || !strings.Contains(plan, "job_open_file") {
			t.Errorf("%s plans %s; want each id looked up by job_open_item and job_open_file", query.name, plan)
		}
	}
}

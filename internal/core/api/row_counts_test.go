// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// setSetting sets a server setting, as a person's change through /config would.
func (harness testHarness) setSetting(t *testing.T, key, valueJSON string) {
	t.Helper()
	settingstest.Set(t, harness.settings, settings.Key(key), valueJSON)
}

// How many rows Continue Watching and the job view list are the admin's settings, not numbers in
// code.
func TestRowCounts_ContinueWatchingAndTheJobViewListAsManyAsTheirSettingsSay(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.nodes.roots = []string{"/m"}         // the shelf's node has said hello
	harness.setPositionReadAt(t, 2, 60_000, 150) // the pilot, of unknown length
	harness.setPositionReadAt(t, 1, 500, 200)    // Metropolis, of unknown length, latest
	for fileID := range int64(3) {
		request := jobs.Request{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 100 + fileID}}
		if _, err := harness.queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	harness.setSetting(t, "continue_watching_max_works", "1")
	harness.setSetting(t, "job_view_max_jobs", "2")

	if rows := harness.continueWatching(t, admin); len(rows) != 1 || rows[0].ID != 10 {
		t.Errorf("Continue Watching at continue_watching_max_works 1: %v, want only Metropolis, the latest", rowsOf(rows))
	}
	if listed := harness.listedJobs(t, admin); len(listed) != 2 {
		t.Errorf("the job view at job_view_max_jobs 2 lists %d jobs, want 2", len(listed))
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// setSetting sets a server setting, as a person's change through /config would.
func (harness testHarness) setSetting(t *testing.T, key, valueJSON string) {
	t.Helper()
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE setting SET value_json = '"+valueJSON+"', is_modified = 1 WHERE key = '"+key+"'")
}

// How many rows Continue Watching, the job view and the System page list are the admin's
// settings, not numbers in code.
func TestRowCounts_ContinueWatchingTheJobViewAndTheSystemPageListAsManyAsTheirSettingsSay(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.nodes.roots = []string{"/m"}         // the shelf's node has said hello
	harness.setPositionReadAt(t, 2, 60_000, 150) // the pilot, of unknown length
	harness.setPositionReadAt(t, 1, 500, 200)    // Metropolis, of unknown length, latest
	for fileID := range int64(3) {
		request := jobs.Request{Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: 100 + fileID}}
		if _, err := harness.queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	finishPrepare(t, harness, 1, "h264_qsv", 6.5)
	finishPrepare(t, harness, 2, "h264_qsv", 6.5)
	harness.setSetting(t, "continue_watching_max_works", "1")
	harness.setSetting(t, "job_view_max_jobs", "2")
	harness.setSetting(t, "system_page_recent_prepares_per_node", "1")

	if rows := harness.continueWatching(t, admin); len(rows) != 1 || rows[0].ID != 10 {
		t.Errorf("Continue Watching at continue_watching_max_works 1: %v, want only Metropolis, the latest", rowsOf(rows))
	}
	if listed := harness.listedJobs(t, admin); len(listed) != 2 {
		t.Errorf("the job view at job_view_max_jobs 2 lists %d jobs, want 2", len(listed))
	}
	answer := harness.do(t, http.MethodGet, "/system", admin, nil)
	var system struct {
		Nodes []SystemNodeBody `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(answer.body), &system); err != nil || len(system.Nodes) != 1 {
		t.Fatalf("GET /system = %d %s", answer.status, answer.body)
	}
	if prepares := system.Nodes[0].RecentPrepares; len(prepares) != 1 {
		t.Errorf("the System page at system_page_recent_prepares_per_node 1 lists %+v, want one", prepares)
	}
}

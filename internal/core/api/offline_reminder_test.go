// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestOfflineReminder_ShowsWhatIsOfflineUntilDismissedThenReturnsAfterItsInterval(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	reminder := func() OfflineReminderOutput {
		t.Helper()
		got := harness.do(t, http.MethodGet, "/offline-reminder", admin, nil)
		var output OfflineReminderOutput
		if err := json.Unmarshal([]byte(got.body), &output.Body); err != nil || got.status != http.StatusOK {
			t.Fatalf("GET /offline-reminder = %d %s", got.status, got.body)
		}
		return output
	}
	if shown := reminder(); shown.Body.IsShown {
		t.Errorf("with nothing offline the reminder is %+v, want it hidden", shown.Body)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, unavailable_since)
		 VALUES (1, '/media', 'unreadable', 9000, 5000)`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present, missing_since)
		 VALUES (1, '/media', '/media/disk1', 21, 0, 3000)`)
	shown := reminder().Body
	if !shown.IsShown || shown.RootCount != 1 || shown.DiskCount != 1 || shown.OfflineAt != 3000 {
		t.Errorf("reminder = %+v, want it shown: one folder and one disk, since the disk's 3000", shown)
	}
	dismissed := harness.do(t, http.MethodPost, "/offline-reminder/dismissal", admin, nil)
	if got := dismissed; got.status != http.StatusNoContent {
		t.Fatalf("POST /offline-reminder/dismissal = %d %s", got.status, got.body)
	}
	if reminder().Body.IsShown {
		t.Error("the reminder is shown just after the admin dismissed it")
	}
	harness.clock.Advance(7*24*time.Hour - time.Minute)
	if reminder().Body.IsShown {
		t.Error("the reminder came back before offline_reminder_interval_days, 7, passed")
	}
	harness.clock.Advance(time.Minute)
	if !reminder().Body.IsShown {
		t.Error("the reminder did not come back after offline_reminder_interval_days while the folder is still offline")
	}
}

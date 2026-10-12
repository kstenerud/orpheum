// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The admin's needs-attention counts are what is offline, folder and disk apart, and nothing while
// nothing is (D280); a person who is not an admin is refused.
func TestNeedsAttention_CountsTheFoldersAndDisksOffline(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	counts := func() NeedsAttentionOutput {
		t.Helper()
		got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
		var output NeedsAttentionOutput
		if err := json.Unmarshal([]byte(got.body), &output.Body); err != nil || got.status != http.StatusOK {
			t.Fatalf("GET /needs-attention = %d %s", got.status, got.body)
		}
		return output
	}
	if nothing := counts().Body; nothing.RootCount+nothing.DiskCount+nothing.WaitingRootCount+nothing.UnreadCount != 0 {
		t.Errorf("with nothing offline: %+v, want nothing counted", nothing)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, unavailable_since)
		 VALUES (1, '/media', 'unreadable', 9000, 5000)`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present, missing_since)
		 VALUES (1, '/media', '/media/disk1', 21, 0, 3000)`)
	if offline := counts().Body; offline.RootCount != 1 || offline.DiskCount != 1 {
		t.Errorf("counts = %+v, want one folder and one disk offline", offline)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	person := harness.signIn(t, "partner", "partner-password")
	if got := harness.do(t, http.MethodGet, "/needs-attention", person, nil); got.status != http.StatusForbidden {
		t.Errorf("GET /needs-attention by a person who is not an admin = %d, want 403", got.status)
	}
}

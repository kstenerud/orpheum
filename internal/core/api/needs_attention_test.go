// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// The owner, 2026-10-13: "Needs a match should show up as one of the things under the exclamation
// point tray icon." A film waiting for a person's choice is counted.
func TestNeedsAttention_CountsWhatWaitsForAMatch(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	var output NeedsAttentionOutput
	if err := json.Unmarshal([]byte(got.body), &output.Body); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /needs-attention = %d %s", got.status, got.body)
	}
	if output.Body.MatchCount != 1 {
		t.Errorf("counts = %+v; want the one film waiting for its match", output.Body)
	}
}

// A series some of whose episodes have no number outside a specials or extras folder is told, its
// files' names likely read wrongly (D306 §6).
func TestNeedsAttention_TellsTheSeriesWhoseEpisodesWereLikelyReadWrongly(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	var statements []string
	for index := range 5 {
		id := 40 + index
		statements = append(statements,
			fmt.Sprintf("INSERT INTO item (id, kind, library_id, added_at) VALUES (%d, 'title', 1, 2)", id),
			fmt.Sprintf("INSERT INTO title (id, kind, identification_state) VALUES (%d, 'episode', 'unidentified')",
				id),
			fmt.Sprintf("INSERT INTO version (id, title_id, created_at) VALUES (%d, %d, 2)", id, id),
			fmt.Sprintf(`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id,
			 first_seen_at, last_seen_at) VALUES (%d, 1, 1, '/m/BB/Brass Arena - 1x%d.mkv', 10, X'0%d', 'listed',
			 %d, 1, 1)`, id, index+2, index+3, id),
			fmt.Sprintf(`INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order)
			 VALUES (1, %d, 0, NULL, %d)`, id, 20000+index))
	}
	storetest.ExecRaw(t, harness.databasePath, statements...)
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	if !strings.Contains(got.body, `"seriesReadWrongly":[{"seriesId":20,"name":"Brass Arena","unplacedCount":5}]`) {
		t.Errorf("GET /needs-attention = %d %s; want Brass Arena's five unplaced episodes told", got.status, got.body)
	}
}

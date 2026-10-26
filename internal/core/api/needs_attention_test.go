// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The admin's needs-attention counts are what is offline, folder and disk apart, and nothing while
// nothing is (D280); a person who is not an admin is refused.
func TestNeedsAttention_CountsTheFoldersAndDisksOffline(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	counts := func() api.NeedsAttentionOutput {
		t.Helper()
		got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
		var output api.NeedsAttentionOutput
		if err := json.Unmarshal([]byte(got.body), &output.Body); err != nil || got.status != http.StatusOK {
			t.Fatalf("GET /needs-attention = %d %s", got.status, got.body)
		}
		return output
	}
	if nothing := counts().Body; nothing.RootCount+nothing.DiskCount+nothing.WaitingRootCount+nothing.UnreadCount != 0 {
		t.Errorf("with nothing offline: %+v, want nothing counted", nothing)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 9000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media', '/media', 5000, 'unread')`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/media', '/media/disk1', 21, 0)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media/disk1', '/media', 3000, 'missing')`)
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
	var output api.NeedsAttentionOutput
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
		id := int64(40 + index)
		librarytest.Title(t, harness.database, librarytest.TitleSpec{
			ID: id, Library: 1, Kind: "episode", IdentificationState: "unidentified", AddedAt: 2,
		})
		librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: id, TitleID: id, CreatedAt: 2})
		librarytest.File(t, harness.database, librarytest.FileSpec{
			ID: id, LibraryID: 1, NodeID: 1, Path: fmt.Sprintf("/m/BB/Brass Arena - 1x%d.mkv", index+2),
			SizeBytes: 10, PayloadHash: []byte{byte(index + 3)}, Filing: "listed", VersionID: id,
		})
		statements = append(statements, fmt.Sprintf(
			`INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order)
			 VALUES (1, %d, 0, NULL, %d)`, id, 20000+index))
	}
	storetest.ExecRaw(t, harness.databasePath, statements...)
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	if !strings.Contains(got.body, `"seriesReadWrongly":[{"seriesId":20,"name":"Brass Arena","unplacedCount":5}]`) {
		t.Errorf("GET /needs-attention = %d %s; want Brass Arena's five unplaced episodes told", got.status, got.body)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// An admin's report of a work filed wrongly is a Markdown file on the server's disk, naming each of
// its files by its path in the library, what the path was read as and how it was filed, and the
// System page reads it back; nothing leaves the server, and no one else may ask (D306 §3).
func TestFilingReport_IsMarkdownOnTheServersDiskThatTheAdminReadsBack(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/m')",
		"UPDATE file SET path = '/m/Brass Arena/Brass Arena - S01E01 - Pilot.mkv' WHERE id = 2")
	made := harness.do(t, http.MethodPost, "/items/20/filing-report", admin, nil)
	var report api.FilingReportBody
	if err := json.Unmarshal([]byte(made.body), &report); err != nil || made.status != http.StatusCreated {
		t.Fatalf("POST /items/20/filing-report = %d %s", made.status, made.body)
	}
	for _, want := range []string{
		"# Filed wrongly: Brass Arena", "Orpheum test", "| `Brass Arena/Brass Arena - S01E01 - Pilot.mkv` |",
		`an episode of "Brass Arena", S01E01`, `episode "Pilot"`, "## What it should have been",
	} {
		if !strings.Contains(report.Markdown, want) {
			t.Errorf("the report says:\n%s\nwant %q in it", report.Markdown, want)
		}
	}
	if strings.Contains(report.Markdown, "/m/") {
		t.Errorf("the report names the library's own folder:\n%s", report.Markdown)
	}
	onDisk, err := os.ReadFile(report.Path)
	if err != nil || string(onDisk) != report.Markdown {
		t.Errorf("on the server's disk at %s: %q (%v); want the report", report.Path, onDisk, err)
	}
	listed := harness.do(t, http.MethodGet, "/filing-reports", admin, nil)
	if !strings.Contains(listed.body, `"name":"`+report.Name+`"`) {
		t.Errorf("GET /filing-reports = %d %s; want the report listed", listed.status, listed.body)
	}
	read := harness.do(t, http.MethodGet, "/filing-reports/"+report.Name, admin, nil)
	if !strings.Contains(read.body, "Filed wrongly: Brass Arena") {
		t.Errorf("GET /filing-reports/%s = %d %s", report.Name, read.status, read.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/filing-reports/..%2Fsecret.key", admin, nil),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodPost, "/items/99/filing-report", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
	if _, err = harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	person := harness.signIn(t, "partner", "partner-password")
	if got := harness.do(t, http.MethodPost, "/items/20/filing-report", person,
		nil); got.status != http.StatusForbidden {
		t.Errorf("a report asked for by a person who is not an admin = %d, want 403", got.status)
	}
}

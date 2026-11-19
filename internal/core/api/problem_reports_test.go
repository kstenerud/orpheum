// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/problemreport"
)

// problemReportWords are the reporting person's words in the API's tests.
const problemReportWords = "it stalls at the start"

// signInPartner makes a person who is not an admin, "partner", and signs them in.
func signInPartner(t *testing.T, harness testHarness) string {
	t.Helper()
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	return harness.signIn(t, "partner", "partner-password")
}

// makeProblemReport is a report by token of problemReportWords, its id.
func makeProblemReport(t *testing.T, harness testHarness, token string) int64 {
	t.Helper()
	report := map[string]any{
		"words": problemReportWords, "page": "/watch/12",
		"device":   []map[string]any{{"name": "Screen", "value": "1920×1080"}},
		"playback": map[string]any{"itemId": 12, "positionMs": 90000, "state": "stalled", "quality": "1080p"},
	}
	header := http.Header{"User-Agent": {"Firefox/158.0"}}
	got := harness.doWithHeader(t, http.MethodPost, "/problem-reports", token, header, report)
	var made struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(got.body), &made); err != nil || got.status != http.StatusCreated || made.ID == 0 {
		t.Fatalf("POST /problem-reports = %d %s; want the report made", got.status, got.body)
	}
	return made.ID
}

// A person signed in reports a problem; the admin alone lists it and downloads it, in the form they
// name each time, the shareable one without the person's name (D524 §2, §4).
func TestProblemReports_APersonReportsAndTheAdminAloneListsAndDownloadsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	person := signInPartner(t, harness)
	reportID := makeProblemReport(t, harness, person)
	path := "/problem-reports/" + strconv.FormatInt(reportID, 10)

	got := harness.do(t, http.MethodGet, "/problem-reports", admin, nil)
	var listed api.ProblemReportsOutput
	if err := json.Unmarshal([]byte(got.body), &listed.Body); err != nil || got.status != http.StatusOK ||
		len(listed.Body.Reports) != 1 || listed.Body.Reports[0].UserName != "partner" ||
		listed.Body.Reports[0].Words != problemReportWords {
		t.Errorf("GET /problem-reports = %d %s; want partner's report", got.status, got.body)
	}
	for _, refused := range []string{"/problem-reports", path + "?form=full"} {
		if got := harness.do(t, http.MethodGet, refused, person, nil); got.status != http.StatusForbidden {
			t.Errorf("GET %s by a person who is not an admin = %d, want 403", refused, got.status)
		}
	}
	if got := harness.do(t, http.MethodGet, path, admin, nil); got.status != http.StatusUnprocessableEntity {
		t.Errorf("GET %s naming no form = %d %s, want 422: there is no default", path, got.status, got.body)
	}
	full := downloadProblemReport(t, harness, admin, path, problemreport.FormFull)
	for _, want := range []string{"partner", problemReportWords, "1920×1080", "Firefox/158.0", "stalled", "1080p"} {
		if !strings.Contains(full, want) {
			t.Errorf("the full report does not say %q:\n%s", want, full)
		}
	}
	if shareable := downloadProblemReport(t, harness, admin, path, problemreport.FormShareable); strings.Contains(
		shareable, "partner") || !strings.Contains(shareable, problemReportWords) {
		t.Errorf("the shareable report names its person, or lost their words:\n%s", shareable)
	}
}

// downloadProblemReport is the report at path in form, whole.
func downloadProblemReport(t *testing.T, harness testHarness, token, path string, form problemreport.Form) string {
	t.Helper()
	got := harness.do(t, http.MethodGet, path+"?form="+string(form), token, nil)
	var downloaded api.DownloadProblemReportOutput
	if err := json.Unmarshal([]byte(got.body), &downloaded.Body); err != nil || got.status != http.StatusOK ||
		downloaded.Body.Summary == "" {
		t.Fatalf("GET %s?form=%s = %d %s; want the report and its summary", path, form, got.status, got.body)
	}
	return downloaded.Body.Whole
}

// Only a person signed in reports, and only so many an hour; the admin deletes a report, which is
// then gone (D524 §2, §4).
func TestProblemReports_ARefusedReportAndADeletedOne(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	report := map[string]any{"words": problemReportWords, "page": "/", "device": []map[string]any{}}
	if got := harness.do(t, http.MethodPost, "/problem-reports", "", report); got.status != http.StatusUnauthorized {
		t.Errorf("a report from a page signed in to nothing = %d, want 401", got.status)
	}
	reportID := makeProblemReport(t, harness, admin)
	for range problemreport.PerPersonPerHour - 1 {
		makeProblemReport(t, harness, admin)
	}
	got := harness.do(t, http.MethodPost, "/problem-reports", admin, report)
	if got.status != http.StatusTooManyRequests {
		t.Errorf("a report past the hour's = %d %s, want 429", got.status, got.body)
	}
	path := "/problem-reports/" + strconv.FormatInt(reportID, 10)
	if got := harness.do(t, http.MethodDelete, path, admin, nil); got.status != http.StatusNoContent {
		t.Errorf("DELETE %s = %d %s, want 204", path, got.status, got.body)
	}
	if got := harness.do(t, http.MethodDelete, path, admin, nil); got.status != http.StatusNotFound {
		t.Errorf("DELETE %s again = %d, want 404", path, got.status)
	}
	if got := harness.do(t, http.MethodGet, path+"?form=full", admin, nil); got.status != http.StatusNotFound {
		t.Errorf("GET %s once deleted = %d, want 404", path, got.status)
	}
}

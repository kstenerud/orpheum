// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/core/unexpected"
)

// TestUnknownFailures_AnAdminReadsTheAlertAndEveryFailureInFullAndNoOneElseDoes is D509's API: the
// alert past the setting, and the report of every failure with what it said, to an admin
// alone.
func TestUnknownFailures_AnAdminReadsTheAlertAndEveryFailureInFullAndNoOneElseDoes(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	settingstest.Set(t, harness.settings, settings.UnknownFailuresAlertPerHour, "1")
	for item := range 2 {
		err := harness.unknownFailures.Record(t.Context(), unexpected.Failure{
			Kind: "fetch_metadata", Subject: "item " + strconv.Itoa(item+1), Error: "not JSON", Output: "<html>",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	got := harness.do(t, http.MethodGet, "/unknown-failures/alerts", admin, nil)
	var alerts api.UnknownFailureAlertsBody
	want := api.UnknownFailureAlertBody{Kind: "fetch_metadata", LastHour: 2}
	if err := json.Unmarshal([]byte(got.body), &alerts); err != nil || got.status != http.StatusOK ||
		len(alerts.Alerts) != 1 || alerts.Alerts[0] != want {
		t.Errorf("GET /unknown-failures/alerts = %d %s; want fetch_metadata's, 2 in the hour", got.status, got.body)
	}
	got = harness.do(t, http.MethodGet, "/unknown-failures", admin, nil)
	var report api.UnknownFailuresBody
	if err := json.Unmarshal([]byte(got.body), &report); err != nil || got.status != http.StatusOK ||
		!isBothInFull(report) {
		t.Errorf("GET /unknown-failures = %d %s; want both failures in full, the kind raised", got.status, got.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	person := harness.signIn(t, "partner", "partner-password")
	for _, path := range []string{"/unknown-failures/alerts", "/unknown-failures"} {
		if got := harness.do(t, http.MethodGet, path, person, nil); got.status != http.StatusForbidden {
			t.Errorf("GET %s by a person who is not an admin = %d, want 403", path, got.status)
		}
	}
}

// isBothInFull is whether report is the test's two failures of fetch_metadata, raised, in full.
func isBothInFull(report api.UnknownFailuresBody) bool {
	if report.Version != "test" || len(report.Kinds) != 1 {
		return false
	}
	kind := report.Kinds[0]
	return kind.SinceStart == 2 && kind.IsRaised && len(kind.Failures) == 2 &&
		kind.Failures[0].Error == "not JSON" && kind.Failures[0].Output == "<html>"
}

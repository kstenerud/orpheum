// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/reachability"
)

// Only an admin checks the server's reachability, and a server with no public name or relay says
// it checked neither; the echo answers a nonce no check made as nothing (D243 §7).
func TestReachability_AnAdminChecksItAndTheEchoAnswersOnlyItsOwnNonce(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	wantError(t, harness.do(t, http.MethodPost, "/system/reachability", partner, nil), http.StatusForbidden,
		CodeForbidden)
	checked := harness.do(t, http.MethodPost, "/system/reachability", admin, nil)
	var report ReachabilityOutput
	if err := json.Unmarshal([]byte(checked.body), &report.Body); err != nil || checked.status != http.StatusOK {
		t.Fatalf("POST /system/reachability = %d %s", checked.status, checked.body)
	}
	for hop, result := range map[string]ReachabilityResult{
		"https": report.Body.HTTPS, "relayUdp": report.Body.RelayUDP, "relayTcp": report.Body.RelayTCP,
	} {
		if result.Outcome != reachability.NotChecked {
			t.Errorf("%s with no name and no relay = %+v; want not checked", hop, result)
		}
	}
	wantError(t, harness.do(t, http.MethodGet, "/reachability/made-up", "", nil), http.StatusNotFound, CodeNotFound)
}

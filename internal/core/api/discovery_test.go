// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/discoveryproto"
)

// enrollingDiscovery answers a challenge and an enrolment with a claim code, as a discovery does;
// internal/core/discovery's tests check the signatures and the rest of the protocol.
func enrollingDiscovery(t *testing.T) *httptest.Server {
	t.Helper()
	fake := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var answer any
		switch request.URL.Path {
		case discoveryproto.PathChallenge:
			answer = discoveryproto.ChallengeResponse{Nonce: "a-nonce"}
		case discoveryproto.PathEnrol:
			answer = discoveryproto.EnrolResponse{CoreID: "k7xq2", ClaimCode: "WXYZ-2345", ClaimExpiresAt: 1}
		default:
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(answer) //nolint:errcheck // the core reads what arrived
	}))
	t.Cleanup(fake.Close)
	return fake
}

// Only an admin connects the core to a discovery; an address that is not a discovery's is refused
// naming the field, the claim code comes back to show the owner, a second connect is a conflict,
// and leaving forgets it (D273).
func TestDiscovery_AnAdminConnectsSeesTheClaimCodeAndLeaves(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	wantError(t, harness.do(t, http.MethodGet, "/system/discovery", partner, nil), http.StatusForbidden, CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, "/system/discovery", admin, map[string]any{
		"baseUrl": "http://discovery.example.com/core/v1",
	}), http.StatusUnprocessableEntity, CodeInvalidRequest)
	fake := enrollingDiscovery(t)
	connected := harness.do(t, http.MethodPost, "/system/discovery", admin, map[string]any{"baseUrl": fake.URL})
	var status DiscoveryStatusBody
	if err := json.Unmarshal([]byte(connected.body), &status); err != nil || connected.status != http.StatusOK {
		t.Fatalf("POST /system/discovery = %d %s", connected.status, connected.body)
	}
	if status.BaseURL != fake.URL || status.State != "pending" || status.ClaimCode != "WXYZ-2345" {
		t.Errorf("connected = %+v; want pending at %s with its claim code", status, fake.URL)
	}
	if len(status.WebRTCPaths) != 6 || status.WebRTCPaths[0].Path != "direct_ipv6" || status.WebRTCPaths[0].Count != 0 {
		t.Errorf("the WebRTC paths = %+v; want every path listed, none taken yet", status.WebRTCPaths)
	}
	if status.PortMapping.IsWanted || status.PortMapping.Mappings == nil {
		t.Errorf("a pending core's port mapping = %+v; want none asked for, said as an empty list",
			status.PortMapping)
	}
	wantError(t, harness.do(t, http.MethodPost, "/system/discovery", admin, map[string]any{"baseUrl": fake.URL}),
		http.StatusConflict, CodeConflict)
	if left := harness.do(t, http.MethodDelete, "/system/discovery", admin, nil); left.status != http.StatusNoContent {
		t.Fatalf("DELETE /system/discovery = %d %s", left.status, left.body)
	}
	got := harness.do(t, http.MethodGet, "/system/discovery", admin, nil)
	if err := json.Unmarshal([]byte(got.body), &status); err != nil || status.BaseURL != "" {
		t.Errorf("after leaving = %s; want no discovery", got.body)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
)

// enrollingDiscovery answers a challenge and an enrolment with a claim code, a session, the core's
// status, and a claim by the key abcd-efgh-jkmn-pqrs, as a discovery does; internal/core/discovery's
// tests check the signatures and the rest of the protocol.
func enrollingDiscovery(t *testing.T) *httptest.Server {
	t.Helper()
	var claimed atomic.Bool
	fake := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var answer any
		switch request.URL.Path {
		case discoveryproto.PathChallenge:
			answer = discoveryproto.ChallengeResponse{Nonce: "a-nonce"}
		case discoveryproto.PathEnrol:
			answer = discoveryproto.EnrolResponse{CoreID: "k7xq2", ClaimCode: "WXYZ-2345", ClaimExpiresAt: 1}
		case discoveryproto.PathSession:
			answer = discoveryproto.SessionResponse{Token: "a-token", ExpiresIn: 3600}
		case discoveryproto.PathStatus:
			answer = discoveryproto.StatusResponse{State: discoveryproto.StatePending}
			if claimed.Load() {
				answer = discoveryproto.StatusResponse{
					State: discoveryproto.StateClaimed, Account: "karl@example.com", Realm: "karlnas",
				}
			}
		case discoveryproto.PathClaim:
			var asked discoveryproto.ClaimRequest
			_ = json.NewDecoder(request.Body).Decode(&asked) //nolint:errcheck // a wrong body is a wrong key
			if asked.Key != "abcd-efgh-jkmn-pqrs" {
				writer.WriteHeader(http.StatusForbidden)
				answer = discoveryproto.Refusal{Code: discoveryproto.RefusalWrongKey, Message: "no such key"}
				break
			}
			claimed.Store(true)
			answer = discoveryproto.ClaimResponse{Account: "karl@example.com"}
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

// fingerprintText is a fingerprint as a person reads it: 13 groups of four safe32 characters.
var fingerprintText = regexp.MustCompile(`^([0-9a-hjkmnp-tv-z]{4}-){12}[0-9a-hjkmnp-tv-z]{4}$`)

// wantPendingWithItsCode fails the test unless a connected core is pending at baseURL with its
// claim code and its identity key's fingerprint.
func wantPendingWithItsCode(t *testing.T, status api.DiscoveryStatusBody, baseURL string) {
	t.Helper()
	if status.BaseURL != baseURL || status.State != "pending" || status.ClaimCode != "WXYZ-2345" ||
		!fingerprintText.MatchString(status.Fingerprint) {
		t.Errorf("connected = %+v; want pending at %s with its claim code and the key's fingerprint, "+
			"in safe32 groups for a person to read", status, baseURL)
	}
}

// Only an admin connects the core to a discovery; an address that is not a discovery's is refused
// naming the field, the claim code comes back to show the owner, a second connect is a conflict,
// and leaving forgets it, but not its people's realm keys (D273, D299).
func TestDiscovery_AnAdminConnectsSeesTheClaimCodeAndLeaves(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	// Signed in under a realm, a person has a realm key, which leaving keeps (D299).
	harness.authService.SetRealm("karlnas")
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	partnerPerson := proof.RealmPerson("karlnas", "partner")
	if _, err := harness.authService.RealmKeyOfPerson(t.Context(), partnerPerson); err != nil {
		t.Fatalf("the partner's realm key after signing in: %v", err)
	}
	wantError(t, harness.do(t, http.MethodGet, "/system/discovery", partner, nil), http.StatusForbidden,
		api.CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, "/system/discovery", admin, map[string]any{
		"baseUrl": "http://discovery.example.com/core/v1",
	}), http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	fake := enrollingDiscovery(t)
	connected := harness.do(t, http.MethodPost, "/system/discovery", admin, map[string]any{"baseUrl": fake.URL})
	var status api.DiscoveryStatusBody
	if err := json.Unmarshal([]byte(connected.body), &status); err != nil || connected.status != http.StatusOK {
		t.Fatalf("POST /system/discovery = %d %s", connected.status, connected.body)
	}
	wantPendingWithItsCode(t, status, fake.URL)
	if len(status.WebRTCPaths) != 6 || status.WebRTCPaths[0].Path != "direct_ipv6" || status.WebRTCPaths[0].Count != 0 {
		t.Errorf("the WebRTC paths = %+v; want every path listed, none taken yet", status.WebRTCPaths)
	}
	if status.PortMapping.IsWanted || status.PortMapping.Mappings == nil {
		t.Errorf("a pending core's port mapping = %+v; want none asked for, said as an empty list",
			status.PortMapping)
	}
	wantError(t, harness.do(t, http.MethodPost, "/system/discovery", admin, map[string]any{"baseUrl": fake.URL}),
		http.StatusConflict, api.CodeConflict)
	if left := harness.do(t, http.MethodDelete, "/system/discovery", admin, nil); left.status != http.StatusNoContent {
		t.Fatalf("DELETE /system/discovery = %d %s", left.status, left.body)
	}
	got := harness.do(t, http.MethodGet, "/system/discovery", admin, nil)
	if err := json.Unmarshal([]byte(got.body), &status); err != nil || status.BaseURL != "" {
		t.Errorf("after leaving = %s; want no discovery", got.body)
	}
	// Come back under the same master realm, the partner is found with no sign-in at home first.
	harness.authService.SetRealm("karlnas")
	if _, err := harness.authService.RealmKeyOfPerson(t.Context(), partnerPerson); err != nil {
		t.Errorf("after leaving and coming back the partner's realm key = %v; want kept", err)
	}
	if harness.signIn(t, "partner", "partner-password") == "" {
		t.Error("after leaving the partner's password no longer signs in")
	}
}

// An admin enters the account's key, and the core then says it is claimed and names the account; a
// wrong key is refused naming the field, and another once claimed is a conflict (D310 §5, §8).
func TestDiscovery_AnAdminEntersTheAccountsKeyAndTheCoreNamesTheAccount(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	fake := enrollingDiscovery(t)
	connected := harness.do(t, http.MethodPost, "/system/discovery", admin, map[string]any{"baseUrl": fake.URL})
	if connected.status != http.StatusOK {
		t.Fatalf("connecting = %d %s", connected.status, connected.body)
	}
	keyed := func(key, password string) response {
		return harness.do(t, http.MethodPost, "/system/discovery/claim", admin,
			map[string]any{"key": key, "password": password})
	}
	wantError(t, keyed("wxyz-wxyz-wxyz-wxyz", "owner-password"), http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	// A mistyped password is refused before the key is sent, so it spends no key.
	wrong := keyed("abcd-efgh-jkmn-pqrs", "not-the-password")
	wantError(t, wrong, http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	if !strings.Contains(wrong.body, "body.password") {
		t.Errorf("a wrong password = %s; want it refused naming the field", wrong.body)
	}
	claimed := keyed("abcd-efgh-jkmn-pqrs", "owner-password")
	var status api.DiscoveryStatusBody
	if err := json.Unmarshal([]byte(claimed.body), &status); err != nil || claimed.status != http.StatusOK ||
		status.State != "claimed" || status.Account != "karl@example.com" {
		t.Fatalf("the key = %d %s; want claimed, naming karl's account", claimed.status, claimed.body)
	}
	// The admin set up the core before it had a realm; the claim's password makes their key for the
	// account's, so they sign in from away at once (D297 §3).
	if _, err := harness.authService.RealmKeyOfPerson(t.Context(), proof.RealmPerson("karlnas", "owner")); err != nil {
		t.Errorf("the admin's realm key after the claim: %v; want one for karlnas", err)
	}
	wantError(t, keyed("abcd-efgh-jkmn-pqrs", "owner-password"), http.StatusConflict, api.CodeConflict)
}

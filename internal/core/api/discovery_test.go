// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// enrollingDiscovery serves its root chain and its signed address document at its issuer, its own
// URL, and answers a challenge and an enrolment with a claim code, a session, the core's status, and
// a claim by the key abcd-efgh-jkmn-pqrs, as a discovery does; it returns its identity too. The core
// discovery package's tests check the signatures and the rest of the protocol.
func enrollingDiscovery(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	var claimed atomic.Bool
	chain, everyday, _ := trusttest.NewDiscovery(t)
	files := map[string]trust.Signed{}
	fake := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		name, isSignature := strings.CutSuffix(request.URL.Path, addressdoc.SignatureEnd)
		if files[name].Payload != "" {
			text := files[name].Payload
			if isSignature {
				text = files[name].Signature
			}
			_, _ = writer.Write([]byte(text)) //nolint:errcheck // the core reads what arrived
			return
		}
		var answer any
		switch request.URL.Path {
		case discoveryproto.PathChallenge:
			answer = discoveryproto.ChallengeResponse{Nonce: "a-nonce"}
		case discoveryproto.PathEnrol:
			answer = discoveryproto.EnrolResponse{CoreID: "k7xq2", ClaimCode: "WXYZ-2345", ClaimExpiresAt: 1}
		case discoveryproto.PathSession:
			answer = discoveryproto.SessionResponse{Token: "a-token", ExpiresInSeconds: 3600}
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
		case discoveryproto.PathLinkToken:
			var asked discoveryproto.LinkTokenRequest
			_ = json.NewDecoder(request.Body).Decode(&asked) //nolint:errcheck // a wrong body shows in the page
			page := "https://account.example/link?" +
				url.Values{"back": {asked.ReturnTo}, "name": {asked.Name}}.Encode()
			answer = discoveryproto.LinkTokenResponse{Token: "t", ExpiresAt: 1, LinkURL: page}
		default:
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(answer) //nolint:errcheck // the core reads what arrived
	}))
	t.Cleanup(fake.Close)
	payload, err := (addressdoc.Document{
		ID: chain.ID(), RootVersion: 1, Sequence: 1, Issued: trusttest.Issued(),
		Expires: trusttest.Issued().Add(365 * 24 * time.Hour), Name: "Discovery", Issuer: fake.URL, CoreAPI: fake.URL,
		ClientAPI: fake.URL + "/client", LoaderOrigin: "https://loader.example",
		ClientOrigins: "https://{core}.client.example",
	}).Payload()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range trust.ChainFiles(addressdoc.RootPathPrefix, chain.Statements) {
		files[file.Path] = file.Signed
	}
	files[addressdoc.DocumentPath] = trusttest.Sign(t, trust.NamespaceDiscoveryAddress, payload, everyday)
	return fake, chain.ID()
}

// fingerprintText is a fingerprint as a person reads it: 13 groups of four safe32 characters.
var fingerprintText = regexp.MustCompile(`^([0-9a-hjkmnp-tv-z]{4}-){12}[0-9a-hjkmnp-tv-z]{4}$`)

// wantPendingWithItsCode fails the test unless a connected core is pending at the discovery issuer
// names, with its claim code, its identity key's fingerprint, the loader its document names and the
// one root key it trusts.
func wantPendingWithItsCode(t *testing.T, status api.DiscoveryStatusBody, issuer string) {
	t.Helper()
	if status.Issuer != issuer || status.CoreAPI != issuer || status.State != "pending" || status.Name != "Discovery" ||
		status.ClaimCode != "WXYZ-2345" || !fingerprintText.MatchString(status.Fingerprint) ||
		status.LoaderOrigin != "https://loader.example" || len(status.RootKeys) != 2 {
		t.Errorf("connected = %+v; want pending at %s with its claim code and the key's fingerprint, "+
			"in safe32 groups for a person to read, its loader and its two root keys", status, issuer)
	}
}

// connecting is the body that connects the core to the discovery at issuer, of identity, with the
// owner's password.
func connecting(issuer, identity string) map[string]any {
	return map[string]any{"issuer": issuer, "identity": identity, "password": "owner-password"}
}

// Only an admin connects the core to a discovery, with their password; a name that is not an issuer
// is refused naming the field, as is a wrong password, the claim code comes back to show the owner,
// a second connect is a conflict, and leaving forgets it, but not its people's realm keys (D273,
// D299, D391 §3).
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
	wantError(t, harness.do(t, http.MethodPost, "/system/discovery", admin,
		connecting("http://discovery.example.com", "")), http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	fake, identity := enrollingDiscovery(t)
	connected := harness.do(t, http.MethodPost, "/system/discovery", admin, connecting(fake.URL, identity))
	var status api.DiscoveryStatusBody
	if err := json.Unmarshal([]byte(connected.body), &status); err != nil || connected.status != http.StatusOK {
		t.Fatalf("POST /system/discovery = %d %s", connected.status, connected.body)
	}
	wantPendingWithItsCode(t, status, fake.URL)
	if len(status.WebRTCPaths) != 7 || status.WebRTCPaths[0].Path != "directIpv6" || status.WebRTCPaths[0].Count != 0 {
		t.Errorf("the WebRTC paths = %+v; want every path listed, none taken yet", status.WebRTCPaths)
	}
	if status.PortMapping.IsWanted || status.PortMapping.Mappings == nil {
		t.Errorf("a pending core's port mapping = %+v; want none asked for, said as an empty list",
			status.PortMapping)
	}
	wantError(t, harness.do(t, http.MethodPost, "/system/discovery", admin, connecting(fake.URL, identity)),
		http.StatusConflict, api.CodeConflict)
	if left := harness.do(t, http.MethodDelete, "/system/discovery", admin, nil); left.status != http.StatusNoContent {
		t.Fatalf("DELETE /system/discovery = %d %s", left.status, left.body)
	}
	got := harness.do(t, http.MethodGet, "/system/discovery", admin, nil)
	if err := json.Unmarshal([]byte(got.body), &status); err != nil || status.Issuer != "" {
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

// Only an admin has a core refused as a copy ask its discovery again (D437 §3); a core that does not
// stand aside is unchanged by it, and says it asks.
func TestDiscovery_OnlyAnAdminHasTheCoreAskAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	wantError(t, harness.do(t, http.MethodPost, "/system/discovery/ask-again", partner, nil), http.StatusForbidden,
		api.CodeForbidden)
	if asked := harness.do(t, http.MethodPost, "/system/discovery/ask-again", admin, nil); asked.status !=
		http.StatusNoContent {
		t.Fatalf("POST /system/discovery/ask-again = %d %s", asked.status, asked.body)
	}
	var status api.DiscoveryStatusBody
	got := harness.do(t, http.MethodGet, "/system/discovery", admin, nil)
	if err := json.Unmarshal([]byte(got.body), &status); err != nil || status.StoodAsideAt != 0 ||
		status.CopyRefusedFrom != "" || status.CopyRefusedAt != 0 {
		t.Errorf("after asking again = %s; want a core that asks, and no copy refused", got.body)
	}
}

// Connecting asks the admin's password, as a claim does: which discovery a core trusts is as weighty
// (D391 §3). A wrong one is refused naming the field, and connects nothing.
func TestDiscovery_ConnectingAsksTheAdminsPassword(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	fake, identity := enrollingDiscovery(t)
	wrongPassword := harness.do(t, http.MethodPost, "/system/discovery", admin,
		map[string]any{"issuer": fake.URL, "identity": identity, "password": "not-the-password"})
	wantError(t, wrongPassword, http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	if !strings.Contains(wrongPassword.body, "body.password") {
		t.Errorf("a wrong password = %s; want it refused naming the field", wrongPassword.body)
	}
	var status api.DiscoveryStatusBody
	got := harness.do(t, http.MethodGet, "/system/discovery", admin, nil)
	if err := json.Unmarshal([]byte(got.body), &status); err != nil || status.Issuer != "" {
		t.Errorf("after a wrong password = %s; want no discovery", got.body)
	}
}

// An admin enters the account's key, and the core then says it is claimed and names the account; a
// wrong key is refused naming the field, and another once claimed is a conflict (D310 §5, §8).
func TestDiscovery_AnAdminEntersTheAccountsKeyAndTheCoreNamesTheAccount(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	fake, identity := enrollingDiscovery(t)
	connected := harness.do(t, http.MethodPost, "/system/discovery", admin, connecting(fake.URL, identity))
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

// An admin asks what a discovery shows before enrolling: its identity and name, to compare with what
// its operator shows; connecting with no identity, or another, is refused naming the field, and
// enrols nothing (the root key audit's §3.8; D393 §4).
func TestDiscovery_AnAdminComparesTheDiscoverysIdentityBeforeEnrolling(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	fake, identity := enrollingDiscovery(t)
	shown := harness.do(t, http.MethodPost, "/system/discovery/identify", admin, map[string]any{"issuer": fake.URL})
	var identified struct {
		Identity string `json:"identity"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal([]byte(shown.body), &identified); err != nil || shown.status != http.StatusOK ||
		identified.Identity != identity || identified.Name != "Discovery" {
		t.Fatalf("identify = %d %s; want the identity %s and the name", shown.status, shown.body, identity)
	}
	for _, given := range []string{"", "od1-another"} {
		refused := harness.do(t, http.MethodPost, "/system/discovery", admin, connecting(fake.URL, given))
		wantError(t, refused, http.StatusUnprocessableEntity, api.CodeInvalidRequest)
		if !strings.Contains(refused.body, "body.identity") {
			t.Errorf("identity %q = %s; want it refused naming the field", given, refused.body)
		}
	}
	connected := harness.do(t, http.MethodPost, "/system/discovery", admin, connecting(fake.URL, identity))
	if connected.status != http.StatusOK {
		t.Errorf("the identity shown = %d %s; want enrolled", connected.status, connected.body)
	}
}

// An admin's Connect enrols the core with the discovery named when it has none, and answers the
// account page with the token the discovery made, which brings the browser back to this server's
// System page; it asks the admin's password only when the core holds no away-from-home key for them,
// and makes the key from it; a page that sent no origin is refused naming it, and only an admin
// connects (D527 §2.1, §2.2).
func TestDiscovery_ConnectEnrolsWhenNeededAndAnswersTheAccountPageThatLinksIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	fake, identity := enrollingDiscovery(t)
	from := http.Header{"Origin": {"https://home.example:8443"}}
	body := map[string]any{"issuer": fake.URL, "identity": identity}
	wantError(t, harness.do(t, http.MethodPost, "/system/discovery/link", admin, body),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	linked := harness.doWithHeader(t, http.MethodPost, "/system/discovery/link", admin, from, body)
	var page api.LinkDiscoveryOutput
	if err := json.Unmarshal([]byte(linked.body), &page.Body); err != nil || linked.status != http.StatusOK {
		t.Fatalf("POST /system/discovery/link = %d %s", linked.status, linked.body)
	}
	want := "https://account.example/link?back=https%3A%2F%2Fhome.example%3A8443%2Fsystem&name=home.example%3A8443"
	if page.Body.LinkURL != want {
		t.Errorf("the account page is %q; want %q, back to this page", page.Body.LinkURL, want)
	}
	var status api.DiscoveryStatusBody
	got := harness.do(t, http.MethodGet, "/system/discovery", admin, nil)
	if err := json.Unmarshal([]byte(got.body), &status); err != nil || status.Issuer != fake.URL {
		t.Errorf("after Connect the core stands %s; want enrolled at the discovery named", got.body)
	}
	// An admin with no key yet, from before the core made them, gives their password once.
	ownerID := storetest.QueryRaw(t, harness.databasePath, "SELECT id FROM user WHERE name = 'owner'")[0]
	storetest.ExecRaw(t, harness.databasePath, fmt.Sprintf("DELETE FROM person_key WHERE user_id = %s", ownerID))
	for _, password := range []string{"", "a guess"} {
		refused := harness.doWithHeader(t, http.MethodPost, "/system/discovery/link", admin, from,
			map[string]any{"password": password})
		wantError(t, refused, http.StatusUnprocessableEntity, api.CodeInvalidRequest)
		if !strings.Contains(refused.body, "body.password") {
			t.Errorf("Connect with the password %q = %s; want it refused naming the password", password, refused.body)
		}
	}
	again := harness.doWithHeader(t, http.MethodPost, "/system/discovery/link", admin, from,
		map[string]any{"password": "owner-password"})
	if again.status != http.StatusOK {
		t.Fatalf("Connect with the admin's password = %d %s", again.status, again.body)
	}
	if keys := storetest.QueryRaw(t, harness.databasePath,
		"SELECT count(*) FROM person_key WHERE user_id = "+ownerID); keys[0] != "1" {
		t.Errorf("after Connect with the password the admin has %v keys; want 1", keys)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	wantError(t, harness.doWithHeader(t, http.MethodPost, "/system/discovery/link", partner, from, body),
		http.StatusForbidden, api.CodeForbidden)
}

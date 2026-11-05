// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// readSignIn reads a sign-in's answer: the token, and whose it is.
func readSignIn(t *testing.T, answer response) (token, name string) {
	t.Helper()
	var signedIn struct {
		Token string `json:"token"`
		User  struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(answer.body), &signedIn); err != nil || signedIn.Token == "" {
		t.Fatalf("a sign-in = %d %s", answer.status, answer.body)
	}
	return signedIn.Token, signedIn.User.Name
}

func TestAccounts_TheAdminMakesAPersonWhoRegistersByLinkAndAddsATelevisionByCode(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	wantError(t, harness.do(t, http.MethodPost, "/invitations", admin, map[string]string{"role": "user"}),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	invitation := harness.invite(t, admin, "user", "the partner")
	if pending := harness.do(t, http.MethodGet, "/invitations", admin, nil); !strings.Contains(pending.body,
		`"role":"user","adminNote":"the partner"`) {
		t.Errorf("GET /invitations = %s, want the partner's", pending.body)
	}
	kind := harness.do(t, http.MethodPost, "/registration/link", "",
		map[string]string{"secret": invitation.Link.Secret})
	if !strings.Contains(kind.body, `"isInvitation":true`) {
		t.Errorf("POST /registration/link = %d %s, want an invitation", kind.status, kind.body)
	}

	registered := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "partner", "password": "partner-password",
		"device": testDevice,
	})
	partner, name := readSignIn(t, registered)
	if name != "partner" {
		t.Errorf("registering signed in as %q, want partner", name)
	}
	var made struct{ Account api.UserEntryBody }
	made.Account = harness.accountNamed(t, admin, "partner")
	if made.Account.AdminNote != "the partner" || !made.Account.HasPassword {
		t.Errorf("GET /users lists partner as %+v, want the invitation's note and a password", made.Account)
	}
	again := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "another", "password": "x", "device": testDevice,
	})
	wantError(t, again, http.StatusUnauthorized, api.CodeAuthRequired)
	wantError(t, harness.do(t, http.MethodPost, "/invitations", partner,
		map[string]string{"role": "user", "adminNote": "x"}), http.StatusForbidden, api.CodeForbidden)

	television := harness.activate(t, partner, map[string]any{"deviceName": "Living room TV"})
	if _, name := readSignIn(t, television); name != "partner" {
		t.Errorf("the television signed in as %q, want partner", name)
	}

	wantErrorSaying(t, harness.do(t, http.MethodPut, "/me/password", partner,
		map[string]string{"currentPassword": "wrong", "newPassword": "new"}), http.StatusForbidden, api.CodeForbidden,
		"the current password is wrong")
	if changed := harness.do(t, http.MethodPut, "/me/password", partner,
		map[string]string{"currentPassword": "partner-password", "newPassword": "new"}); changed.status !=
		http.StatusNoContent {
		t.Errorf("PUT /me/password = %d %s", changed.status, changed.body)
	}
	harness.signIn(t, "partner", "new")
}

func TestDevices_APersonListsRenamesAndRemovesTheirOwnAndAnAdminAnyones(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	second := harness.signIn(t, "owner", "owner-password")
	listed := harness.do(t, http.MethodGet, "/me/devices", admin, nil)
	var mine api.DevicesOutput
	if err := json.Unmarshal([]byte(listed.body), &mine.Body); err != nil || len(mine.Body.Devices) != 2 {
		t.Fatalf("GET /me/devices = %d %s", listed.status, listed.body)
	}
	var other api.DeviceBody
	for _, device := range mine.Body.Devices {
		if !device.IsThisDevice {
			other = device
		}
	}
	if other.ID == 0 || !other.IsSignedIn {
		t.Fatalf("devices = %+v, want this one and another, signed in", mine.Body.Devices)
	}
	if renamed := harness.do(t, http.MethodPut, "/me/devices/"+itoa(other.ID), admin,
		map[string]string{"name": "Laptop"}); renamed.status != http.StatusNoContent {
		t.Errorf("rename = %d %s", renamed.status, renamed.body)
	}
	if removed := harness.do(t, http.MethodDelete, "/me/devices/"+itoa(other.ID), admin, nil); removed.status !=
		http.StatusNoContent {
		t.Errorf("remove = %d %s", removed.status, removed.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/me", second, nil), http.StatusUnauthorized, api.CodeAuthRequired)
	wantError(t, harness.do(t, http.MethodDelete, "/me/devices/9999", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
	if byAdmin := harness.do(t, http.MethodGet, "/users/1/devices", admin, nil); !strings.Contains(byAdmin.body,
		`"isThisDevice":true`) {
		t.Errorf("GET /users/1/devices = %s", byAdmin.body)
	}
	if signedOut := harness.do(t, http.MethodPost, "/me/devices/sign-out-elsewhere", admin, nil); signedOut.status !=
		http.StatusNoContent {
		t.Errorf("sign out elsewhere = %d %s", signedOut.status, signedOut.body)
	}
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

func TestSession_ARemovedDeviceIsToldItWasSignedOutAndMeNamesTheDevice(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	other := harness.signIn(t, "owner", "owner-password")
	me := harness.do(t, http.MethodGet, "/me", other, nil)
	var body api.UserBody
	if err := json.Unmarshal([]byte(me.body), &body); err != nil || body.Device == nil ||
		body.Device.Name != "Firefox on Linux" {
		t.Fatalf("GET /me = %s, want the device it is on", me.body)
	}
	harness.do(t, http.MethodDelete, "/me/devices/"+itoa(body.Device.ID), admin, nil)
	signedOut := harness.do(t, http.MethodGet, "/items/1", other, nil)
	wantError(t, signedOut, http.StatusUnauthorized, api.CodeAuthRequired)
	if !strings.Contains(signedOut.body, `"reason":"`+api.ReasonSessionEnded+`"`) {
		t.Errorf("a removed device's request = %s, want the reason %s", signedOut.body, api.ReasonSessionEnded)
	}
	if never := harness.do(t, http.MethodGet, "/me", "", nil); !strings.Contains(never.body,
		`"reason":"`+api.ReasonNoSession+`"`) {
		t.Errorf("a request with no session = %s, want the reason %s", never.body, api.ReasonNoSession)
	}
}

// A client that signs in again naming the device it was before, as previousDeviceId, with the device
// secret its sign-in gave as the proof, is that device again (D221 §1; the third audit's A8), not a
// new one.
func TestSession_ASignInNamingThePreviousDeviceWithItsProofResumesIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	var issued struct {
		Token        string `json:"token"`
		DeviceSecret string `json:"deviceSecret"`
	}
	answered := harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "owner", "password": "owner-password", "device": testDevice,
	})
	if err := json.Unmarshal([]byte(answered.body), &issued); err != nil || issued.DeviceSecret == "" ||
		issued.DeviceSecret == issued.Token {
		t.Fatalf("a sign-in = %d %s; want a device secret apart from the token", answered.status, answered.body)
	}
	first := issued.Token
	before := harness.thisDeviceID(t, first)
	// Ended from another device, as sign out everywhere else does, the device stays listed (D320 §2).
	elsewhere := harness.signIn(t, "owner", "owner-password")
	if out := harness.do(t, http.MethodPost, "/me/devices/sign-out-elsewhere", elsewhere, nil); out.status !=
		http.StatusNoContent {
		t.Fatalf("POST /me/devices/sign-out-elsewhere = %d %s", out.status, out.body)
	}
	signedIn := harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "owner", "password": "owner-password",
		"device": map[string]any{
			"name": "Firefox on Linux", "platform": "web", "previousDeviceId": before,
			"previousDeviceProof": issued.DeviceSecret,
		},
	})
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(signedIn.body), &session); err != nil || session.Token == "" {
		t.Fatalf("sign in again: %d %s", signedIn.status, signedIn.body)
	}
	if after := harness.thisDeviceID(t, session.Token); after != before {
		t.Errorf("signing in again naming device %d with its proof is device %d, want %d", before, after, before)
	}
	// Naming the device without the proof makes a new one (the second audit's K1).
	unproven := harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "owner", "password": "owner-password",
		"device": map[string]any{"name": "Intruder", "platform": "web", "previousDeviceId": before},
	})
	if err := json.Unmarshal([]byte(unproven.body), &session); err != nil || session.Token == "" {
		t.Fatalf("sign in naming the device without proof: %d %s", unproven.status, unproven.body)
	}
	if after := harness.thisDeviceID(t, session.Token); after == before {
		t.Errorf("a sign-in naming device %d without its proof was put on it", before)
	}
}

func TestPeople_AnAdminRenamesAPersonAndSeesTheirPage(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	created := harness.do(t, http.MethodPost, "/users", admin,
		map[string]string{"name": "partner", "password": "partner-password", "role": "user", "adminNote": "Anna"})
	if created.status != http.StatusCreated ||
		!strings.Contains(created.body, `"adminNote":"Anna","hasPassword":true`) {
		t.Fatalf("POST /users, set up in full = %d %s", created.status, created.body)
	}
	if _, name := readSignIn(t, harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "partner", "password": "partner-password", "device": testDevice,
	})); name != "partner" {
		t.Errorf("an account set up in full signs in as %q, want partner", name)
	}
	noted := harness.do(t, http.MethodPut, "/users/2/admin-note", admin, map[string]string{"adminNote": "Anna B."})
	if noted.status != http.StatusNoContent {
		t.Errorf("PUT /users/2/admin-note = %d %s", noted.status, noted.body)
	}
	wantError(t, harness.do(t, http.MethodPut, "/users/2", admin, map[string]string{"name": "OWNER"}),
		http.StatusConflict, api.CodeConflict)
	if renamed := harness.do(t, http.MethodPut, "/users/2", admin, map[string]string{"name": "Sam"}); renamed.status !=
		http.StatusNoContent {
		t.Fatalf("rename = %d %s", renamed.status, renamed.body)
	}
	page := harness.do(t, http.MethodGet, "/users/2", admin, nil)
	if !strings.Contains(page.body, `"name":"Sam"`) || !strings.Contains(page.body, `"adminNote":"Anna B."`) ||
		!strings.Contains(page.body, `"watchedMs":0`) {
		t.Errorf("GET /users/2 = %d %s", page.status, page.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/users/99", admin, nil), http.StatusNotFound, api.CodeNotFound)
}

// activate activates a television by the code it asks for, entered by the session enteredBy with the
// rest of body, the check codes confirmed as matching, and returns the television's session.
func (harness testHarness) activate(t *testing.T, enteredBy string, body map[string]any) response {
	t.Helper()
	asked := harness.do(t, http.MethodPost, "/pairings", "", map[string]any{"platform": "web"})
	var pairing api.PairingOutput
	if err := json.Unmarshal([]byte(asked.body), &pairing.Body); err != nil || len(pairing.Body.Code) != 9 {
		t.Fatalf("POST /pairings = %d %s", asked.status, asked.body)
	}
	body["code"] = strings.ToUpper(pairing.Body.Code)
	entered := harness.do(t, http.MethodPost, "/pairings/code", enteredBy, body)
	var claim api.ActivationOutput
	if err := json.Unmarshal([]byte(entered.body), &claim.Body); err != nil || entered.status != http.StatusOK {
		t.Fatalf("POST /pairings/code = %d %s", entered.status, entered.body)
	}
	// The person is shown what asked for the code, and when (the device code audit's M3).
	if claim.Body.Platform != "web" || claim.Body.AskedSecondsAgo != 0 {
		t.Errorf("POST /pairings/code shows the platform %q asked %d s ago; want web, just now",
			claim.Body.Platform, claim.Body.AskedSecondsAgo)
	}
	secret := map[string]string{"secret": pairing.Body.Secret}
	if state := harness.do(t, http.MethodPost, "/pairings/state", "", secret); !strings.Contains(state.body,
		`"state":"checking","checkCode":"`+claim.Body.CheckCode+`"`) {
		t.Errorf("the television's pairing = %s; want the person's check code, %s", state.body, claim.Body.CheckCode)
	}
	if answered := harness.do(t, http.MethodPost, "/pairings/check", enteredBy,
		map[string]any{"code": pairing.Body.Code, "isMatch": true}); answered.status != http.StatusNoContent {
		t.Fatalf("POST /pairings/check = %d %s", answered.status, answered.body)
	}
	if state := harness.do(t, http.MethodPost, "/pairings/state", "", secret); !strings.Contains(state.body,
		`"state":"activated"`) {
		t.Errorf("the television's pairing once confirmed = %s", state.body)
	}
	session := harness.do(t, http.MethodPost, "/pairings/session", "", secret)
	if session.status != http.StatusCreated {
		t.Fatalf("POST /pairings/session = %d %s", session.status, session.body)
	}
	return session
}

// An admin activates a device for someone else, which no one else may; a code no device shows is
// not found, and a new device needs a name (D288 §2).
func TestPairing_AnAdminActivatesADeviceForSomeoneElseAndOnlyAnAdmin(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	made, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	forPartner := harness.activate(t, admin, map[string]any{"deviceName": "Bedroom TV", "userId": made})
	if _, name := readSignIn(t, forPartner); name != "partner" {
		t.Errorf("the admin's activation signed the television in as %q, want partner", name)
	}
	asked := harness.do(t, http.MethodPost, "/pairings", "", map[string]any{"platform": "web"})
	var pairing api.PairingOutput
	if err = json.Unmarshal([]byte(asked.body), &pairing.Body); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodPost, "/pairings/code", partner,
		map[string]any{"code": pairing.Body.Code, "deviceName": "TV", "userId": 1}),
		http.StatusForbidden, api.CodeForbidden)
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/pairings/code", partner,
		map[string]any{"code": pairing.Body.Code}), http.StatusUnprocessableEntity, "body.deviceName")
	wantError(t, harness.do(t, http.MethodPost, "/pairings/code", partner,
		map[string]any{"code": "zzzz-zzzz", "deviceName": "TV"}), http.StatusNotFound, api.CodeNotFound)
	// Never for another admin, and never from away, where the admin does not administer (the
	// Plex-chain review's T1).
	other, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "second", Password: "second-password", Role: auth.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodPost, "/pairings/code", admin,
		map[string]any{"code": pairing.Body.Code, "deviceName": "TV", "userId": other}),
		http.StatusForbidden, api.CodeForbidden)
	harness.server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")})
	wantError(t, harness.do(t, http.MethodPost, "/pairings/code", admin,
		map[string]any{"code": pairing.Body.Code, "deviceName": "TV", "userId": made}),
		http.StatusForbidden, api.CodeForbidden)
}

// Codes no device shows, entered again and again, limit the person entering them, as failed
// sign-ins do (D288 §2).
func TestPairing_TooManyWrongCodesLimitThePersonEnteringThem(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	var refused response
	for range 100 {
		refused = harness.do(t, http.MethodPost, "/pairings/code", admin,
			map[string]any{"code": "WRONGCODE", "deviceName": "TV"})
		if refused.status != http.StatusNotFound {
			break
		}
	}
	wantErrorSaying(t, refused, http.StatusTooManyRequests, api.CodeRateLimited,
		"too many failed sign-ins; wait and try again")
}

// Using an invitation's link makes the account and ends the link, and the list then no longer
// shows the invitation, with nothing to withdraw (D403).
func TestInvitations_AUsedOneIsNoLongerListed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	invitation := harness.invite(t, admin, "user", "the cousin")
	listed := harness.do(t, http.MethodGet, "/invitations", admin, nil)
	if !strings.Contains(listed.body, `"liveLinks":1`) {
		t.Errorf("GET /invitations = %s; want one live link", listed.body)
	}
	registered := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "cousin", "password": "cousin-password", "device": testDevice,
	})
	readSignIn(t, registered)
	wantError(t, harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "another", "password": "x", "device": testDevice,
	}), http.StatusUnauthorized, api.CodeAuthRequired)
	var after api.InvitationsOutput
	answer := harness.do(t, http.MethodGet, "/invitations", admin, nil)
	if err := json.Unmarshal([]byte(answer.body), &after.Body); err != nil || len(after.Body.Invitations) != 0 {
		t.Errorf("GET /invitations = %s; want the used invitation gone", answer.body)
	}
	withdraw := fmt.Sprintf("/invitations/%d", invitation.Invitation.ID)
	wantError(t, harness.do(t, http.MethodDelete, withdraw, admin, nil), http.StatusConflict, api.CodeConflict)
}

// The admin's list shows each waiting invitation's live links again, to pass on, until one is used
// (D267 §5, as the owner chose for invitations, 2026-10-28).
func TestInvitations_TheListShowsEachLiveLinkAgainUntilOneIsUsed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	invitation := harness.invite(t, admin, "user", "the neighbour")
	listedLinks := func() []api.IssuedSecretBody {
		var listed api.InvitationsOutput
		answer := harness.do(t, http.MethodGet, "/invitations", admin, nil)
		if err := json.Unmarshal([]byte(answer.body), &listed.Body); err != nil || len(listed.Body.Invitations) > 1 {
			t.Fatalf("GET /invitations = %d %s", answer.status, answer.body)
		}
		if len(listed.Body.Invitations) == 0 {
			return nil
		}
		return listed.Body.Invitations[0].Links
	}
	if links := listedLinks(); len(links) != 1 || links[0].Secret != invitation.Link.Secret ||
		links[0].ExpiresAt != invitation.Link.ExpiresAt {
		t.Errorf("the invitation's links = %+v; want its link, %+v, to pass on again", links, invitation.Link)
	}
	registered := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "neighbour", "password": "neighbour-password", "device": testDevice,
	})
	readSignIn(t, registered)
	if links := listedLinks(); len(links) != 0 {
		t.Errorf("the used invitation's links = %+v; want it not listed", links)
	}
}

// invite makes an invitation into role, with the admin's note, and returns it and its link.
func (harness testHarness) invite(t *testing.T, admin, role, note string) InvitationAndLink {
	t.Helper()
	invited := harness.do(t, http.MethodPost, "/invitations", admin, map[string]string{"role": role, "adminNote": note})
	var made api.InviteOutput
	if err := json.Unmarshal([]byte(invited.body), &made.Body); err != nil || invited.status != http.StatusCreated ||
		made.Body.Link.Secret == "" || made.Body.Invitation.AdminNote != note || made.Body.Invitation.Role != role {
		t.Fatalf("POST /invitations = %d %s", invited.status, invited.body)
	}
	return InvitationAndLink{Invitation: made.Body.Invitation, Link: made.Body.Link}
}

// InvitationAndLink is an invitation and its link, as POST /invitations answers.
type InvitationAndLink struct {
	Invitation api.InvitationBody
	Link       api.IssuedSecretBody
}

// accountNamed is the person named name, as the admin's list shows them.
func (harness testHarness) accountNamed(t *testing.T, admin, name string) api.UserEntryBody {
	t.Helper()
	listed := harness.do(t, http.MethodGet, "/users", admin, nil)
	var everyone api.UsersOutput
	if err := json.Unmarshal([]byte(listed.body), &everyone.Body); err != nil {
		t.Fatal(err)
	}
	for _, account := range everyone.Body.Users {
		if account.Name == name {
			return account
		}
	}
	t.Fatalf("GET /users lists no %s: %s", name, listed.body)
	return api.UserEntryBody{}
}

func TestPeople_AnAdminRemovesAPersonWhoseNextRequestIsRefused(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.do(t, http.MethodPost, "/users", admin,
		map[string]string{"name": "partner", "password": "partner-password", "role": "user"})
	partner, _ := readSignIn(t, harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "partner", "password": "partner-password", "device": testDevice,
	}))
	if me := harness.do(t, http.MethodGet, "/me", partner, nil); !strings.Contains(me.body, `"hasNoLibrary":true`) {
		t.Errorf("GET /me of a role given no library = %d %s", me.status, me.body)
	}
	if me := harness.do(t, http.MethodGet, "/me", admin, nil); !strings.Contains(me.body, `"hasNoLibrary":false`) {
		t.Errorf("GET /me of the admin = %d %s", me.status, me.body)
	}
	wantError(t, harness.do(t, http.MethodDelete, "/users/1", partner, nil), http.StatusForbidden, api.CodeForbidden)
	wantError(t, harness.do(t, http.MethodDelete, "/users/1", admin, nil), http.StatusConflict, api.CodeConflict)
	if removed := harness.do(t, http.MethodDelete, "/users/2", admin, nil); removed.status != http.StatusNoContent {
		t.Fatalf("DELETE /users/2 = %d %s", removed.status, removed.body)
	}
	if me := harness.do(t, http.MethodGet, "/me", partner, nil); me.status != http.StatusUnauthorized {
		t.Errorf("the removed person's next request = %d %s, want 401", me.status, me.body)
	}
	if listed := harness.do(t, http.MethodGet, "/users", admin, nil); strings.Contains(listed.body, "partner") {
		t.Errorf("GET /users after removal = %s", listed.body)
	}
	wantError(t, harness.do(t, http.MethodDelete, "/users/2", admin, nil), http.StatusNotFound, api.CodeNotFound)
}

// Removing a person keeps what they set for everyone, an override, a setting and a key, and forgets
// that they set it: no record of who set what is kept (D305 §1).
func TestPeople_ARemovedPersonsSettingsStayAndWhoSetThemIsForgotten(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.do(t, http.MethodPost, "/users", admin,
		map[string]string{"name": "partner", "password": "partner-password", "role": "user"})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 10})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{TitleID: 10})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO field_override (item_id, field, language, override_value, origin, set_by_user_id, set_at)
		 VALUES (10, 'name', '', 'Metropolis', 'typed', 2, 1)`,
		"UPDATE setting SET set_by_user_id = 2 WHERE key = 'sort_locale'",
		`INSERT INTO secret (scope, user_id, name, key_id, nonce, ciphertext, last_chars, set_by_user_id, set_at)
		 VALUES ('server', NULL, 'tmdb_api_key', 1, X'02', X'03', 'abcd', 2, 1)`)
	if removed := harness.do(t, http.MethodDelete, "/users/2", admin, nil); removed.status != http.StatusNoContent {
		t.Fatalf("DELETE /users/2 = %d %s", removed.status, removed.body)
	}
	//nolint:gosec // G101: a key's name in a query, not a key
	for table, kept := range map[string]string{
		"field_override": "item_id = 10", "setting": "key = 'sort_locale'", "secret": "name = 'tmdb_api_key'",
	} {
		got := storetest.QueryRaw(t, harness.databasePath, fmt.Sprintf(
			"SELECT (SELECT count(*) FROM %[1]s WHERE %[2]s) || '|' || "+
				"(SELECT count(*) FROM %[1]s WHERE set_by_user_id = 2)", table, kept))
		if !slices.Equal(got, []string{"1|0"}) {
			t.Errorf("%s after removal: %v rows kept|naming them; want 1|0, what they set kept and no one named",
				table, got)
		}
	}
}

// Removing a person deletes what deleted files left of their watching too (D305 §1, D306 §8).
func TestPeople_ARemovedPersonsWatchingOfDeletedFilesGoesWithThem(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.do(t, http.MethodPost, "/users", admin,
		map[string]string{"name": "partner", "password": "partner-password", "role": "user"})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 10})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 20, TitleID: 10})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO purged_watching (user_id, version_id, completed_count, watched_ms, last_watched_at)
		 VALUES (1, 20, 1, 600000, 1), (2, 20, 3, 900000, 1)`)
	if removed := harness.do(t, http.MethodDelete, "/users/2", admin, nil); removed.status != http.StatusNoContent {
		t.Fatalf("DELETE /users/2 = %d %s", removed.status, removed.body)
	}
	if got := storetest.QueryRaw(t, harness.databasePath, "SELECT user_id FROM purged_watching"); !slices.Equal(got,
		[]string{"1"}) {
		t.Errorf("watching of deleted files after removing person 2: %v; want only person 1's", got)
	}
}

// DELETE /session signs out on the device itself, which leaves the person's devices (D320 §1).
func TestSignOut_TheDeviceSignedOutOnLeavesThePersonsDevices(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	second := harness.signIn(t, "owner", "owner-password")
	if out := harness.do(t, http.MethodDelete, "/session", second, nil); out.status != http.StatusNoContent {
		t.Fatalf("DELETE /session = %d %s", out.status, out.body)
	}
	listed := harness.do(t, http.MethodGet, "/me/devices", admin, nil)
	var mine api.DevicesOutput
	if err := json.Unmarshal([]byte(listed.body), &mine.Body); err != nil || len(mine.Body.Devices) != 1 ||
		!mine.Body.Devices[0].IsThisDevice {
		t.Errorf("GET /me/devices after the other device signed out = %d %s; want this device alone",
			listed.status, listed.body)
	}
}

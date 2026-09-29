// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
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
	created := harness.do(t, http.MethodPost, "/users", admin, map[string]string{"name": "partner", "role": "user"})
	var made struct {
		Account                AccountBody    `json:"account"`
		RegistrationInvitation InvitationBody `json:"registrationInvitation"`
	}
	if err := json.Unmarshal([]byte(created.body), &made); err != nil || created.status != http.StatusCreated ||
		made.RegistrationInvitation.Secret == "" {
		t.Fatalf("POST /users = %d %s", created.status, created.body)
	}
	listed := harness.do(t, http.MethodGet, "/users", admin, nil)
	if !strings.Contains(listed.body, `"name":"partner","role":"user","isOutsider":false,"hasPassword":false`) {
		t.Errorf("GET /users = %s, want partner without a password yet", listed.body)
	}

	registered := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": made.RegistrationInvitation.Secret, "password": "partner-password", "device": testDevice,
	})
	partner, name := readSignIn(t, registered)
	if name != "partner" {
		t.Errorf("registering signed in as %q, want partner", name)
	}
	again := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": made.RegistrationInvitation.Secret, "password": "x", "device": testDevice,
	})
	wantError(t, again, http.StatusUnauthorized, CodeAuthRequired)
	wantError(t, harness.do(t, http.MethodPost, "/users", partner, map[string]string{"name": "x", "role": "user"}),
		http.StatusForbidden, CodeForbidden)

	code := harness.do(t, http.MethodPost, "/device-codes", partner, map[string]string{"deviceName": "Living room TV"})
	var issued InvitationBody
	if err := json.Unmarshal([]byte(code.body), &issued); err != nil || len(issued.Secret) != 9 {
		t.Fatalf("POST /device-codes = %d %s", code.status, code.body)
	}
	television := harness.do(t, http.MethodPost, "/session/code", "", map[string]string{
		"code": strings.ToUpper(issued.Secret), "platform": "web",
	})
	if _, name := readSignIn(t, television); name != "partner" {
		t.Errorf("the television signed in as %q, want partner", name)
	}
	wantError(t, harness.do(t, http.MethodPost, "/device-codes", partner,
		map[string]any{"deviceName": "TV", "userId": 1}), http.StatusForbidden, CodeForbidden)
	forPartner := harness.do(t, http.MethodPost, "/device-codes", admin,
		map[string]any{"deviceName": "Bedroom TV", "userId": made.Account.ID})
	if forPartner.status != http.StatusOK {
		t.Errorf("an admin's code for someone else: %d %s", forPartner.status, forPartner.body)
	}

	wantErrorSaying(t, harness.do(t, http.MethodPut, "/me/password", partner,
		map[string]string{"currentPassword": "wrong", "newPassword": "new"}), http.StatusForbidden, CodeForbidden,
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
	var mine DevicesOutput
	if err := json.Unmarshal([]byte(listed.body), &mine.Body); err != nil || len(mine.Body.Devices) != 2 {
		t.Fatalf("GET /me/devices = %d %s", listed.status, listed.body)
	}
	var other DeviceBody
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
	wantError(t, harness.do(t, http.MethodGet, "/me", second, nil), http.StatusUnauthorized, CodeAuthRequired)
	wantError(t, harness.do(t, http.MethodDelete, "/me/devices/9999", admin, nil), http.StatusNotFound, CodeNotFound)
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
	var body UserBody
	if err := json.Unmarshal([]byte(me.body), &body); err != nil || body.Device == nil ||
		body.Device.Name != "Firefox on Linux" {
		t.Fatalf("GET /me = %s, want the device it is on", me.body)
	}
	harness.do(t, http.MethodDelete, "/me/devices/"+itoa(body.Device.ID), admin, nil)
	signedOut := harness.do(t, http.MethodGet, "/items/1", other, nil)
	wantError(t, signedOut, http.StatusUnauthorized, CodeAuthRequired)
	if !strings.Contains(signedOut.body, `"reason":"`+ReasonSessionEnded+`"`) {
		t.Errorf("a removed device's request = %s, want the reason %s", signedOut.body, ReasonSessionEnded)
	}
	if never := harness.do(t, http.MethodGet, "/me", "", nil); !strings.Contains(never.body,
		`"reason":"`+ReasonNoSession+`"`) {
		t.Errorf("a request with no session = %s, want the reason %s", never.body, ReasonNoSession)
	}
}

// A client that signs in again naming the device it was before, as previousDeviceId, is that
// device again (D221 §1), not a new one.
func TestSession_ASignInNamingThePreviousDeviceResumesIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	first := harness.createAdminAndSignIn(t)
	before := harness.thisDeviceID(t, first)
	if out := harness.do(t, http.MethodDelete, "/session", first, nil); out.status != http.StatusNoContent {
		t.Fatalf("DELETE /session = %d %s", out.status, out.body)
	}
	signedIn := harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "owner", "password": "owner-password",
		"device": map[string]any{"name": "Firefox on Linux", "platform": "web", "previousDeviceId": before},
	})
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(signedIn.body), &session); err != nil || session.Token == "" {
		t.Fatalf("sign in again: %d %s", signedIn.status, signedIn.body)
	}
	if after := harness.thisDeviceID(t, session.Token); after != before {
		t.Errorf("signing in again naming device %d as previousDeviceId is device %d, want %d", before, after, before)
	}
}

func TestPeople_AnAdminRenamesAPersonAndSeesTheirPage(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.do(t, http.MethodPost, "/users", admin, map[string]string{"name": "partner", "role": "user"})
	wantError(t, harness.do(t, http.MethodPut, "/users/2", admin, map[string]string{"name": "OWNER"}),
		http.StatusConflict, CodeConflict)
	if renamed := harness.do(t, http.MethodPut, "/users/2", admin, map[string]string{"name": "Sam"}); renamed.status !=
		http.StatusNoContent {
		t.Fatalf("rename = %d %s", renamed.status, renamed.body)
	}
	page := harness.do(t, http.MethodGet, "/users/2", admin, nil)
	if !strings.Contains(page.body, `"name":"Sam"`) || !strings.Contains(page.body, `"watchedMs":0`) {
		t.Errorf("GET /users/2 = %d %s", page.status, page.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/users/99", admin, nil), http.StatusNotFound, CodeNotFound)
}

func TestDeviceCodes_OneWaitsAndIsForANewOrAListedDevice(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	waiting := func() string {
		t.Helper()
		return harness.do(t, http.MethodGet, "/me/device-code", admin, nil).body
	}
	if got := waiting(); !strings.Contains(got, `"isWaiting":false`) {
		t.Errorf("before any code: %s", got)
	}
	harness.do(t, http.MethodPost, "/device-codes", admin, map[string]string{"deviceName": "TV"})
	if got := waiting(); !strings.Contains(got, `"isWaiting":true,"deviceName":"TV"`) {
		t.Errorf("with a code made: %s", got)
	}
	wantError(t, harness.do(t, http.MethodPost, "/device-codes", admin, map[string]any{}),
		http.StatusBadRequest, CodeInvalidRequest)
	forListed := harness.do(t, http.MethodPost, "/device-codes", admin, map[string]any{"deviceId": 1})
	var code InvitationBody
	if err := json.Unmarshal([]byte(forListed.body), &code); err != nil || code.Secret == "" {
		t.Fatalf("a code for a listed device = %d %s", forListed.status, forListed.body)
	}
	used := harness.do(t, http.MethodPost, "/session/code", "",
		map[string]string{"code": code.Secret, "platform": "web"})
	if _, name := readSignIn(t, used); name != "owner" {
		t.Errorf("the code signed in %q", name)
	}
	if got := waiting(); !strings.Contains(got, `"isWaiting":false`) {
		t.Errorf("after the code was used: %s", got)
	}
}

func TestDeviceCodes_TooManyWrongCodesAreTooManyFailedSignIns(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	var refused response
	for range 100 {
		refused = harness.do(t, http.MethodPost, "/session/code", "",
			map[string]string{"code": "WRONGCODE", "platform": "web"})
		if refused.status != http.StatusUnauthorized {
			break
		}
	}
	wantErrorSaying(t, refused, http.StatusTooManyRequests, CodeRateLimited,
		"too many failed sign-ins; wait and try again")
	wantErrorSaying(t, harness.do(t, http.MethodPost, "/session", "",
		map[string]any{"name": "owner", "password": "owner-password", "device": testDevice}),
		http.StatusTooManyRequests, CodeRateLimited, "too many failed sign-ins; wait and try again")
}

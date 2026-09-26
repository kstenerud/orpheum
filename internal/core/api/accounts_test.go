// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// session reads a sign-in's answer: the token, and whose it is.
func session(t *testing.T, answer response) (token, name string) {
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
		Account      AccountBody    `json:"account"`
		Registration InvitationBody `json:"registration"`
	}
	if err := json.Unmarshal([]byte(created.body), &made); err != nil || created.status != http.StatusCreated ||
		made.Registration.Secret == "" {
		t.Fatalf("POST /users = %d %s", created.status, created.body)
	}
	listed := harness.do(t, http.MethodGet, "/users", admin, nil)
	if !strings.Contains(listed.body, `"name":"partner","role":"user","isOutsider":false,"hasPassword":false`) {
		t.Errorf("GET /users = %s, want partner without a password yet", listed.body)
	}

	registered := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": made.Registration.Secret, "password": "partner-password", "device": testDevice,
	})
	partner, name := session(t, registered)
	if name != "partner" {
		t.Errorf("registering signed in as %q, want partner", name)
	}
	again := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": made.Registration.Secret, "password": "x", "device": testDevice,
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
	if _, name := session(t, television); name != "partner" {
		t.Errorf("the television signed in as %q, want partner", name)
	}
	wantError(t, harness.do(t, http.MethodPost, "/device-codes", partner,
		map[string]any{"deviceName": "TV", "userId": 1}), http.StatusForbidden, CodeForbidden)
	forPartner := harness.do(t, http.MethodPost, "/device-codes", admin,
		map[string]any{"deviceName": "Bedroom TV", "userId": made.Account.ID})
	if forPartner.status != http.StatusOK {
		t.Errorf("an admin's code for someone else: %d %s", forPartner.status, forPartner.body)
	}

	wantError(t, harness.do(t, http.MethodPut, "/me/password", partner,
		map[string]string{"currentPassword": "wrong", "newPassword": "new"}), http.StatusForbidden, CodeForbidden)
	if changed := harness.do(t, http.MethodPut, "/me/password", partner,
		map[string]string{"currentPassword": "partner-password", "newPassword": "new"}); changed.status !=
		http.StatusNoContent {
		t.Errorf("PUT /me/password = %d %s", changed.status, changed.body)
	}
	harness.signIn(t, "partner", "new")
}

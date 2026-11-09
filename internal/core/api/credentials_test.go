// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
)

// pathParameter is a {name} in an operation's path.
var pathParameter = regexp.MustCompile(`\{[^}]+\}`)

// offByOne is token with its last character changed: its length and alphabet, but not it.
func offByOne(token string) string {
	replacement := "A"
	if strings.HasSuffix(token, "A") {
		replacement = "B"
	}
	return token[:len(token)-1] + replacement
}

// wrongCredentials are credentials that are present and well formed but no one's live session:
// what Plex2Shell's first step needed only to be there (security-week-2026-10-30 §5). Each is
// made through the API, as a client would hold it.
func wrongCredentials(t *testing.T, harness testHarness, admin string) map[string]string {
	t.Helper()
	adminPrincipal, err := harness.authService.Authenticate(t.Context(), admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	signedOut := harness.signIn(t, "partner", "partner-password")
	if answer := harness.do(t, http.MethodDelete, "/session", signedOut, nil); answer.status >= http.StatusBadRequest {
		t.Fatalf("sign out: %d %s", answer.status, answer.body)
	}
	signedIn := harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "partner", "password": "partner-password", "device": testDevice,
	})
	var issued struct {
		DeviceSecret string `json:"deviceSecret"`
	}
	if err = json.Unmarshal([]byte(signedIn.body), &issued); err != nil || issued.DeviceSecret == "" {
		t.Fatalf("a sign-in gave no device secret: %d %s", signedIn.status, signedIn.body)
	}
	if _, err = harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "leaver", Password: "leaver-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	removed := harness.signIn(t, "leaver", "leaver-password")
	removedPrincipal, err := harness.authService.Authenticate(t.Context(), removed)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.authService.RemoveUser(t.Context(), adminPrincipal.UserID, removedPrincipal.UserID); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"a well-formed token no one was given": strings.Repeat("A", len(admin)),
		"a live session's, one character off":  offByOne(admin),
		"a signed-out session's":               signedOut,
		"a device secret, not a session":       issued.DeviceSecret,
		"a removed person's session":           removed,
	}
}

// Every operation answers a request carrying a wrong credential, in its header, its media cookie or
// its viewer header, no better than one carrying none: where none is refused as not signed in, so is
// the wrong one, and where none is refused otherwise, the wrong one is refused too. A session that
// lapsed unused is answered as none too. So no operation checks that a credential is there rather
// than that it is a live one.
func TestCredentials_EveryOperationAnswersAWrongCredentialNoBetterThanNone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	wrong := wrongCredentials(t, harness, admin)
	lapsing := harness.signIn(t, "partner", "partner-password")

	type carried struct {
		token  string
		header http.Header
	}
	ways := func(credential string) map[string]carried {
		return map[string]carried{
			"header":        {token: credential},
			"media cookie":  {header: http.Header{"Cookie": {api.MediaCookieName + "=" + credential}}},
			"viewer cookie": {header: http.Header{"Cookie": {api.ViewerCookieName + "=" + credential}}},
			"viewer header": {header: http.Header{api.ViewerHeaderName: {credential}}},
		}
	}
	operations := api.OperationsIn(harness.server.OpenAPI())
	ask := func(operation api.DocumentedOperation, way carried) response {
		path := pathParameter.ReplaceAllString(operation.Path(), "1")
		var body any
		switch operation.Method() {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			body = map[string]any{}
		}
		return harness.doWithHeader(t, operation.Method(), path, way.token, way.header, body)
	}
	judge := func(operation api.DocumentedOperation, what string, none, got response) {
		switch {
		case none.status == http.StatusUnauthorized && got.status != http.StatusUnauthorized:
			t.Errorf("%s (%s %s) with %s = %d %.200s; want 401, as with no credential", operation.ID(),
				operation.Method(), operation.Path(), what, got.status, got.body)
		case none.status >= http.StatusBadRequest && got.status < http.StatusBadRequest:
			t.Errorf("%s (%s %s) with %s = %d; want it refused, as with no credential (%d)", operation.ID(),
				operation.Method(), operation.Path(), what, got.status, none.status)
		}
	}
	refusedAsSignedOut := 0
	for _, operation := range operations {
		none := ask(operation, carried{})
		if none.status == http.StatusUnauthorized {
			refusedAsSignedOut++
		}
		for name, credential := range wrong {
			for way, carrying := range ways(credential) {
				judge(operation, name+" in its "+way, none, ask(operation, carrying))
			}
		}
	}
	// So the sweep above is seen to reach the operations' sign-in checks, and not only refusals of
	// their bodies or parameters.
	if refusedAsSignedOut < 50 {
		t.Errorf("only %d operations refused a request with no credential as not signed in; want the "+
			"sweep to reach their sign-in checks", refusedAsSignedOut)
	}

	if answer := harness.do(t, http.MethodGet, "/me", lapsing, nil); answer.status != http.StatusOK {
		t.Fatalf("the session to lapse does not sign in yet: %d", answer.status)
	}
	harness.clock.Advance(2161 * time.Hour) // past sessionIdleExpiryHours' starting 2160
	for _, operation := range operations {
		none := ask(operation, carried{})
		judge(operation, "a session that lapsed unused", none, ask(operation, carried{token: lapsing}))
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The loader signs in for a code and keeps no session; the client origin takes the session by the
// code, once, as a sign-in's answer (D308 §4).
func TestHandOff_TheLoaderGetsACodeAndTheClientOriginTheSession(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	const credentials = `{"name":"owner","password":"owner-password",` +
		`"device":{"name":"Firefox on Linux","platform":"web"}}`
	handedOff, handedOffBody := harness.doWithHeaders(t, http.MethodPost, "/session/handoff", credentials, nil)
	var code struct {
		Code  string `json:"code"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(handedOffBody), &code); err != nil || handedOff.StatusCode != http.StatusCreated ||
		code.Code == "" || code.Token != "" {
		t.Fatalf("POST /session/handoff = %d %s (%v); want a code and no session", handedOff.StatusCode,
			handedOffBody, err)
	}
	if cookies := (&http.Response{Header: handedOff.Header}).Cookies(); len(cookies) != 0 {
		t.Errorf("the hand-off set cookies %+v; want none, the loader keeping nothing", cookies)
	}
	redeem := `{"code":"` + code.Code + `"}`
	redeemed, redeemedBody := harness.doWithHeaders(t, http.MethodPost, "/session/redeem", redeem, nil)
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(redeemedBody), &session); err != nil ||
		redeemed.StatusCode != http.StatusCreated || session.Token == "" {
		t.Fatalf("POST /session/redeem = %d %s (%v); want the session", redeemed.StatusCode, redeemedBody, err)
	}
	if me := harness.do(t, http.MethodGet, "/me", session.Token, nil); me.status != http.StatusOK {
		t.Errorf("GET /me by the session taken = %d; want 200", me.status)
	}
	again, _ := harness.doWithHeaders(t, http.MethodPost, "/session/redeem", redeem, nil)
	if again.StatusCode != http.StatusUnauthorized {
		t.Errorf("the code redeemed a second time = %d; want 401", again.StatusCode)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
)

// signInFrom signs the owner in with header on the request, and returns the session's token.
func (harness testHarness) signInFrom(t *testing.T, header http.Header) string {
	t.Helper()
	token, _ := readSignIn(t, harness.doWithHeader(t, http.MethodPost, "/session", "", header, map[string]any{
		"name": "owner", "password": "owner-password", "device": testDevice,
	}))
	return token
}

// mayAdminister is whether a session may make an admin's call and change its own password.
func (harness testHarness) mayAdminister(t *testing.T, token string) (isAdmin, canManageAccount bool) {
	t.Helper()
	isAdmin = harness.do(t, http.MethodGet, "/users", token, nil).status == http.StatusOK
	changed := harness.do(t, http.MethodPut, "/me/password", token, map[string]string{
		"currentPassword": "owner-password", "newPassword": "owner-password",
	})
	return isAdmin, changed.status != http.StatusForbidden
}

// A sign-in from a page another origin served (a discovery's client) gets no admin and may not change
// the account's security, whatever its role; one from the core's own origin, or no browser's, does
// (D292 §4; the security audit's A1).
func TestSignIn_FromAnotherOriginCarriesNoAdmin(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	for name, origin := range map[string]string{
		"a discovery's client": "https://app.orpheum-app.com", "an opaque origin": "null",
	} {
		token := harness.signInFrom(t, http.Header{"Origin": {origin}})
		if isAdmin, canManage := harness.mayAdminister(t, token); isAdmin || canManage {
			t.Errorf("a sign-in from %s may administer: %t, manage its account: %t; want neither", name, isAdmin,
				canManage)
		}
	}
	for name, header := range map[string]http.Header{
		"the core's own origin": {"Origin": {harness.httpServer.URL}}, "no browser": nil,
	} {
		if isAdmin, canManage := harness.mayAdminister(t, harness.signInFrom(t, header)); !isAdmin || !canManage {
			t.Errorf("a sign-in from %s may administer: %t, manage its account: %t; want both", name, isAdmin,
				canManage)
		}
	}
}

// A sign-in over a WebRTC data channel gets no admin, though its head claims the core's own origin:
// the head is the page's, and the core believes none of it that says where it came from (A1).
func TestSignIn_OverWebRTCCarriesNoAdminWhateverItsHeadClaims(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	rtc := rtcapi.New(t.Context(), harness.server.Handler(), slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = rtc.Close() }) //nolint:errcheck // the test is over
	harness.server.SetRTC(rtc)
	admin := harness.createAdminAndSignIn(t)
	//nolint:contextcheck // the harness's requests carry the test's context
	link := rtcapitest.Connect(t, func(_ context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		answered := harness.do(t, http.MethodPost, "/rtc/offer", admin, map[string]any{
			"type": offer.Type, "sdp": offer.SDP,
		})
		var answer SessionDescriptionBody
		if answered.status != http.StatusOK || json.Unmarshal([]byte(answered.body), &answer) != nil {
			return rtcapi.Description{}, fmt.Errorf("POST /rtc/offer = %d %s", answered.status, answered.body)
		}
		return rtcapi.Description{Type: answer.Type, SDP: answer.SDP}, nil
	})
	body, err := json.Marshal(map[string]any{"name": "owner", "password": "owner-password", "device": testDevice})
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(harness.httpServer.URL, "http://")
	signedIn := link.Do(rtcapi.RequestHead{
		Method: http.MethodPost, Path: Prefix + "/session",
		Headers: http.Header{
			"content-type": {"application/json"}, "origin": {harness.httpServer.URL}, "host": {host},
		},
	}, body)
	token, _ := readSignIn(t, response{status: signedIn.Head.Status, body: string(signedIn.Body)})
	if isAdmin, canManage := harness.mayAdminister(t, token); isAdmin || canManage {
		t.Errorf("a sign-in over WebRTC may administer: %t, manage its account: %t; want neither", isAdmin, canManage)
	}
}

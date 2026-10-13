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

// An admin administers from anywhere: a sign-in from a page another origin served (a discovery's
// client), from the core's own origin or from no browser carries the whole role (D294 §1, §4).
func TestSignIn_AnAdminAdministersFromAnyOrigin(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	for name, header := range map[string]http.Header{
		"a discovery's client":  {"Origin": {"https://app.orpheum-app.com"}},
		"the core's own origin": {"Origin": {harness.httpServer.URL}}, "no browser": nil,
	} {
		if isAdmin, canManage := harness.mayAdminister(t, harness.signInFrom(t, header)); !isAdmin || !canManage {
			t.Errorf("a sign-in from %s may administer: %t, manage its account: %t; want both", name, isAdmin,
				canManage)
		}
	}
}

// An admin administers over WebRTC too, the one way into a core no direct path reaches (D294 §4).
func TestSignIn_AnAdminAdministersOverWebRTC(t *testing.T) {
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
	if isAdmin, canManage := harness.mayAdminister(t, token); !isAdmin || !canManage {
		t.Errorf("a sign-in over WebRTC may administer: %t, manage its account: %t; want both", isAdmin, canManage)
	}
}

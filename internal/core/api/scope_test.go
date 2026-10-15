// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
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

// An admin at home administers whatever page they signed in from: one another origin served (a
// discovery's client), the core's own, or no browser (D294 §1, §4; D297 §6 keeps it to the house).
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

// An admin administers only from the house's network, directly: over WebRTC, whose source the core
// cannot know, and from any other network, their session is a person's, until passkeys (D297 §6,
// revising D294 §4).
func TestAdmin_AdministersOnlyFromTheHousesNetworkNeverOverWebRTC(t *testing.T) {
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
	host := strings.TrimPrefix(harness.httpServer.URL, "http://")
	overWebRTC := link.Do(rtcapi.RequestHead{
		Method: http.MethodGet, Path: Prefix + "/users",
		Headers: http.Header{"authorization": {"Bearer " + admin}, "origin": {harness.httpServer.URL}, "host": {host}},
	}, nil)
	if overWebRTC.Head.Status != http.StatusForbidden || !strings.Contains(string(overWebRTC.Body), "own network") {
		t.Errorf("an admin's GET /users over WebRTC = %d %s; want 403, administering only at home",
			overWebRTC.Head.Status, overWebRTC.Body)
	}
	// Asked over WebRTC, the page is told it does not administer there, so it offers nothing it may not do.
	asked := link.Do(rtcapi.RequestHead{
		Method: http.MethodGet, Path: Prefix + "/me",
		Headers: http.Header{"authorization": {"Bearer " + admin}, "origin": {harness.httpServer.URL}, "host": {host}},
	}, nil)
	var away UserBody
	isRead := asked.Head.Status == http.StatusOK && json.Unmarshal(asked.Body, &away) == nil
	if !isRead || away.IsAdmin || away.Role != "admin" {
		t.Errorf("an admin's /me over WebRTC = %d %s; want their role, and isAdmin false",
			asked.Head.Status, asked.Body)
	}
	if isAdmin, _ := harness.mayAdminister(t, admin); !isAdmin {
		t.Error("the admin may not administer from home, this machine")
	}
	harness.server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")})
	wantError(t, harness.do(t, http.MethodGet, "/users", admin, nil), http.StatusForbidden, CodeForbidden)
	if _, canManage := harness.mayAdminister(t, admin); !canManage {
		t.Error("away from home the admin may not manage their own account, which a person may")
	}
}

// A device a code activated watches and hosts but may not change the account's security: its
// password, removing a device, signing out the others, or entering a code for another device (D292
// §4; the security audit's A11).
func TestPairing_ACodeActivatedDeviceMayNotChangeTheAccountsSecurity(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	television, _ := readSignIn(t, harness.activate(t, admin, map[string]any{"deviceName": "Living room TV"}))
	if isAdmin, canManage := harness.mayAdminister(t, television); isAdmin || canManage {
		t.Errorf("the television may administer: %t, change its password: %t; want neither", isAdmin, canManage)
	}
	adminDevice := harness.thisDeviceID(t, admin)
	wantError(t, harness.do(t, http.MethodDelete, "/me/devices/"+strconv.FormatInt(adminDevice, 10), television,
		nil), http.StatusForbidden, CodeForbidden)
	wantError(t, harness.doWithHeader(t, http.MethodPost, "/me/devices/sign-out-elsewhere", television,
		http.Header{"Idempotency-Key": {"k1"}}, nil), http.StatusForbidden, CodeForbidden)
	asked := harness.do(t, http.MethodPost, "/pairings", "", map[string]any{"platform": "web"})
	var pairing PairingOutput
	if err := json.Unmarshal([]byte(asked.body), &pairing.Body); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodPost, "/activations", television,
		map[string]any{"code": pairing.Body.Code, "deviceName": "Phone"}), http.StatusForbidden, CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, "/activations/answer", television,
		map[string]any{"code": pairing.Body.Code, "isMatch": true}), http.StatusForbidden, CodeForbidden)
	if me := harness.do(t, http.MethodGet, "/me", television, nil); !strings.Contains(me.body,
		`"canManageAccount":false`) {
		t.Errorf("GET /me for the television = %s; want canManageAccount false", me.body)
	}
}

// Each session's beginning is logged with the path it came by and whether it administers there, so
// "why does my page offer no admin" is read from the log, not dug for (the owner, 2026-10-15).
func TestSession_ItsBeginningIsLoggedWithItsPathAndWhetherItAdministers(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.logs.AwaitLine(t, `"msg":"a session began"`, `"path":"direct from home","isAdmin":true`)
	harness.server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")})
	harness.signIn(t, "owner", "owner-password")
	harness.logs.AwaitLine(t, `"msg":"a session began"`, `"path":"direct from away","isAdmin":false`)
}

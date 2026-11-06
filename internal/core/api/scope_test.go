// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
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
	isAdmin = harness.do(t, http.MethodGet, "/users?limit=500", token, nil).status == http.StatusOK
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

// An admin administers only from the house's network: directly, or over a WebRTC connection whose
// selected pair reaches the core directly from it, as its own end of the pair says; from any other
// network, over WebRTC too, their session is a person's, until passkeys (D297 §6, D300 §3).
func TestAdmin_AdministersOnlyFromTheHousesNetworkDirectlyOrOverWebRTCFromIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	rtc := rtcapi.New(t.Context(), harness.server.Handler(), slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = rtc.Close() }) //nolint:errcheck // the test is over
	harness.server.SetRTC(rtc)
	// Home is this machine: the pair may reach the core over any of its addresses, loopback or an
	// interface's, whichever ICE settles on first (it chose the container's own address under load).
	harness.server.SetHomeNetworks(machineNetworks(t))
	admin := harness.createAdminAndSignIn(t)
	//nolint:contextcheck // the harness's requests carry the test's context
	link := rtcapitest.Connect(t, func(_ context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		answered := harness.do(t, http.MethodPost, "/rtc/offer", admin, map[string]any{
			"type": offer.Type, "sdp": offer.SDP,
		})
		var answer api.SessionDescriptionBody
		if answered.status != http.StatusOK || json.Unmarshal([]byte(answered.body), &answer) != nil {
			return rtcapi.Description{}, fmt.Errorf("POST /rtc/offer = %d %s", answered.status, answered.body)
		}
		return rtcapi.Description{Type: answer.Type, SDP: answer.SDP}, nil
	})
	host := strings.TrimPrefix(harness.httpServer.URL, "http://")
	ask := func(path string) rtcapitest.Reply {
		headers := http.Header{"authorization": {"Bearer " + admin}, "origin": {harness.httpServer.URL}, "host": {host}}
		return link.Do(rtcapi.RequestHead{Method: http.MethodGet, Path: api.Prefix + path, Headers: headers}, nil)
	}
	// The harness's home is this machine, which the connection's pair reaches directly.
	if users := ask("/users?limit=500"); users.Head.Status != http.StatusOK {
		t.Errorf("an admin's GET /users over WebRTC from home = %d %s; want 200", users.Head.Status, users.Body)
	}
	if !isAdminOver(t, ask("/me")) {
		t.Error("an admin's /me over WebRTC from home says they do not administer")
	}
	harness.server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")})
	users := ask("/users?limit=500")
	if users.Head.Status != http.StatusForbidden || !strings.Contains(string(users.Body), "own network") {
		t.Errorf("an admin's GET /users over WebRTC from away = %d %s; want 403, administering only at home",
			users.Head.Status, users.Body)
	}
	// Asked over WebRTC from away, the page is told it does not administer there.
	if isAdminOver(t, ask("/me")) {
		t.Error("an admin's /me over WebRTC from away says they administer")
	}
	wantError(t, harness.do(t, http.MethodGet, "/users?limit=500", admin, nil), http.StatusForbidden, api.CodeForbidden)
	if _, canManage := harness.mayAdminister(t, admin); !canManage {
		t.Error("away from home the admin may not manage their own account, which a person may")
	}
}

// isAdminOver is whether a /me answered over WebRTC says its session administers there, its role
// read as an admin's either way.
func isAdminOver(t *testing.T, asked rtcapitest.Reply) bool {
	t.Helper()
	var me api.UserBody
	if asked.Head.Status != http.StatusOK || json.Unmarshal(asked.Body, &me) != nil || me.Role != "admin" {
		t.Fatalf("an admin's /me over WebRTC = %d %s; want their role", asked.Head.Status, asked.Body)
	}
	return me.IsAdmin
}

// A relayed WebRTC pair never counts as home, whatever address its relay has: only a pair with no
// relay between reaches the core from the house (D300 §3).
func TestIsDirectPath_CountsNoRelayedOrFailedPath(t *testing.T) {
	t.Parallel()
	for path, want := range map[rtcapi.Path]bool{
		rtcapi.PathDirectIPv4: true, rtcapi.PathDirectIPv6: true, rtcapi.PathPunchedIPv4: true,
		rtcapi.PathPunchedIPv6: true, rtcapi.PathRelayed: false, rtcapi.PathFailed: false,
		rtcapi.PathHairpinnedIPv4: false,
	} {
		if got := api.IsDirectPath(path); got != want {
			t.Errorf("isDirectPath(%s) = %t; want %t", path, got, want)
		}
	}
}

// A device a code activated watches and hosts but may not change the account's security: its
// password, removing a device, signing out the others, entering a code for another device, or reading
// the person's own link (D292 §4; the security audit's A11; the device code audit's M4).
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
		nil), http.StatusForbidden, api.CodeForbidden)
	wantError(t, harness.doWithHeader(t, http.MethodPost, "/me/devices/sign-out-elsewhere", television,
		http.Header{"Idempotency-Key": {"k1"}}, nil), http.StatusForbidden, api.CodeForbidden)
	asked := harness.do(t, http.MethodPost, "/pairings", "", map[string]any{"platform": "web"})
	var pairing api.PairingOutput
	if err := json.Unmarshal([]byte(asked.body), &pairing.Body); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodPost, "/pairings/code", television,
		map[string]any{"code": pairing.Body.Code, "deviceName": "Phone"}), http.StatusForbidden, api.CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, "/pairings/check", television,
		map[string]any{"code": pairing.Body.Code, "isMatch": true}), http.StatusForbidden, api.CodeForbidden)
	if me := harness.do(t, http.MethodGet, "/me", television, nil); !strings.Contains(me.body,
		`"canManageAccount":false`) {
		t.Errorf("GET /me for the television = %s; want canManageAccount false", me.body)
	}
	// The person's devices say which a code activated, which their account page names (M4).
	if listed := harness.do(t, http.MethodGet, "/me/devices", admin, nil); !strings.Contains(listed.body,
		`"name":"Living room TV"`) || strings.Count(listed.body, `"isByCode":true`) != 1 {
		t.Errorf("GET /me/devices = %s; want the television alone marked as activated by a code", listed.body)
	}
	// Nor carry away the person's own link, which outlives the device removed (the device code
	// audit's M4); a password sign-in reads it.
	wantError(t, harness.do(t, http.MethodGet, "/me/own-link", television, nil), http.StatusForbidden,
		api.CodeForbidden)
	if own := harness.do(t, http.MethodGet, "/me/own-link", admin, nil); own.status == http.StatusForbidden {
		t.Errorf("GET /me/own-link for the admin's own sign-in = %d %s", own.status, own.body)
	}
}

// Each session's beginning is logged with the path it came by and whether it administers there, so
// "why does my page offer no admin" is read from the log, not dug for (the owner, 2026-10-15).
func TestSession_ItsBeginningIsLoggedWithItsPathAndWhetherItAdministers(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.logs.AwaitEvent(t, "sessionBegan", `"path":"direct from home","isAdmin":true`)
	harness.server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")})
	harness.signIn(t, "owner", "owner-password")
	harness.logs.AwaitEvent(t, "sessionBegan", `"path":"direct from away","isAdmin":false`)
}

// An admin administers from away from home only when they set it so, from home, and a person who is
// no admin sets nothing (D302): the server decides it from where the request came and the setting.
func TestAdmin_AdministersFromAwayOnlyByTheirOwnSettingSetFromHome(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	home := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}
	away := []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}
	setAway := func(token, value string) response {
		return harness.do(t, http.MethodPut, "/me/admin-away", token, map[string]any{"adminAway": value})
	}
	if me := harness.do(t, http.MethodGet, "/me", admin, nil); !strings.Contains(me.body, `"adminAway":"never"`) {
		t.Errorf("an admin's /me = %s; want never, the default", me.body)
	}
	harness.server.SetHomeNetworks(away)
	wantError(t, harness.do(t, http.MethodGet, "/users?limit=500", admin, nil), http.StatusForbidden, api.CodeForbidden)
	wantError(t, setAway(admin, "always"), http.StatusForbidden, api.CodeForbidden)
	harness.server.SetHomeNetworks(home)
	if set := setAway(admin, "always"); set.status != http.StatusNoContent {
		t.Fatalf("setting always from home = %d %s", set.status, set.body)
	}
	harness.server.SetHomeNetworks(away)
	if users := harness.do(t, http.MethodGet, "/users?limit=500", admin, nil); users.status != http.StatusOK {
		t.Errorf("an admin set to always, from away: GET /users = %d %s; want 200", users.status, users.body)
	}
	if isAdmin, _ := harness.mayAdminister(t, admin); !isAdmin {
		t.Error("an admin set to always is told from away that they do not administer")
	}
	// From away they may not turn it off either: it is set at home alone.
	wantError(t, setAway(admin, "never"), http.StatusForbidden, api.CodeForbidden)
	harness.server.SetHomeNetworks(home)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	wantError(t, setAway(partner, "always"), http.StatusForbidden, api.CodeForbidden)
	if me := harness.do(t, http.MethodGet, "/me", partner, nil); strings.Contains(me.body, "adminAway") {
		t.Errorf("a person's /me = %s; says nothing of administering", me.body)
	}
	wantError(t, setAway(admin, "sometimes"), http.StatusUnprocessableEntity, api.CodeInvalidRequest)
}

// machineNetworks are this machine's own addresses, each as a one-address network.
func machineNetworks(t *testing.T) []netip.Prefix {
	t.Helper()
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var networks []netip.Prefix
	for _, address := range addresses {
		if prefix, parseErr := netip.ParsePrefix(address.String()); parseErr == nil {
			networks = append(networks, netip.PrefixFrom(prefix.Addr().Unmap().WithZone(""), prefix.Addr().BitLen()))
		}
	}
	return networks
}

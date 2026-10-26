// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// A sign-in hands the device its locator, apart from its token and device secret, and each visit a
// fresh one in its place, over the direct path alone (discovery-core.md K7).
func TestSession_ASignInHandsTheDeviceALocatorAndAVisitAFreshOne(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	var issued struct {
		Token        string `json:"token"`
		DeviceSecret string `json:"deviceSecret"`
		Locator      string `json:"locator"`
	}
	answered := harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "owner", "password": "owner-password", "device": testDevice,
	})
	if err := json.Unmarshal([]byte(answered.body), &issued); err != nil || issued.Locator == "" ||
		issued.Locator == issued.Token || issued.Locator == issued.DeviceSecret {
		t.Fatalf("a sign-in = %d %s; want a locator apart from the token and the device secret", answered.status,
			answered.body)
	}
	var fresh struct {
		Locator string `json:"locator"`
	}
	visited := harness.do(t, http.MethodPost, "/me/locator", issued.Token, nil)
	if err := json.Unmarshal([]byte(visited.body), &fresh); err != nil || fresh.Locator == "" ||
		fresh.Locator == issued.Locator {
		t.Errorf("a visit's locator = %d %s; want a fresh one", visited.status, visited.body)
	}
	signedOut := harness.do(t, http.MethodPost, "/me/locator", "", nil)
	if signedOut.status != http.StatusUnauthorized {
		t.Errorf("a locator for no session = %d; want refused", signedOut.status)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// A link made while a discovery has claimed the core is the discovery's, naming the core and its key's
// fingerprint around the secret; before, the client makes it on its own origin (the protocol's §3.1).
func TestDiscoveryLink_IsTheDiscoverysFormOnlyWhileItHasClaimedTheCore(t *testing.T) {
	t.Parallel()
	fingerprint := "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw"
	claimed := discovery.Status{
		State: discoveryproto.StateClaimed, CoreID: "k7xq2", ClientOrigin: "https://app.orpheum-app.com",
		Fingerprint: fingerprint,
	}
	got := api.DiscoveryLink(claimed, "/register", "a-secret")
	if got != "https://app.orpheum-app.com/register#k7xq2.a-secret."+fingerprint {
		t.Errorf("a claimed core's link is %q; want the discovery's form", got)
	}
	read, err := link.Read(got[len("https://app.orpheum-app.com/register"):])
	if err != nil || read.Secret != "a-secret" || read.CoreID != "k7xq2" {
		t.Errorf("the link reads back as %+v, %v; want its core and secret", read, err)
	}
	for name, status := range map[string]discovery.Status{
		"pending": {
			State: discoveryproto.StatePending, CoreID: "k7xq2", ClientOrigin: "https://app.example",
			Fingerprint: fingerprint,
		},
		"no client origin":   {State: discoveryproto.StateClaimed, CoreID: "k7xq2", Fingerprint: fingerprint},
		"enrolled with none": {},
	} {
		if got := api.DiscoveryLink(status, "/register", "a-secret"); got != "" {
			t.Errorf("a core %s makes the link %q; want none, the client making it on its own origin", name, got)
		}
	}
}

// A device that joins a Watch Together session is handed the locator a discovery finds the core by
// for it, which the core registers (discovery-core.md K7); a core no discovery has claimed makes its
// links on the client's origin, naming none.
func TestTogether_AJoiningDeviceIsHandedTheLocatorTheCoreRegisters(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	joined := harness.joinByLink(t, admin, opened.Body.SessionID, "my partner", "Pat")
	dependencies, err := harness.server.ReadyDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if joined.Body.Locator == "" ||
		!slices.Equal(dependencies.Together.LiveLocators(), []string{link.LocatorHash(joined.Body.Locator)}) {
		t.Errorf("the joined device was handed locator %q, and the core registers %v; want the one handed",
			joined.Body.Locator, dependencies.Together.LiveLocators())
	}
	if made := api.TogetherLink(dependencies, "a-secret"); made != "" {
		t.Errorf("a core no discovery has claimed names the link %q; want none", made)
	}
}

// A person's page says until when their own link leads them back, and the admin alone ends it,
// leaving them signed in (D292 §4; A12).
func TestPerson_TheAdminSeesAndEndsAPersonsOwnLink(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	invitation := harness.invite(t, admin, "user", "the partner")
	partner, _ := readSignIn(t, harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "partner", "password": "partner-password", "device": testDevice,
	}))
	id := harness.accountNamed(t, admin, "partner").ID
	path := fmt.Sprintf("/users/%d", id)
	if page := harness.personPage(t, admin, path); page.Body.OwnLinkExpiresAt <= harness.clock.Now().UnixMilli() {
		t.Errorf("the person's page says their own link ends at %d; want a time to come", page.Body.OwnLinkExpiresAt)
	}
	wantError(t, harness.do(t, http.MethodDelete, path+"/own-link", partner, nil), http.StatusForbidden,
		api.CodeForbidden)
	if ended := harness.do(t, http.MethodDelete, path+"/own-link", admin, nil); ended.status != http.StatusNoContent {
		t.Fatalf("DELETE %s/own-link = %d %s", path, ended.status, ended.body)
	}
	if page := harness.personPage(t, admin, path); page.Body.OwnLinkExpiresAt != 0 {
		t.Errorf("after it ended, the person's page says their own link ends at %d; want none",
			page.Body.OwnLinkExpiresAt)
	}
	if me := harness.do(t, http.MethodGet, "/me", partner, nil); me.status != http.StatusOK {
		t.Errorf("ending their own link signed the person out: GET /me = %d", me.status)
	}
	wantError(t, harness.do(t, http.MethodDelete, "/users/99/own-link", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
}

// personPage is GET path, a person's page, as the admin.
func (harness testHarness) personPage(t *testing.T, admin, path string) api.UserPageOutput {
	t.Helper()
	read := harness.do(t, http.MethodGet, path, admin, nil)
	var page api.UserPageOutput
	if err := json.Unmarshal([]byte(read.body), &page.Body); err != nil || read.status != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, read.status, read.body)
	}
	return page
}

// A person who registered by a link is shown it again as their own link, in the discovery's form
// once one has claimed the server; someone with none is told so (D292 §4; discovery-core.md K7).
func TestOwnLink_IsTheLinkThePersonRegisteredBy(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	invitation := harness.invite(t, admin, "user", "the partner")
	partner, _ := readSignIn(t, harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "partner", "password": "partner-password", "device": testDevice,
	}))
	read := harness.do(t, http.MethodGet, "/me/own-link", partner, nil)
	var own api.OwnLinkOutput
	if err := json.Unmarshal([]byte(read.body), &own.Body); err != nil || read.status != http.StatusOK {
		t.Fatalf("GET /me/own-link = %d %s", read.status, read.body)
	}
	if own.Body.Secret != invitation.Link.Secret || own.Body.ExpiresAt <= harness.clock.Now().UnixMilli() {
		t.Errorf("the partner's own link is %+v; want the invitation's secret, alive", own.Body)
	}
	if own.Body.Link != "" {
		t.Errorf("a server no discovery has claimed names the link %q; want none", own.Body.Link)
	}
	wantError(t, harness.do(t, http.MethodGet, "/me/own-link", admin, nil), http.StatusNotFound, api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodGet, "/me/own-link", "", nil), http.StatusUnauthorized, api.CodeAuthRequired)
}

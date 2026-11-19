// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
)

// sectionsOf is the sections a page's first read carries (the client-server audit's finding 10),
// each part as its JSON; nil when it carries none.
func sectionsOf(t *testing.T, label string, answer response) map[string]json.RawMessage {
	t.Helper()
	if answer.status != http.StatusOK {
		t.Fatalf("%s = %d %s", label, answer.status, answer.body)
	}
	var body struct {
		Sections map[string]json.RawMessage `json:"sections"`
	}
	if err := json.Unmarshal([]byte(answer.body), &body); err != nil {
		t.Fatalf("%s: %v in %s", label, err, answer.body)
	}
	return body.Sections
}

// wantSections is that the first read at path carries each part as the endpoint it names answers
// admin, that the plain read carries none, and that someone who does not administer is refused both.
func (harness testHarness) wantSections(
	t *testing.T, path, admin, partner string, parts map[string]string,
) {
	t.Helper()
	first := path + "?sections=true"
	if strings.Contains(path, "?") {
		first = path + "&sections=true"
	}
	sections := sectionsOf(t, "GET "+first, harness.do(t, http.MethodGet, first, admin, nil))
	if len(sections) != len(parts) {
		t.Errorf("GET %s carries %d sections, want %d: %v", first, len(sections), len(parts), sections)
	}
	for name, endpoint := range parts {
		part, isCarried := sections[name]
		if !isCarried {
			t.Errorf("GET %s carries no %s", first, name)
			continue
		}
		harness.wantPart(t, fmt.Sprintf("GET %s's %s", first, name), part, endpoint, admin)
	}
	if plain := sectionsOf(t, "GET "+path, harness.do(t, http.MethodGet, path, admin, nil)); plain != nil {
		t.Errorf("GET %s carries sections %v; its polls and refreshes read each at its own endpoint", path, plain)
	}
	wantError(t, harness.do(t, http.MethodGet, first, partner, nil), http.StatusForbidden, api.CodeForbidden)
}

// signInPartner is a person who does not administer, signed in.
func (harness testHarness) signInPartner(t *testing.T) string {
	t.Helper()
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	return harness.signIn(t, "partner", "partner-password")
}

// The System page's first read, GET /system?sections=true, carries what its other sections read at
// its endpoints, each as that endpoint says it, so the page waits on one request (the client-server
// audit's finding 10): the reachability check, asked for as the admin asks, and the discovery, whose
// credentials a gzipped answer never carries (D342), are not among them, and on a server keeping no
// network settings, neither is the network.
func TestSystem_ThePagesFirstReadCarriesWhatItsSectionsReadApart(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	partner := harness.signInPartner(t)
	harness.wantSections(t, "/system", admin, partner, map[string]string{
		"nowPlaying":     "/now-playing",
		"resourceUse":    "/system/resource-use",
		"backups":        "/backups",
		"unusedSettings": "/config/unused",
		"filingReports":  "/filing-reports?limit=500",
		"problemReports": "/problem-reports",
	})
}

// A user's page's first read, GET /users/{id}?sections=true, carries their devices and the roles to
// choose theirs from, each as its endpoint says it (the client-server audit's finding 10).
func TestUser_ThePagesFirstReadCarriesTheirDevicesAndTheRolesToChoose(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	partner := harness.signInPartner(t)
	path := fmt.Sprintf("/users/%d", harness.accountNamed(t, admin, "partner").ID)
	harness.wantSections(t, path, admin, partner, map[string]string{
		"devices": path + "/devices",
		"roles":   "/roles",
	})
}

// The Users page's first read, GET /users?limit=500&sections=true, carries the roles as GET /roles
// says them (the client-server audit's finding 10); the invitations, which carry their links, a
// gzipped answer never carries (D342).
func TestUsers_ThePagesFirstReadCarriesTheRoles(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	partner := harness.signInPartner(t)
	harness.wantSections(t, "/users?limit=500", admin, partner, map[string]string{"roles": "/roles"})
}

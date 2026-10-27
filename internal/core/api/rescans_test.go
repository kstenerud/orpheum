// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// D357 §1, §5: the rescans are an admin's. An admin's press scans the library and starts its rescan,
// whose progress the Libraries page reads; a second press adds nothing; a person of another role is
// refused every rescan.
func TestRescan_IsForAnAdminOnly(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/shows")
	if created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")); created.status !=
		http.StatusCreated {
		t.Fatalf("POST /libraries = %d %s", created.status, created.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	user := harness.signIn(t, "partner", "partner-password")
	storetest.ExecRaw(t, harness.databasePath, // the user role reaches the library, as an admin gives it
		`INSERT INTO role_library (role_id, library_id) SELECT id, 1 FROM role WHERE name = 'user'`)
	for _, path := range []string{"/libraries/rescan", "/libraries/1/rescan", "/items/1/rescan"} {
		wantError(t, harness.do(t, http.MethodPost, path, user, nil), http.StatusForbidden, api.CodeForbidden)
	}
	harness.nodes.scanned = nil
	started := harness.do(t, http.MethodPost, "/libraries/1/rescan", admin, nil)
	if started.status != http.StatusAccepted || !strings.Contains(started.body, `"isStarted":true`) {
		t.Fatalf("an admin's POST /libraries/1/rescan = %d %s", started.status, started.body)
	}
	if !slices.Equal(harness.nodes.scanned, []string{"/media/shows"}) {
		t.Errorf("scans asked for: %v; want the library's root", harness.nodes.scanned)
	}
	again := harness.do(t, http.MethodPost, "/libraries/1/rescan", admin, nil)
	if again.status != http.StatusAccepted || !strings.Contains(again.body, `"isStarted":false`) {
		t.Errorf("a second press = %d %s; want nothing started", again.status, again.body)
	}
	everything := harness.do(t, http.MethodPost, "/libraries/rescan", admin, nil)
	if everything.status != http.StatusAccepted || !strings.Contains(everything.body, `"startedCount":0`) {
		t.Errorf("POST /libraries/rescan while it runs = %d %s; want none started", everything.status,
			everything.body)
	}
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"rescan":{"isScanning":true,"itemCount":0,"matchedCount":0}`) {
		t.Errorf("an admin's GET /libraries = %s; want the library's rescan, waiting for its scan", listed.body)
	}
	if seen := harness.do(t, http.MethodGet, "/libraries", user, nil); strings.Contains(seen.body, `"rescan"`) {
		t.Errorf("a user's GET /libraries = %s; want no rescan shown", seen.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/9/rescan", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodPost, "/items/99/rescan", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
}

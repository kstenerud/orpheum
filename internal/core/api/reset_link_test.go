// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// A registered person's page says until when their reset link is live, never the link; the admin
// alone cancels it, and a new one replaces it (D466 §2, §4).
func TestPerson_ThePageSaysUntilWhenAResetLinkIsLiveAndTheAdminCancelsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.do(t, http.MethodPost, "/users", admin,
		map[string]string{"name": "partner", "password": "partner-password", "role": "user"})
	partner := harness.signIn(t, "partner", "partner-password")
	id := harness.accountNamed(t, admin, "partner").ID
	path := fmt.Sprintf("/users/%d", id)
	if page := harness.personPage(t, admin, path); page.Body.ResetLinkExpiresAt != 0 {
		t.Errorf("before any reset link, the page says one ends at %d; want none", page.Body.ResetLinkExpiresAt)
	}
	first := harness.issueResetLink(t, admin, path)
	second := harness.issueResetLink(t, admin, path)
	read := harness.do(t, http.MethodGet, path, admin, nil)
	if strings.Contains(read.body, first.Secret) || strings.Contains(read.body, second.Secret) {
		t.Errorf("the person's page shows a reset link: %s", read.body)
	}
	if page := harness.personPage(t, admin, path); page.Body.ResetLinkExpiresAt != second.ExpiresAt {
		t.Errorf("the page says the reset link ends at %d; want the new one's %d",
			page.Body.ResetLinkExpiresAt, second.ExpiresAt)
	}
	wantError(t, harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": first.Secret, "password": "taken over", "device": testDevice,
	}), http.StatusUnauthorized, api.CodeAuthRequired)
	wantError(t, harness.do(t, http.MethodDelete, path+"/registration-invitation", partner, nil),
		http.StatusForbidden, api.CodeForbidden)
	if cancelled := harness.do(t, http.MethodDelete, path+"/registration-invitation", admin, nil); cancelled.status !=
		http.StatusNoContent {
		t.Fatalf("DELETE %s/registration-invitation = %d %s", path, cancelled.status, cancelled.body)
	}
	if page := harness.personPage(t, admin, path); page.Body.ResetLinkExpiresAt != 0 {
		t.Errorf("after the reset link was cancelled, the page says it ends at %d; want none",
			page.Body.ResetLinkExpiresAt)
	}
	wantError(t, harness.do(t, http.MethodDelete, "/users/99/registration-invitation", admin, nil),
		http.StatusNotFound, api.CodeNotFound)
}

// issueResetLink is POST path/registration-invitation, as the admin.
func (harness testHarness) issueResetLink(t *testing.T, admin, path string) api.IssuedSecretBody {
	t.Helper()
	made := harness.do(t, http.MethodPost, path+"/registration-invitation", admin, nil)
	var issued api.IssuedSecretBody
	if err := json.Unmarshal([]byte(made.body), &issued); err != nil || made.status != http.StatusOK {
		t.Fatalf("POST %s/registration-invitation = %d %s", path, made.status, made.body)
	}
	return issued
}

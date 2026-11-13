// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

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

// The page a link opens asks, without using it, what the link is for: a reset link says so, by its
// person, so the page asks for a new password (D466); an unknown, used, replaced, cancelled and
// expired link are answered alike, naming no one, so the page says at once that it is no good.
func TestLinkKind_AResetLinkSaysSoAndOneThatNoLongerWorksIsRefusedAlike(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.do(t, http.MethodPost, "/users", admin,
		map[string]string{"name": "spouse", "password": "spouse-password", "role": "user"})
	path := fmt.Sprintf("/users/%d", harness.accountNamed(t, admin, "spouse").ID)
	kindOf := func(secret string) response {
		return harness.do(t, http.MethodPost, "/registration/link", "", map[string]string{"secret": secret})
	}
	replaced := harness.issueResetLink(t, admin, path)
	used := harness.issueResetLink(t, admin, path)
	if kind := kindOf(used.Secret); kind.status != http.StatusOK ||
		!strings.Contains(kind.body, `"isInvitation":false,"name":"spouse","isReset":true`) {
		t.Errorf("POST /registration/link for a reset link = %d %s, want spouse's reset link", kind.status, kind.body)
	}
	invitation := harness.invite(t, admin, "user", "the newcomer")
	if kind := kindOf(invitation.Link.Secret); !strings.Contains(kind.body, `"isInvitation":true,"isReset":false`) {
		t.Errorf("POST /registration/link for an invitation = %d %s, want no reset link", kind.status, kind.body)
	}
	if registered := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": used.Secret, "password": "remembered", "device": testDevice,
	}); registered.status != http.StatusOK {
		t.Fatalf("POST /registration with the reset link = %d", registered.status)
	}
	cancelled := harness.issueResetLink(t, admin, path)
	if answer := harness.do(t, http.MethodDelete, path+"/registration-invitation", admin, nil); answer.status !=
		http.StatusNoContent {
		t.Fatalf("DELETE %s/registration-invitation = %d %s", path, answer.status, answer.body)
	}
	expired := harness.issueResetLink(t, admin, path)
	harness.clock.Advance(61 * time.Minute)
	unknown := kindOf("no-such-link")
	wantError(t, unknown, http.StatusUnauthorized, api.CodeAuthRequired)
	for name, secret := range map[string]string{
		"replaced": replaced.Secret, "used": used.Secret, "cancelled": cancelled.Secret, "expired": expired.Secret,
	} {
		if kind := kindOf(secret); kind.status != unknown.status || kind.body != unknown.body {
			t.Errorf("POST /registration/link for a %s reset link = %d %s, want an unknown link's %d %s",
				name, kind.status, kind.body, unknown.status, unknown.body)
		}
	}
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

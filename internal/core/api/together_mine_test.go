// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// mine is GET /together/mine as token.
func (harness testHarness) mine(t *testing.T, token string) MyTogetherOutput {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/together/mine", token, nil)
	var mine MyTogetherOutput
	if err := json.Unmarshal([]byte(answer.body), &mine.Body); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET /together/mine = %d %s", answer.status, answer.body)
	}
	return mine
}

// Someone who joins by a link signed in has the session on any device they sign in on, and so does
// the host; a guest who joined signed out is nobody's (D267 §2).
func TestTogether_ASessionFollowsWhoeverJoinedSignedIn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	opened := harness.openTogether(t, admin)
	made := harness.do(t, http.MethodPost, fmt.Sprintf("/together/%d/links", opened.Body.SessionID), admin,
		map[string]string{"hostNote": "my partner"})
	var link TogetherLinkOutput
	if err := json.Unmarshal([]byte(made.body), &link.Body); err != nil {
		t.Fatal(err)
	}
	joined := harness.do(t, http.MethodPost, "/together/join", partner,
		map[string]any{"secret": link.Body.Secret, "name": "Pat"})
	if joined.status != http.StatusOK {
		t.Fatalf("POST /together/join, signed in = %d %s", joined.status, joined.body)
	}
	harness.joinByLink(t, admin, opened.Body.SessionID, "a neighbour", "Sam") // signed out
	theirs := harness.mine(t, partner).Body.Sessions
	if len(theirs) != 1 || theirs[0].SessionID != opened.Body.SessionID || theirs[0].IsHost ||
		theirs[0].MembershipCredential == "" {
		t.Errorf("the partner's sessions are %+v; want the one they joined, with a credential", theirs)
	}
	hosted := harness.mine(t, admin).Body.Sessions
	if len(hosted) != 1 || !hosted[0].IsHost {
		t.Errorf("the host's sessions are %+v; want theirs, as host", hosted)
	}
	signedOut := harness.do(t, http.MethodGet, "/together/mine", "", nil)
	if signedOut.status != http.StatusUnauthorized {
		t.Errorf("GET /together/mine signed out = %d, want 401", signedOut.status)
	}
}

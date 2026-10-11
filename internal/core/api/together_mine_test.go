// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/together"
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

// The host's view of the session carries each link's secret, to pass on again after a reload, used
// or not (D267 §5).
func TestTogether_TheHostSeesEachLinkToPassOnAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	path := fmt.Sprintf("/together/%d", opened.Body.SessionID)
	made := harness.do(t, http.MethodPost, path+"/links", admin, map[string]string{"hostNote": "Grandma"})
	var link TogetherLinkOutput
	if err := json.Unmarshal([]byte(made.body), &link.Body); err != nil {
		t.Fatal(err)
	}
	read := harness.do(t, http.MethodGet, path, admin, nil)
	var details TogetherDetailsOutput
	if err := json.Unmarshal([]byte(read.body), &details.Body); err != nil || read.status != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, read.status, read.body)
	}
	if len(details.Body.Links) != 1 || details.Body.Links[0].Secret != link.Body.Secret {
		t.Errorf("the host sees links %+v; want the one made, with its secret", details.Body.Links)
	}
}

// A join whose sign-in cannot be checked, the database failing, is the server's failure, not a join
// as nobody, after which the session would not follow its person; a lapsed sign-in joins as nobody
// (D267 §2; the M4 closing audit, 2026-10-08).
func TestTogether_AJoinWhoseSignInCannotBeCheckedFails(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	made := harness.do(t, http.MethodPost, fmt.Sprintf("/together/%d/links", opened.Body.SessionID), admin,
		map[string]string{"hostNote": "my partner"})
	var link TogetherLinkOutput
	if err := json.Unmarshal([]byte(made.body), &link.Body); err != nil {
		t.Fatal(err)
	}
	lapsed := harness.do(t, http.MethodPost, "/together/join", "not-a-live-session",
		map[string]any{"secret": link.Body.Secret, "name": "Pat"})
	if lapsed.status != http.StatusOK {
		t.Fatalf("a join with a lapsed sign-in = %d %s; want it joined as nobody", lapsed.status, lapsed.body)
	}
	second := harness.do(t, http.MethodPost, fmt.Sprintf("/together/%d/links", opened.Body.SessionID), admin,
		map[string]string{"hostNote": "a neighbour"})
	if err := json.Unmarshal([]byte(second.body), &link.Body); err != nil {
		t.Fatal(err)
	}
	if err := harness.database.Close(); err != nil {
		t.Fatal(err)
	}
	failed := harness.do(t, http.MethodPost, "/together/join", admin,
		map[string]any{"secret": link.Body.Secret, "name": "Sam"})
	if failed.status != http.StatusInternalServerError {
		t.Errorf("a join whose sign-in could not be checked = %d %s; want 500", failed.status, failed.body)
	}
}

// A device whose sign-in cannot be checked, the database failing, is closed as the server's failure,
// to come again, not attached as nobody (D267 §2; the M4 closing audit, 2026-10-08).
func TestTogether_ADeviceWhoseSignInCannotBeCheckedIsClosed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	if err := harness.database.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	address := "ws" + strings.TrimPrefix(harness.httpServer.URL, "http") + togetherSocketPath
	connection, _, err := websocket.Dial(ctx, address, nil) //nolint:bodyclose // coder/websocket closes it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() }) //nolint:errcheck // closing what may be closed
	if err = wsjson.Write(ctx, connection, together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageHello, Credential: opened.Body.MembershipCredential,
		Role: together.RolePlayback, SessionToken: admin,
	}); err != nil {
		t.Fatal(err)
	}
	var message together.Outbound
	err = wsjson.Read(ctx, connection, &message)
	if status := websocket.CloseStatus(err); status != websocket.StatusInternalError {
		t.Errorf("a device whose sign-in could not be checked read %+v, %v; want closed as the server's failure",
			message, err)
	}
	harness.logs.AwaitLine(t, `"msg":"watch together: a device's sign-in could not be checked"`)
}

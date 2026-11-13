// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// shellOf is the shell a body holding who is signed in carries (D476), each part as its JSON; nil
// when it carries none.
func shellOf(t *testing.T, label string, user json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var body struct {
		Shell map[string]json.RawMessage `json:"shell"`
	}
	if err := json.Unmarshal(user, &body); err != nil {
		t.Fatalf("%s: %v in %s", label, err, user)
	}
	return body.Shell
}

// userOfSignIn is the user a sign-in's answer carries.
func userOfSignIn(t *testing.T, answer response) json.RawMessage {
	t.Helper()
	var signedIn struct {
		User json.RawMessage `json:"user"`
	}
	if err := json.Unmarshal([]byte(answer.body), &signedIn); err != nil || signedIn.User == nil {
		t.Fatalf("a sign-in = %d %s", answer.status, answer.body)
	}
	return signedIn.User
}

// wantPart is that part says what the endpoint at path answers token, or, for libraries, its list.
func (harness testHarness) wantPart(t *testing.T, label string, part json.RawMessage, path, token string) {
	t.Helper()
	answer := harness.do(t, http.MethodGet, path, token, nil)
	if answer.status != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, answer.status, answer.body)
	}
	want := json.RawMessage(answer.body)
	if path == "/libraries" {
		var listed struct {
			Libraries json.RawMessage `json:"libraries"`
		}
		if err := json.Unmarshal(want, &listed); err != nil {
			t.Fatal(err)
		}
		want = listed.Libraries
	}
	var got, wanted bytes.Buffer
	if err := json.Compact(&got, part); err != nil {
		t.Fatalf("%s: %v in %s", label, err, part)
	}
	if err := json.Compact(&wanted, want); err != nil {
		t.Fatal(err)
	}
	if got.String() != wanted.String() {
		t.Errorf("%s = %s; want GET %s's %s", label, got.String(), path, wanted.String())
	}
}

// The shell's one read, GET /me?shell=true, and a sign-in's answer carry what the shell read at four
// endpoints after it, so its first screen waits on one request (D476, principles §30): for an admin,
// the client settings, the libraries, what needs them and the update state, each as its endpoint
// says it. A check that the session still works (GET /me) carries none of it.
func TestMe_TheShellsReadCarriesWhatTheShellReadsApart(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelve(t)
	signedIn := harness.do(t, http.MethodPost, "/session", "", map[string]any{
		"name": "owner", "password": "owner-password", "device": testDevice,
	})
	admin, _ := readSignIn(t, signedIn)

	plain := json.RawMessage(harness.do(t, http.MethodGet, "/me", admin, nil).body)
	if shell := shellOf(t, "GET /me", plain); shell != nil {
		t.Errorf("GET /me carries a shell %v; want none: it checks the session, every minute", shell)
	}
	for label, user := range map[string]json.RawMessage{
		"the sign-in's user":   userOfSignIn(t, signedIn),
		"GET /me?shell=true's": json.RawMessage(harness.do(t, http.MethodGet, "/me?shell=true", admin, nil).body),
	} {
		shell := shellOf(t, label, user)
		if len(shell) != 4 {
			t.Fatalf("%s shell = %v; want its four parts", label, shell)
		}
		harness.wantPart(t, label+" client settings", shell["clientSettings"], "/client-settings", admin)
		harness.wantPart(t, label+" libraries", shell["libraries"], "/libraries", admin)
		harness.wantPart(t, label+" needs attention", shell["needsAttention"], "/needs-attention", admin)
		harness.wantPart(t, label+" update", shell["update"], "/system/update", admin)
	}
}

// Someone who does not administer is told the client settings and the libraries they reach, and
// nothing of the admin's: no counts of what needs one, no update state (D476).
func TestMe_TheShellOfSomeoneWhoDoesNotAdministerCarriesNoAdminsData(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	invitation := harness.invite(t, admin, "user", "a friend")
	registered := harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "friend", "password": "friend-password", "device": testDevice,
	})
	friend, _ := readSignIn(t, registered)

	for label, user := range map[string]json.RawMessage{
		"the registration's user": userOfSignIn(t, registered),
		"GET /me?shell=true's":    json.RawMessage(harness.do(t, http.MethodGet, "/me?shell=true", friend, nil).body),
	} {
		shell := shellOf(t, label, user)
		if _, has := shell["needsAttention"]; has {
			t.Errorf("%s shell carries needsAttention %s; want none to someone who does not administer",
				label, shell["needsAttention"])
		}
		if _, has := shell["update"]; has {
			t.Errorf("%s shell carries update %s; want none to someone who does not administer", label, shell["update"])
		}
		harness.wantPart(t, label+" client settings", shell["clientSettings"], "/client-settings", friend)
		harness.wantPart(t, label+" libraries", shell["libraries"], "/libraries", friend)
	}
}

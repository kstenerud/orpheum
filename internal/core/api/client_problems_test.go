// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// A failure a page met is logged with whose page it was, and only from a signed-in page, bounded:
// a phone's console is read by no one (staging, 2026-10-14).
func TestClientProblems_ALoggedFailureSaysWhoseAndWhereAndIsBounded(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	problem := map[string]any{"message": "TypeError: a request failed", "page": "/play/17"}
	answer := harness.do(t, http.MethodPost, "/client-problems", user, problem)
	if answer.status != http.StatusNoContent {
		t.Fatalf("POST /client-problems = %d %s", answer.status, answer.body)
	}
	harness.logs.AwaitEvent(t, "pageFailed", `"userId":1`, `"page":"/play/17"`,
		`"message":"TypeError: a request failed"`)
	answer = harness.do(t, http.MethodPost, "/client-problems", "", problem)
	if answer.status != http.StatusUnauthorized {
		t.Errorf("a page signed in to nothing reported a failure: %d", answer.status)
	}
	long := map[string]any{"message": strings.Repeat("x", 501), "page": "/"}
	answer = harness.do(t, http.MethodPost, "/client-problems", user, long)
	if answer.status != http.StatusUnprocessableEntity {
		t.Errorf("a failure longer than 500 characters was taken: %d", answer.status)
	}
}

// A page's note of what it did, not a failure, is logged at the debug level, so a remote's OK on a
// Fire TV's form says what its keys did (the owner, 2026-10-30); any other level is refused.
func TestClientProblems_APagesNoteIsLoggedAtTheDebugLevel(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	note := map[string]any{"message": "a form's OK on a checkbox: keydown Enter/13", "page": "/", "level": "debug"}
	if answer := harness.do(t, http.MethodPost, "/client-problems", user, note); answer.status != http.StatusNoContent {
		t.Fatalf("POST /client-problems = %d %s", answer.status, answer.body)
	}
	harness.logs.AwaitEvent(t, "pageNoted", `"level":"DEBUG"`, `"userId":1`,
		`"message":"a form's OK on a checkbox: keydown Enter/13"`)
	note["level"] = "error"
	if answer := harness.do(t, http.MethodPost, "/client-problems", user, note); answer.status !=
		http.StatusUnprocessableEntity {
		t.Errorf("a page's line at the level %q was taken: %d", "error", answer.status)
	}
}

// How a page chose its path to the core is logged in one line, with its reasons and WebRTC's pairs as
// its browser sees them, the selected one apart, from a signed-in page alone, bounded: a page at home
// on the carrier's hairpin said nothing of why (local-path-2026-10-27.md §4 items 1, 2).
func TestClientPaths_APagesChoiceOfPathIsLoggedWithItsReasonsAndItsPairs(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	report := map[string]any{
		"browser": "firefox", "isLanNameUsed": false, "localNetwork": "denied", "directOffered": 1, "lanOffered": 1,
		"reached": "webrtc",
		"names": []map[string]any{
			{
				"name":    "https://192-168-1-199.k7xq2.example:8443",
				"outcome": "left out: the browser refuses LAN names",
			},
		},
		"webrtcPairs": []map[string]any{
			{
				"state": "succeeded", "browser": "srflx udp 11.22.33.144:51234",
				"core": "srflx udp 11.22.33.144:40001", "isSelected": true,
			},
			{"state": "failed", "browser": "host udp 3f2a.local:51233", "core": "host udp 192.168.1.199:8443"},
		},
	}
	answer := harness.do(t, http.MethodPost, "/client-paths", user, report)
	if answer.status != http.StatusNoContent {
		t.Fatalf("POST /client-paths = %d %s", answer.status, answer.body)
	}
	harness.logs.AwaitEvent(t, "pagePathChosen", `"userId":1`, `"browser":"firefox"`,
		`"isLanNameUsed":false`, `"localNetwork":"denied"`, `"directOffered":1`, `"lanOffered":1`,
		`"names":"https://192-168-1-199.k7xq2.example:8443 left out: the browser refuses LAN names"`,
		`"reached":"webrtc"`,
		`"selectedPair":"succeeded srflx udp 11.22.33.144:51234 -> srflx udp 11.22.33.144:40001"`,
		`"otherPairs":"failed host udp 3f2a.local:51233 -> host udp 192.168.1.199:8443"`)
	if answer = harness.do(t, http.MethodPost, "/client-paths", "", report); answer.status != http.StatusUnauthorized {
		t.Errorf("a page signed in to nothing reported its path: %d", answer.status)
	}
	report["reached"] = strings.Repeat("x", 301)
	if answer = harness.do(t, http.MethodPost, "/client-paths", user, report); answer.status !=
		http.StatusUnprocessableEntity {
		t.Errorf("a path's name longer than 300 characters was taken: %d", answer.status)
	}
}

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
	harness.logs.AwaitLine(t, `"msg":"a page reported a failure"`, `"userId":1`, `"page":"/play/17"`,
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

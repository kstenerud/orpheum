// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A request body is read strictly (D456): a key said twice is refused before any handler runs,
// where encoding/json would have let the last one win and the sign-in go ahead.
func TestRequestBody_AKeySaidTwiceIsRefused(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	body := json.RawMessage(`{"name":"nobody","name":"owner","password":"owner-password",` +
		`"device":{"name":"Firefox on Linux","platform":"web"}}`)
	answer := harness.do(t, http.MethodPost, "/session", "", body)
	if answer.status != http.StatusBadRequest || !strings.Contains(answer.body, "duplicate key") {
		t.Errorf("POST /session with the name said twice = %d %s; want 400 naming the duplicate key",
			answer.status, answer.body)
	}
}

// A key naming a field in another case is refused (D456 §8), never read into the field.
func TestRequestBody_AKeyNamingAFieldInAnotherCaseIsRefused(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	body := json.RawMessage(`{"name":"nobody","Name":"owner","password":"owner-password",` +
		`"device":{"name":"Firefox on Linux","platform":"web"}}`)
	answer := harness.do(t, http.MethodPost, "/session", "", body)
	if answer.status != http.StatusUnprocessableEntity && answer.status != http.StatusBadRequest {
		t.Errorf("POST /session with the name in another case = %d %s; want it refused",
			answer.status, answer.body)
	}
}

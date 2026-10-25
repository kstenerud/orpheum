// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// What a lab page measured on a device is logged as one line, whose name the page is given to show,
// with the result whole inside it, and only from a signed-in page, bounded: a TV's console is read by
// no one, and the device tests are read from the log (tv-device-tests.md).
func TestLab_AResultIsLoggedAsOneLineNamedByItsIDAndIsBounded(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	sent := map[string]any{"probe": "device", "result": map[string]any{
		"screen": map[string]any{"width": 1920, "height": 1080}, "hasMediaSource": true,
	}}
	answer := harness.do(t, http.MethodPost, "/lab-results", user, sent)
	if answer.status != http.StatusOK {
		t.Fatalf("POST /lab-results = %d %s", answer.status, answer.body)
	}
	var named struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(answer.body), &named); err != nil || len(named.ID) != labResultIDLength {
		t.Fatalf("the answer %s names no result (%v)", answer.body, err)
	}
	harness.logs.AwaitLine(t, `"msg":"a lab page sent its results"`, `"labResultId":"`+named.ID+`"`,
		`"probe":"device"`, `"userId":1`,
		`"result":{"hasMediaSource":true,"screen":{"height":1080,"width":1920}}`)

	answer = harness.do(t, http.MethodPost, "/lab-results", "", sent)
	if answer.status != http.StatusUnauthorized {
		t.Errorf("a page signed in to nothing sent a result: %d", answer.status)
	}
	tooLarge := map[string]any{"probe": "remote", "result": map[string]any{
		"log": strings.Repeat("x", labResultBytesAtMost),
	}}
	answer = harness.do(t, http.MethodPost, "/lab-results", user, tooLarge)
	if answer.status != http.StatusRequestEntityTooLarge {
		t.Errorf("a result over %d bytes was taken: %d %s", labResultBytesAtMost, answer.status, answer.body)
	}
	unknown := map[string]any{"probe": "elsewhere", "result": map[string]any{}}
	answer = harness.do(t, http.MethodPost, "/lab-results", user, unknown)
	if answer.status != http.StatusUnprocessableEntity {
		t.Errorf("a result from no lab page was taken: %d", answer.status)
	}
}

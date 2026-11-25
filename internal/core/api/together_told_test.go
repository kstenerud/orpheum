// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/live"
)

// A page of a person's that stays open learns their Watch Together part changed on another device of
// theirs, on its live connection, so it reads the part again: a session started on the phone tells
// the TV's page, and so does its end (the owner's staging walk, 2026-11-25: a page open before showed
// "Start a session" until reloaded, and "Move it here" after the session ended).
func TestTogether_APersonsOpenPagesAreToldTheirPartChanged(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitConnected()
	isAny := func(live.Outbound) bool { return true }
	hosted := harness.openTogether(t, phoneToken)
	tvSocket.await(live.MessageTogether, isAny)
	path := fmt.Sprintf("/together/%d", hosted.Body.SessionID)
	if ended := harness.do(t, http.MethodDelete, path, phoneToken, nil); ended.status != http.StatusNoContent {
		t.Fatalf("DELETE %s = %d %s", path, ended.status, ended.body)
	}
	tvSocket.await(live.MessageTogether, isAny)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

// How a cast's end reaches the devices in it: who may end it with which reason, and what its remote
// is told.

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// A remote whose hello reaches its cast just after the receiver took control back is told why, as
// one attached then was, before its socket closes as for no session: it says the receiver took it
// back, not only that the cast is over (the browser tier, 2026-10-29).
func TestCast_ARemoteSayingHelloJustAfterItsCastEndedIsToldWhy(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	harness.dialLive(t, tvToken).awaitDevices(tv)
	cast := startedCast(t, harness.startCast(t, phoneToken, tv, 0))
	if ended := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", cast.CastID), tvToken,
		map[string]string{"reason": "takenBack"}); ended.status != http.StatusOK {
		t.Fatalf("the TV's take-back = %d %s", ended.status, ended.body)
	}
	late := harness.dialCast(t, cast.MembershipCredential, together.RolePanel)
	if reason := late.awaitEnded(); reason != together.CastTakenBack {
		t.Errorf("the late remote was told %q, want %q", reason, together.CastTakenBack)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var message together.Outbound
	err := wsjson.Read(ctx, late.connection, &message)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("after the end, the late remote read %+v, %v; want closed for policy", message, err)
	}
}

// A receiver asks before it plays (D358): it is handed the film's name to ask with, and its Not now,
// or a minute with no answer, ends the cast, its sender told which, so the sender, still playing, can
// say so (§1, §3, §4).
func TestCast_ADeclinedOrUnansweredCastTellsItsSenderWhy(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.fileFilmAsTitle(t, "Metropolis")
	tvToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	for _, reason := range []string{together.CastDeclined, together.CastUnanswered} {
		started := startedCast(t, harness.startCast(t, phoneToken, tv, 0))
		received := tvSocket.await(live.MessageReceive, func(each live.Outbound) bool {
			return each.Receive.CastID == started.CastID
		}).Receive
		if received.ItemName != "Metropolis" {
			t.Fatalf("the TV was handed the film named %q; want Metropolis, to ask with", received.ItemName)
		}
		sender := harness.dialCast(t, started.MembershipCredential, together.RolePanel)
		sender.awaitWelcome()
		ended := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", started.CastID), tvToken,
			map[string]string{"reason": reason})
		if ended.status != http.StatusOK {
			t.Fatalf("the TV's %s = %d %s", reason, ended.status, ended.body)
		}
		if told := sender.awaitEnded(); told != reason {
			t.Errorf("the sender was told %q, want %q", told, reason)
		}
	}
}

// No sender ends a cast as its receiver gone, its Google Cast session lost or its receiver unheard:
// a castee unheard is the server's end alone, by the cast's own deadline (D555 §3), and the cast
// goes on, its remote told nothing.
func TestCast_NoSenderEndsACastAsItsReceiverGone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	harness.dialLive(t, tvToken).awaitDevices(tv)
	started := startedCast(t, harness.startCast(t, phoneToken, tv, 0))
	ended := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", started.CastID), phoneToken,
		map[string]string{"reason": together.CastReceiverGone})
	wantError(t, ended, http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	stopped := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", started.CastID), phoneToken,
		map[string]string{"reason": together.CastStopped})
	if stopped.status != http.StatusOK {
		t.Errorf("the phone's Stop after the refused end = %d %s; want the cast on till then",
			stopped.status, stopped.body)
	}
}

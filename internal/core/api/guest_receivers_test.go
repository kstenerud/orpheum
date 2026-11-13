// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

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

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/live"
)

// guestGranted is a guest receiver's grant the device token signed in on asked for, its answer read.
func (harness testHarness) guestGranted(t *testing.T, token string) string {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/me/casts/guest-grants", token,
		map[string]any{"receiverName": "Living room TV"})
	var granted api.GuestGrantOutput
	if err := json.Unmarshal([]byte(answer.body), &granted.Body); err != nil ||
		answer.status != http.StatusCreated || granted.Body.Grant == "" {
		t.Fatalf("POST /me/casts/guest-grants = %d %s", answer.status, answer.body)
	}
	return granted.Body.Grant
}

// tradeGuestGrant trades grant at POST /guest-receivers, as a Google Cast device does.
func (harness testHarness) tradeGuestGrant(t *testing.T, grant string) response {
	t.Helper()
	return harness.do(t, http.MethodPost, "/guest-receivers", "", map[string]any{"grant": grant})
}

// tradedGuest is the guest receiver's session a 201 trade answered.
func tradedGuest(t *testing.T, answer response) api.TradeGuestGrantOutput {
	t.Helper()
	var traded api.TradeGuestGrantOutput
	if err := json.Unmarshal([]byte(answer.body), &traded.Body); err != nil ||
		answer.status != http.StatusCreated || traded.Body.Token == "" || traded.Body.DeviceID == 0 {
		t.Fatalf("POST /guest-receivers = %d %s", answer.status, answer.body)
	}
	return traded
}

// A Google Cast device signs in with a grant its sender asked for, traded once; it is no device of its
// person's to list, and only its sender casts to it, once (D465 §1, §2). The grant is in no log.
func TestGuestReceiver_IsSignedInByAGrantTradedOnceAndCastToByItsSenderAlone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	laptopToken := harness.signIn(t, "owner", "owner-password")
	phone := harness.thisDevice(t, phoneToken)
	grant := harness.guestGranted(t, phoneToken)
	guest := tradedGuest(t, harness.tradeGuestGrant(t, grant)).Body
	wantError(t, harness.tradeGuestGrant(t, grant), http.StatusNotFound, api.CodeNotFound)
	harness.logs.AwaitEvent(t, "guestReceiverSignedIn", fmt.Sprintf(`"deviceId":%d`, guest.DeviceID))
	if logged := harness.logs.String(); strings.Contains(logged, grant) {
		t.Errorf("the grant was logged:\n%s", logged)
	}
	guestSocket := harness.dialLive(t, guest.Token)
	guestSocket.awaitConnected()
	phoneSocket := harness.dialLive(t, phoneToken)
	phoneSocket.awaitDevices(phone)
	var devices api.DevicesOutput
	listed := harness.do(t, http.MethodGet, "/me/devices", phoneToken, nil)
	if err := json.Unmarshal([]byte(listed.body), &devices.Body); err != nil {
		t.Fatal(err)
	}
	for _, device := range devices.Body.Devices {
		if device.ID == guest.DeviceID {
			t.Errorf("GET /me/devices lists the guest receiver: %s", listed.body)
		}
	}
	wantError(t, harness.startCast(t, laptopToken, guest.DeviceID, 0), http.StatusConflict, api.CodeConflict)
	started := startedCast(t, harness.startCast(t, phoneToken, guest.DeviceID, 0))
	received := guestSocket.await(live.MessageReceive, func(each live.Outbound) bool {
		return each.Receive.CastID == started.CastID
	}).Receive
	if received.FileID != 1 {
		t.Errorf("the guest receiver was handed %+v; want file 1", received)
	}
	ended := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", started.CastID), phoneToken,
		map[string]any{"reason": "stopped"})
	if ended.status != http.StatusOK && ended.status != http.StatusNoContent {
		t.Fatalf("the cast's end = %d %s", ended.status, ended.body)
	}
	wantError(t, harness.startCast(t, phoneToken, guest.DeviceID, 0), http.StatusConflict, api.CodeConflict)
}

// A guest receiver's session plays its cast's film alone, and reaches no route of a person's; it
// sends no grant of its own (D465 §2).
func TestGuestReceiver_ReachesItsCastAlone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	guest := tradedGuest(t, harness.tradeGuestGrant(t, harness.guestGranted(t, phoneToken))).Body
	wantError(t, harness.do(t, http.MethodGet, "/me/devices", guest.Token, nil), http.StatusForbidden,
		api.CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, "/me/casts/guest-grants", guest.Token,
		map[string]any{"receiverName": "Another TV"}), http.StatusForbidden, api.CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, "/playback", guest.Token, chrome(1)),
		http.StatusForbidden, api.CodeForbidden)
	guestSocket := harness.dialLive(t, guest.Token)
	guestSocket.awaitConnected()
	phoneSocket := harness.dialLive(t, phoneToken)
	phoneSocket.awaitDevices(harness.thisDevice(t, phoneToken))
	started := startedCast(t, harness.startCast(t, phoneToken, guest.DeviceID, 0))
	guestSocket.await(live.MessageReceive, func(each live.Outbound) bool {
		return each.Receive.CastID == started.CastID
	})
	harness.play(t, guest.Token, chrome(1))
	wantError(t, harness.do(t, http.MethodPost, "/playback", guest.Token, chrome(2)),
		http.StatusForbidden, api.CodeForbidden)
}

// A guest receiver ends its own cast, Stop casting here on its page (D469 §3), and saying where it
// stood, but not another of its person's casts, which goes on (D465 §2).
func TestGuestReceiver_EndsItsOwnCastAloneAsAReceiverDoes(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	laptopToken := harness.signIn(t, "owner", "owner-password")
	ours := tradedGuest(t, harness.tradeGuestGrant(t, harness.guestGranted(t, phoneToken))).Body
	theirs := tradedGuest(t, harness.tradeGuestGrant(t, harness.guestGranted(t, laptopToken))).Body
	ourSocket := harness.dialLive(t, ours.Token)
	ourSocket.awaitConnected()
	theirSocket := harness.dialLive(t, theirs.Token)
	theirSocket.awaitConnected()
	phoneSocket := harness.dialLive(t, phoneToken)
	phoneSocket.awaitDevices(harness.thisDevice(t, phoneToken))
	ourCast := startedCast(t, harness.startCast(t, phoneToken, ours.DeviceID, 0))
	theirCast := startedCast(t, harness.startCast(t, laptopToken, theirs.DeviceID, 0))
	takenBack := map[string]string{"reason": "takenBack"}
	wantError(t, harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", theirCast.CastID), ours.Token,
		takenBack), http.StatusNotFound, api.CodeNotFound)
	ended := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", ourCast.CastID), ours.Token, takenBack)
	if ended.status != http.StatusOK || !strings.Contains(ended.body, `"positionMs":`) {
		t.Errorf("the guest receiver's Stop casting here = %d %s; want where it stood", ended.status, ended.body)
	}
	if again := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", theirCast.CastID), theirs.Token,
		takenBack); again.status != http.StatusOK {
		t.Errorf("the other guest receiver's own end = %d %s; want its cast still on, ended by it", again.status,
			again.body)
	}
}

// A guest receiver plays its cast's film by its own session: it fetches the film's bytes, and
// reports where it plays, as a paired TV does (D465 §2).
func TestGuestReceiver_FetchesAndReportsOnItsCastsFilm(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	guest := tradedGuest(t, harness.tradeGuestGrant(t, harness.guestGranted(t, phoneToken))).Body
	guestSocket := harness.dialLive(t, guest.Token)
	guestSocket.awaitConnected()
	phoneSocket := harness.dialLive(t, phoneToken)
	phoneSocket.awaitDevices(harness.thisDevice(t, phoneToken))
	started := startedCast(t, harness.startCast(t, phoneToken, guest.DeviceID, 0))
	guestSocket.await(live.MessageReceive, func(each live.Outbound) bool {
		return each.Receive.CastID == started.CastID
	})
	decided := harness.play(t, guest.Token, chrome(1))
	read := harness.getBytePath(t, decided.URL, guest.Token, false, "bytes=0-3")
	if read.status != http.StatusPartialContent {
		t.Errorf("the guest receiver's read of its film = %d %s; want 206", read.status, read.body)
	}
	reported := harness.report(t, guest.Token, decided, reportBody(1, 60_000, 0))
	if reported.status != http.StatusNoContent && reported.status != http.StatusOK {
		t.Errorf("the guest receiver's report = %d %s; want it taken", reported.status, reported.body)
	}
}

// A guest receiver the core ended, its cast never come in time, is refused a live connection,
// though its session has not yet been removed with its device (D465 §1).
func TestGuestReceiver_EndedIsRefusedItsLiveConnection(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	guest := tradedGuest(t, harness.tradeGuestGrant(t, harness.guestGranted(t, phoneToken))).Body
	harness.clock.Advance(live.UnansweredAfter)
	if ended := harness.live.EndGuests(); len(ended) != 1 || ended[0].DeviceID != guest.DeviceID {
		t.Fatalf("EndGuests = %+v; want the guest receiver %d", ended, guest.DeviceID)
	}
	refused := harness.dialLive(t, guest.Token)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var message live.Outbound
	err := wsjson.Read(ctx, refused.connection, &message)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("an ended guest receiver's live hello read %+v, %v; want closed for policy", message, err)
	}
}

// A person who scanned a device's code casts to it instead of activating it (D469 §1, §2): the device
// takes its session as an activated one does, through the paid service's hand-off (D321), is told it
// is a guest receiver and nothing of the person, is listed nowhere, and takes one cast from the asking
// device alone.
func TestGuestReceiver_ByCodeIsCastToByTheDeviceThatAnsweredTheCodeAlone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	laptopToken := harness.signIn(t, "owner", "owner-password")
	phone := harness.thisDevice(t, phoneToken)
	codes, keyHash := newScannedServiceCodes(t, harness, "Tizen")
	code := map[string]any{"code": "abcd-efgh"}
	wantError(t, harness.do(t, http.MethodPost, "/pairings/cast", phoneToken, code), http.StatusNotFound,
		api.CodeNotFound)
	if entered := harness.do(t, http.MethodPost, "/pairings/code", phoneToken,
		map[string]any{"code": "abcd-efgh", "keyHash": keyHash, "deviceName": "Upstairs TV"}); entered.status !=
		http.StatusOK {
		t.Fatalf("POST /pairings/code = %d %s", entered.status, entered.body)
	}
	answer := harness.do(t, http.MethodPost, "/pairings/cast", phoneToken, code)
	var made api.CastToPairingOutput
	if err := json.Unmarshal([]byte(answer.body), &made.Body); err != nil || answer.status != http.StatusCreated ||
		made.Body.DeviceName != "Upstairs TV" {
		t.Fatalf("POST /pairings/cast = %d %s", answer.status, answer.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/pairings/cast", phoneToken, code), http.StatusNotFound,
		api.CodeNotFound)
	harness.logs.AwaitEvent(t, "guestReceiverByCode", fmt.Sprintf(`"deviceId":%d`, made.Body.DeviceID))
	if len(codes.handOffs) != 1 {
		t.Fatalf("the service was handed %v; want the guest receiver's one hand-off, sealed to the TV", codes.handOffs)
	}
	session := harness.do(t, http.MethodPost, "/session/redeem", "",
		map[string]string{"code": codes.handOffs[0]})
	guestToken, name := readSignIn(t, session)
	if name != "" || !strings.Contains(session.body, `"isGuestReceiver":true`) {
		t.Errorf("the device's session = %s; want a guest receiver's, nothing of the person", session.body)
	}
	if me := harness.do(t, http.MethodGet, "/me", guestToken, nil); me.status != http.StatusOK ||
		!strings.Contains(me.body, `"isGuestReceiver":true`) || strings.Contains(me.body, "owner") {
		t.Errorf("GET /me for the guest = %d %s; want a guest receiver's, nothing of the person", me.status, me.body)
	}
	listed := harness.do(t, http.MethodGet, "/me/devices", phoneToken, nil)
	if strings.Contains(listed.body, "Upstairs TV") {
		t.Errorf("GET /me/devices lists the guest receiver: %s", listed.body)
	}
	// A route of the person's is refused it in words that fit any device, this one no Google Cast's.
	refused := harness.do(t, http.MethodGet, "/me/devices", guestToken, nil)
	wantError(t, refused, http.StatusForbidden, api.CodeForbidden)
	if strings.Contains(refused.body, "Google Cast") {
		t.Errorf("GET /me/devices for a guest by code = %s; want words that name no kind of device", refused.body)
	}
	guestSocket := harness.dialLive(t, guestToken)
	guestSocket.awaitConnected()
	phoneSocket := harness.dialLive(t, phoneToken)
	phoneSocket.awaitDevices(phone)
	wantError(t, harness.startCast(t, laptopToken, made.Body.DeviceID, 0), http.StatusConflict, api.CodeConflict)
	started := startedCast(t, harness.startCast(t, phoneToken, made.Body.DeviceID, 0))
	guestSocket.await(live.MessageReceive, func(each live.Outbound) bool {
		return each.Receive.CastID == started.CastID
	})
}

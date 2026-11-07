// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// liveSocket is one page's live connection, in a test.
type liveSocket struct {
	t          *testing.T
	connection *websocket.Conn
}

// dialLive opens a page's live connection, signed in by token, its tab in front: the session
// socket's address, with a live hello.
func (harness testHarness) dialLive(t *testing.T, token string) liveSocket {
	t.Helper()
	return harness.dialLiveSaying(t, live.Inbound{
		Type: live.MessageHello, Channel: live.ChannelLive, SessionToken: token,
	})
}

// dialLiveSaying opens a page's live connection with hello.
func (harness testHarness) dialLiveSaying(t *testing.T, hello live.Inbound) liveSocket {
	t.Helper()
	return harness.dialLiveAt(t, api.SocketPath, hello)
}

// dialLiveAt opens a page's live connection at path with hello.
func (harness testHarness) dialLiveAt(t *testing.T, path string, hello any) liveSocket {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	address := "ws" + strings.TrimPrefix(harness.httpServer.URL, "http") + path
	connection, _, err := websocket.Dial(ctx, address, nil) //nolint:bodyclose // coder/websocket closes it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() }) //nolint:errcheck // closing what may be closed
	if err := wsjson.Write(ctx, connection, hello); err != nil {
		t.Fatal(err)
	}
	return liveSocket{t: t, connection: connection}
}

// await reads until a message of kind that satisfies isIt, which fails the test when none comes.
func (opened liveSocket) await(kind live.MessageType, isIt func(live.Outbound) bool) live.Outbound {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	for {
		var message live.Outbound
		if err := wsjson.Read(ctx, opened.connection, &message); err != nil {
			opened.t.Fatalf("no %s message as wanted: %v", kind, err)
		}
		if message.Type == kind && isIt(message) {
			return message
		}
	}
}

// keepAlive pings every period until the test ends, as a page does.
func (opened liveSocket) keepAlive(period time.Duration) {
	ctx, cancel := context.WithCancel(opened.t.Context())
	opened.t.Cleanup(cancel)
	go func() {
		ticker := time.NewTicker(period)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ping := live.Inbound{Type: live.MessagePing}
				_ = wsjson.Write(ctx, opened.connection, ping) //nolint:errcheck // until closed
			}
		}
	}()
}

// awaitDevices reads until the person's connected devices are want.
func (opened liveSocket) awaitDevices(want ...int64) {
	opened.t.Helper()
	opened.await(live.MessageDevices, func(message live.Outbound) bool {
		return slices.Equal(message.DeviceIDs, want)
	})
}

// thisDevice is the device token signed in on.
func (harness testHarness) thisDevice(t *testing.T, token string) int64 {
	t.Helper()
	var devices api.DevicesOutput
	listed := harness.do(t, http.MethodGet, "/me/devices", token, nil)
	if err := json.Unmarshal([]byte(listed.body), &devices.Body); err != nil {
		t.Fatal(err)
	}
	for _, device := range devices.Body.Devices {
		if device.IsThisDevice {
			return device.ID
		}
	}
	t.Fatalf("GET /me/devices names no device as this one: %s", listed.body)
	return 0
}

// dialCast opens a device's session socket into a cast by credential, in role.
func (harness testHarness) dialCast(t *testing.T, credential string, role together.DeviceRole) socket {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	address := "ws" + strings.TrimPrefix(harness.httpServer.URL, "http") + api.TogetherSocketPath
	connection, _, err := websocket.Dial(ctx, address, nil) //nolint:bodyclose // coder/websocket closes it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() }) //nolint:errcheck // closing what may be closed
	opened := socket{t: t, connection: connection}
	opened.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageHello, Credential: credential, Role: role,
	})
	return opened
}

// awaitEnded reads until the device is told its session ended, and returns why.
func (opened socket) awaitEnded() string {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	for {
		var message together.Outbound
		if err := wsjson.Read(ctx, opened.connection, &message); err != nil {
			opened.t.Fatalf("not told the session ended: %v", err)
		}
		if message.Type == together.MessageEnded {
			return message.Reason
		}
	}
}

// startCast casts film 1 from the device token signed in on to toDeviceID, from positionMs.
func (harness testHarness) startCast(t *testing.T, token string, toDeviceID, positionMs int64) response {
	t.Helper()
	return harness.do(t, http.MethodPost, "/me/casts", token,
		map[string]any{"toDeviceId": toDeviceID, "fileId": 1, "positionMs": positionMs})
}

// startedCast is the cast a 201 answer started.
func startedCast(t *testing.T, answer response) api.CastBody {
	t.Helper()
	var started api.CastBody
	if err := json.Unmarshal([]byte(answer.body), &started); err != nil || answer.status != http.StatusCreated {
		t.Fatalf("POST /me/casts = %d %s", answer.status, answer.body)
	}
	return started
}

// A device is listed to its person while its live socket is open, and gone once it stays silent past
// the silence limit; a sign-in that fails is refused (D337 §2).
func TestLive_ADeviceIsListedWhileItsSocketIsOpenAndGoneAfterTheSilenceLimit(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	phone, tv := harness.thisDevice(t, phoneToken), harness.thisDevice(t, tvToken)
	harness.server.TogetherSilence().Store(int64(300 * time.Millisecond))
	phoneSocket := harness.dialLive(t, phoneToken)
	phoneSocket.keepAlive(50 * time.Millisecond)
	phoneSocket.awaitDevices(phone)
	harness.dialLive(t, tvToken) // says hello, then nothing
	phoneSocket.awaitDevices(phone, tv)
	phoneSocket.awaitDevices(phone)
	refused := harness.dialLive(t, "not-a-session")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var message live.Outbound
	err := wsjson.Read(ctx, refused.connection, &message)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("a live hello of no sign-in read %+v, %v; want closed for policy", message, err)
	}
}

// A device removed from another is told on its live socket that its sign-in ended, so its page goes
// to sign in at once, not at its person's next press (the owner's Fire TV, 2026-10-27).
func TestLive_ARemovedDeviceIsToldItsSignInEnded(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	if removed := harness.do(t, http.MethodDelete, "/me/devices/"+itoa(tv), phoneToken, nil); removed.status !=
		http.StatusNoContent && removed.status != http.StatusOK {
		t.Fatalf("DELETE /me/devices/%d = %d %s", tv, removed.status, removed.body)
	}
	tvSocket.await(live.MessageSignedOut, func(live.Outbound) bool { return true })
}

// A device casts only to another of its person's devices connected now; the receiver is handed the
// cast on its live socket and leads its session of one, and the sender commands it (D337 §1, §2, §5).
func TestCast_ADeviceCastsOnlyToAnotherOfItsPersonsConnectedNow(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	closedToken := harness.signIn(t, "owner", "owner-password")
	tv, closed := harness.thisDevice(t, tvToken), harness.thisDevice(t, closedToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	wantError(t, harness.startCast(t, phoneToken, closed, 0), http.StatusConflict, api.CodeConflict)
	wantError(t, harness.startCast(t, tvToken, tv, 0), http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	started := startedCast(t, harness.startCast(t, phoneToken, tv, 61_000))
	received := tvSocket.await(live.MessageReceive, func(live.Outbound) bool { return true }).Receive
	if received.CastID != started.CastID || received.FileID != 1 || received.PositionMs == nil ||
		*received.PositionMs != 61_000 || received.FromDeviceName != testDevice.Name {
		t.Fatalf("the TV was handed %+v; want cast %d of file 1 from 61 s, from %q",
			received, started.CastID, testDevice.Name)
	}
	receiver := harness.dialCast(t, received.Credential, together.RoleVideo)
	if welcome := receiver.awaitWelcome(); !welcome.IsHost || welcome.SessionID != started.CastID {
		t.Fatalf("the receiver's welcome = %+v; want the cast's host", welcome)
	}
	remote := harness.dialCast(t, started.MembershipCredential, together.RolePanel)
	remote.awaitWelcome()
	putOn(receiver, remote, 1)
	remote.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandPause,
	})
	receiver.awaitGroup(together.GroupPaused)
	var mine api.MyCastsOutput
	listed := harness.do(t, http.MethodGet, "/me/casts", tvToken, nil)
	if err := json.Unmarshal([]byte(listed.body), &mine.Body); err != nil || len(mine.Body.Casts) != 1 ||
		mine.Body.Casts[0].IsSender || mine.Body.Casts[0].MembershipCredential == "" {
		t.Errorf("GET /me/casts on the TV = %s; want the one it receives, with a credential", listed.body)
	}
}

// A device in the Watch Together session its person is in is said so among their devices, never a
// cast target, from the moment its page says hello signed in to the session: the owner's S23 cast to
// the Mac that hosted the session, which ended it (D384 §3, 2026-10-30).
func TestCast_ADeviceInTheCastersWatchTogetherSessionIsSaidSo(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	macToken := harness.createAdminAndSignIn(t)
	phoneToken := harness.signIn(t, "owner", "owner-password")
	mac, phone := harness.thisDevice(t, macToken), harness.thisDevice(t, phoneToken)
	inSession := func() []int64 {
		var devices api.DevicesOutput
		listed := harness.do(t, http.MethodGet, "/me/devices", phoneToken, nil)
		if err := json.Unmarshal([]byte(listed.body), &devices.Body); err != nil {
			t.Fatal(err)
		}
		var in []int64
		for _, device := range devices.Body.Devices {
			if device.IsInYourWatchTogether {
				in = append(in, device.ID)
			}
		}
		return in
	}
	opened := harness.openTogether(t, macToken)
	host := harness.dialTogether(t, opened.Body.MembershipCredential) // a hello signed in to nothing
	host.awaitWelcome()
	if in := inSession(); len(in) != 0 {
		t.Fatalf("with no device of the person's in the session signed in, devices in it = %v; want none", in)
	}
	harness.dialTogetherSignedIn(t, opened.Body.MembershipCredential, macToken).awaitWelcome()
	if in := inSession(); !slices.Equal(in, []int64{mac}) || slices.Contains(in, phone) {
		t.Errorf("with the Mac's page in the session, devices in it = %v; want the Mac's, %d", in, mac)
	}
}

// A cast goes only to a device with an Orpheum tab in front: a page that says hello behind is
// listed as connected but not in front, a cast to it is refused for the person to bring a tab to
// the front, and once the page says it came to the front the cast is handed to it (D338 §2).
func TestCast_GoesOnlyToADeviceWithATabInFront(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	phone, television := harness.thisDevice(t, phoneToken), harness.thisDevice(t, tvToken)
	phoneSocket := harness.dialLive(t, phoneToken)
	tvSocket := harness.dialLiveSaying(t, live.Inbound{
		Type: live.MessageHello, Channel: live.ChannelLive, SessionToken: tvToken, IsBehind: true,
	})
	phoneSocket.await(live.MessageDevices, func(message live.Outbound) bool {
		return slices.Equal(message.DeviceIDs, []int64{phone, television}) &&
			slices.Equal(message.InFrontDeviceIDs, []int64{phone})
	})
	wantError(t, harness.startCast(t, phoneToken, television, 0), http.StatusConflict, api.CodeConflict)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, tvSocket.connection, live.Inbound{Type: live.MessageFront}); err != nil {
		t.Fatal(err)
	}
	phoneSocket.await(live.MessageDevices, func(message live.Outbound) bool {
		return slices.Equal(message.InFrontDeviceIDs, []int64{phone, television})
	})
	started := startedCast(t, harness.startCast(t, phoneToken, television, 0))
	received := tvSocket.await(live.MessageReceive, func(live.Outbound) bool { return true }).Receive
	if received.CastID != started.CastID {
		t.Errorf("the TV's tab come to the front was handed %+v, want cast %d", received, started.CastID)
	}
}

// A device receiving a cast is no device's target until it ends: a cast to it, from another device
// or from its own sender again, is refused, receiving_a_cast, and its remote goes on; the person's
// devices are told it receives. The receiver taking control back ends the cast for the remote, told
// so, and says where it stood; the device may then be cast to again (D386 §1, D337 §4).
func TestCast_ADeviceReceivingIsNoOnesTargetUntilItTakesControlBack(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	laptopToken := harness.signIn(t, "owner", "owner-password")
	television := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(television)
	first := startedCast(t, harness.startCast(t, phoneToken, television, 0))
	tvSocket.await(live.MessageDevices, func(message live.Outbound) bool {
		return slices.Equal(message.ReceivingDeviceIDs, []int64{television})
	})
	phoneRemote := harness.dialCast(t, first.MembershipCredential, together.RolePanel)
	phoneRemote.awaitWelcome()
	wantError(t, harness.startCast(t, laptopToken, television, 5_000), http.StatusConflict, api.CodeReceivingACast)
	wantError(t, harness.startCast(t, phoneToken, television, 5_000), http.StatusConflict, api.CodeReceivingACast)
	wantError(t, harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", first.CastID), laptopToken,
		map[string]string{"reason": "stopped"}), http.StatusNotFound, api.CodeNotFound)
	ended := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", first.CastID), tvToken,
		map[string]string{"reason": "takenBack"})
	if ended.status != http.StatusOK || !strings.Contains(ended.body, `"positionMs":`) {
		t.Errorf("the TV's take-back = %d %s; want where it stood", ended.status, ended.body)
	}
	if reason := phoneRemote.awaitEnded(); reason != together.CastTakenBack {
		t.Errorf("the phone's remote was told %q, want %q", reason, together.CastTakenBack)
	}
	startedCast(t, harness.startCast(t, laptopToken, television, 5_000))
}

// A receiver's cast back to the device casting to it is refused, casting_here, and the cast goes on:
// each device is not left believing it casts to the other (the owner's 11-20 batch, finding 6).
func TestCast_AReceiversCastBackToItsSenderIsRefused(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	television := harness.thisDevice(t, tvToken)
	phone := harness.thisDevice(t, phoneToken)
	harness.dialLive(t, phoneToken).awaitDevices(phone)
	harness.dialLive(t, tvToken).awaitDevices(phone, television)
	first := startedCast(t, harness.startCast(t, phoneToken, television, 0))
	wantError(t, harness.startCast(t, tvToken, phone, 0), http.StatusConflict, api.CodeCastingHere)
	listed := harness.do(t, http.MethodGet, "/me/casts", phoneToken, nil)
	if !strings.Contains(listed.body, fmt.Sprintf(`"castId":%d`, first.CastID)) ||
		strings.Count(listed.body, `"castId"`) != 1 {
		t.Errorf("the phone's casts = %s; want its cast to the TV alone, going on", listed.body)
	}
}

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

// sessionWithNothingOn signs a phone and a TV in, both pages' live sockets open, and starts casting
// from the phone to the TV with nothing playing, the phone's page told it sends it and the TV handed
// it; it returns the cast, the two tokens and the two live sockets.
func (harness testHarness) sessionWithNothingOn(t *testing.T) (api.CastBody, [2]string, [2]liveSocket) {
	t.Helper()
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.fileFilmAsTitle(t, "Metropolis")
	tvToken := harness.signIn(t, "owner", "owner-password")
	television := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(television)
	phoneSocket := harness.dialLive(t, phoneToken)
	started := harness.do(t, http.MethodPost, "/me/casts", phoneToken, map[string]any{"toDeviceId": television})
	session := startedCast(t, started)
	if session.FileID != 0 || session.IsAccepted {
		t.Fatalf("the session started as %+v; want nothing playing, not yet accepted", session)
	}
	phoneSocket.await(live.MessageSending, func(message live.Outbound) bool {
		return message.Sending != nil && message.Sending.CastID == session.CastID &&
			message.Sending.ReceiverDeviceID == television
	})
	tvSocket.await(live.MessageReceive, func(message live.Outbound) bool { return message.Receive.FileID == 0 })
	return session, [2]string{phoneToken, tvToken}, [2]liveSocket{phoneSocket, tvSocket}
}

// Casting is a session with a device (D364): started with nothing playing, the sender's page told it
// sends it; the receiver alone says Use, which the sender is told (§1, §2, §6). A refusal is logged.
func TestCast_IsASessionOnlyItsReceiverAccepts(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	session, tokens, sockets := harness.sessionWithNothingOn(t)
	acceptPath := fmt.Sprintf("/me/casts/%d/accept", session.CastID)
	wantError(t, harness.do(t, http.MethodPost, acceptPath, tokens[0], nil), http.StatusNotFound, api.CodeNotFound)
	// A refusal is logged, so a request refused can be told from one never sent (principles §28).
	harness.logs.AwaitEvent(t, "castRequestRefused", `"request":"accept"`,
		fmt.Sprintf(`"castId":%d`, session.CastID), `"why":"no such cast"`)
	if accepted := harness.do(t, http.MethodPost, acceptPath, tokens[1], nil); accepted.status != http.StatusOK {
		t.Fatalf("the TV's Use = %d %s", accepted.status, accepted.body)
	}
	sockets[0].await(live.MessageSending, func(message live.Outbound) bool {
		return message.Sending != nil && message.Sending.IsAccepted
	})
}

// Every film the sender plays while it casts is handed to the receiver in the same cast, its name with
// it, and the ready state between videos stops the cast's group, so the next is put on afresh; a file
// the person does not reach is refused (D364 §3, §5).
func TestCast_IsASessionInWhichEachFilmPlayedGoesToTheReceiver(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	session, tokens, sockets := harness.sessionWithNothingOn(t)
	playPath := fmt.Sprintf("/me/casts/%d/play", session.CastID)
	if played := harness.do(t, http.MethodPost, playPath, tokens[0],
		map[string]any{"fileId": 1, "positionMs": 7_000}); played.status != http.StatusOK {
		t.Fatalf("POST %s = %d %s", playPath, played.status, played.body)
	}
	handed := sockets[1].await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.FileID == 1
	}).Receive
	if handed.CastID != session.CastID || handed.ItemName != "Metropolis" || handed.PositionMs == nil ||
		*handed.PositionMs != 7_000 {
		t.Errorf("the TV was handed %+v; want the same cast, of Metropolis from 7 s", handed)
	}
	sockets[0].await(live.MessageSending, func(message live.Outbound) bool {
		return message.Sending != nil && message.Sending.FileID == 1 && message.Sending.ItemName == "Metropolis"
	})
	receiver := harness.dialCast(t, handed.Credential, together.RoleVideo)
	receiver.awaitWelcome()
	remote := harness.dialCast(t, session.MembershipCredential, together.RolePanel)
	remote.awaitWelcome()
	putOn(receiver, remote, 1)
	if ready := harness.do(t, http.MethodPost, playPath, tokens[0], map[string]any{}); ready.status != http.StatusOK {
		t.Fatalf("POST %s with nothing = %d %s", playPath, ready.status, ready.body)
	}
	if idle := remote.awaitGroup(together.GroupIdle); idle.Group.FileID != 0 {
		t.Errorf("ready, the cast's group is %+v; want nothing on", idle.Group)
	}
	sockets[1].await(live.MessageReceive, func(message live.Outbound) bool { return message.Receive.FileID == 0 })
	wantError(t, harness.do(t, http.MethodPost, playPath, tokens[1], map[string]any{"fileId": 999}),
		http.StatusNotFound, api.CodeNotFound)
}

// A person who hosts Watch Together and casts has their receiver handed a credential for their
// session as well, to play it as their player, so what they play plays for the group (D364 §11).
func TestCast_AHostsReceiverIsHandedTheirWatchTogetherSession(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	hosted := harness.openTogether(t, phoneToken)
	startedCast(t, harness.do(t, http.MethodPost, "/me/casts", phoneToken, map[string]any{"toDeviceId": tv}))
	handed := tvSocket.await(live.MessageReceive, func(live.Outbound) bool { return true }).Receive
	if handed.TogetherCredential == "" {
		t.Fatal("the host's TV was handed no credential for their Watch Together session")
	}
	player := harness.dialCast(t, handed.TogetherCredential, together.RoleVideo)
	if welcome := player.awaitWelcome(); !welcome.IsHost || welcome.SessionID != hosted.Body.SessionID {
		t.Errorf("the TV's welcome = %+v; want the host's, in session %d", welcome, hosted.Body.SessionID)
	}
}

// A cast from a page that holds a guest's membership of the session hands its receiver that guest's
// part, following the group, not the host's: the person's own phone, joined as a guest of the session
// they host, casts as its guest (VQ193).
func TestCast_AReceiverTakesTheGuestsPartTheSendingPageHolds(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	hosted := harness.openTogether(t, phoneToken)
	guest := harness.joinByLink(t, phoneToken, hosted.Body.SessionID, "my phone", "Phone")
	startedCast(t, harness.do(t, http.MethodPost, "/me/casts", phoneToken, map[string]any{
		"toDeviceId": tv, "togetherCredential": guest.Body.MembershipCredential,
	}))
	handed := tvSocket.await(live.MessageReceive, func(live.Outbound) bool { return true }).Receive
	if handed.TogetherCredential == "" || !handed.IsTogetherFollower {
		t.Fatalf("the TV was handed %+v; want the guest's part, following", handed)
	}
	player := harness.dialCast(t, handed.TogetherCredential, together.RoleVideo)
	if welcome := player.awaitWelcome(); welcome.IsHost || welcome.MemberID != guest.Body.MemberID {
		t.Errorf("the TV's welcome = %+v; want the guest's, member %d", welcome, guest.Body.MemberID)
	}
}

// A Watch Together session's end ends every cast whose receiver takes a part in it (D389 §1): a
// guest's cast of the group's film, its receiver following in the guest's part, ends as its host ends
// the session, the receiver's cast session told sessionEnded, so it goes home; the remote is told
// too, the person's devices that it receives no more, and the sender lists no cast. The owner's
// staging, 2026-10-30: the S23, a guest casting to the Fire TV, went home as the host ended the
// session, and the TV played on under a cast no page controlled (VQ193).
func TestCast_ASessionsEndEndsTheCastsThatTakeAPartInIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	hosted := harness.openTogether(t, phoneToken)
	guest := harness.joinByLink(t, phoneToken, hosted.Body.SessionID, "my phone", "Phone")
	cast := startedCast(t, harness.do(t, http.MethodPost, "/me/casts", phoneToken, map[string]any{
		"toDeviceId": tv, "togetherCredential": guest.Body.MembershipCredential,
	}))
	handed := tvSocket.await(live.MessageReceive, func(live.Outbound) bool { return true }).Receive
	receiver := harness.dialCast(t, handed.Credential, together.RoleVideo)
	receiver.awaitWelcome()
	remote := harness.dialCast(t, cast.MembershipCredential, together.RolePanel)
	remote.awaitWelcome()
	path := fmt.Sprintf("/together/%d", hosted.Body.SessionID)
	if ended := harness.do(t, http.MethodDelete, path, phoneToken, nil); ended.status != http.StatusNoContent {
		t.Fatalf("DELETE %s = %d %s", path, ended.status, ended.body)
	}
	if reason := receiver.awaitEnded(); reason != together.CastSessionEnded {
		t.Errorf("the TV's cast was told %q, want %q", reason, together.CastSessionEnded)
	}
	if reason := remote.awaitEnded(); reason != together.CastSessionEnded {
		t.Errorf("the phone's remote was told %q, want %q", reason, together.CastSessionEnded)
	}
	tvSocket.await(live.MessageDevices, func(message live.Outbound) bool {
		return len(message.ReceivingDeviceIDs) == 0
	})
	if listed := harness.do(t, http.MethodGet, "/me/casts", phoneToken, nil); !strings.Contains(listed.body,
		`"casts":[]`) {
		t.Errorf("GET /me/casts = %d %s; want no cast", listed.status, listed.body)
	}
}

// A host who starts Watch Together while they cast has one session, not two (D364 §11, D372 §8): the
// receiver is handed the cast again with the host's part in the new session and the film from where
// the cast stands, takes it, leading it, and a guest who joins finds that film on and the receiver
// leading (D371 §4). The owner's staging, 2026-10-29: the receiver stayed in the cast's session, and
// Watch Together's page, where the casting page sent them, held a session with nothing on.
func TestCast_AHostStartingWatchTogetherWhileCastingHasTheReceiverLeadIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	session, tokens, sockets := harness.sessionWithNothingOn(t)
	playPath := fmt.Sprintf("/me/casts/%d/play", session.CastID)
	if played := harness.do(t, http.MethodPost, playPath, tokens[0],
		map[string]any{"fileId": 1, "positionMs": 7_000}); played.status != http.StatusOK {
		t.Fatalf("POST %s = %d %s", playPath, played.status, played.body)
	}
	handed := sockets[1].await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.FileID == 1
	}).Receive
	castPlayer := harness.dialCast(t, handed.Credential, together.RoleVideo)
	castPlayer.awaitWelcome()
	castPlayer.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandChangeItem,
		FileID: 1, PositionMs: 7_000,
	})
	castPlayer.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageState, Playing: true, PositionMs: 42_000, FileID: 1,
	})
	castPlayer.awaitGroup(together.GroupPlaying)
	castPlayer.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandPause,
	})
	castPlayer.awaitGroup(together.GroupPaused)
	hosted := harness.openTogether(t, tokens[0])
	part := sockets[1].await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.TogetherCredential != ""
	}).Receive
	if part.CastID != session.CastID || part.FileID != 1 || part.IsTogetherFollower || part.PositionMs == nil ||
		*part.PositionMs != 42_000 {
		t.Fatalf("the TV was handed %+v; want the same cast of film 1 from 42 s, leading", part)
	}
	player := harness.dialCast(t, part.TogetherCredential, together.RoleVideo)
	welcome := player.awaitWelcome()
	if !welcome.IsHost || welcome.SessionID != hosted.Body.SessionID {
		t.Fatalf("the TV's welcome = %+v; want the host's, in session %d", welcome, hosted.Body.SessionID)
	}
	player.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandChangeItem,
		FileID: 1, PositionMs: *part.PositionMs,
	})
	player.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageState, Playing: true, PositionMs: *part.PositionMs,
		FileID: 1,
	})
	player.awaitGroup(together.GroupPlaying)
	guest := harness.joinByLink(t, tokens[0], hosted.Body.SessionID, "my partner", "Anna")
	guestWelcome := harness.dialTogether(t, guest.Body.MembershipCredential).awaitWelcome()
	if guestWelcome.SessionID != hosted.Body.SessionID || guestWelcome.Group.FileID != 1 {
		t.Errorf("the guest's welcome = %+v; want session %d playing film 1", guestWelcome, hosted.Body.SessionID)
	}
	if !leads(guestWelcome.Roster, welcome.DeviceID) {
		t.Errorf("the guest's roster %+v; want the TV, device %d, leading", guestWelcome.Roster, welcome.DeviceID)
	}
}

// leads is whether device deviceID leads in roster.
func leads(roster []together.MemberView, deviceID int64) bool {
	for _, member := range roster {
		for _, device := range member.Devices {
			if device.ID == deviceID {
				return device.IsLeader
			}
		}
	}
	return false
}

// The session's page names what plays and how long it lasts, for its remote (D364 §15).
func TestTogether_TheHostsDetailsNameWhatPlays(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	hostToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.fileFilmAsTitle(t, "Nosferatu")
	hosted := harness.openTogether(t, hostToken)
	player := harness.dialTogether(t, hosted.Body.MembershipCredential)
	player.awaitWelcome()
	putOn(player, player, 1)
	var details api.TogetherDetailsOutput
	read := harness.do(t, http.MethodGet, fmt.Sprintf("/together/%d", hosted.Body.SessionID), hostToken, nil)
	if err := json.Unmarshal([]byte(read.body), &details.Body); err != nil || details.Body.FileID != 1 ||
		details.Body.TitleName != "Nosferatu" {
		t.Errorf("GET /together/%d = %d %s; want Nosferatu, file 1, playing", hosted.Body.SessionID, read.status, read.body)
	}
}

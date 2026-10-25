// SPDX-License-Identifier: AGPL-3.0-or-later

package api

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

	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// liveSocket is one page's live connection, in a test.
type liveSocket struct {
	t          *testing.T
	connection *websocket.Conn
}

// dialLive opens a page's live connection, signed in by token: the session socket's address, with
// a live hello.
func (harness testHarness) dialLive(t *testing.T, token string) liveSocket {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	address := "ws" + strings.TrimPrefix(harness.httpServer.URL, "http") + togetherSocketPath
	connection, _, err := websocket.Dial(ctx, address, nil) //nolint:bodyclose // coder/websocket closes it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() }) //nolint:errcheck // closing what may be closed
	if err := wsjson.Write(ctx, connection, live.Inbound{Type: live.MessageHello, SessionToken: token}); err != nil {
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
	var devices DevicesOutput
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
	address := "ws" + strings.TrimPrefix(harness.httpServer.URL, "http") + togetherSocketPath
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
func startedCast(t *testing.T, answer response) CastBody {
	t.Helper()
	var started CastBody
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
	harness.server.togetherSilence.Store(int64(300 * time.Millisecond))
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
	wantError(t, harness.startCast(t, phoneToken, closed, 0), http.StatusConflict, CodeConflict)
	wantError(t, harness.startCast(t, tvToken, tv, 0), http.StatusUnprocessableEntity, CodeInvalidRequest)
	started := startedCast(t, harness.startCast(t, phoneToken, tv, 61_000))
	received := tvSocket.await(live.MessageReceive, func(live.Outbound) bool { return true }).Receive
	if received.CastID != started.CastID || received.FileID != 1 || received.PositionMs == nil ||
		*received.PositionMs != 61_000 || received.FromDeviceName != testDevice.Name {
		t.Fatalf("the TV was handed %+v; want cast %d of file 1 from 61 s, from %q",
			received, started.CastID, testDevice.Name)
	}
	receiver := harness.dialCast(t, received.Credential, together.RolePlayback)
	if welcome := receiver.awaitWelcome(); !welcome.IsHost || welcome.SessionID != started.CastID {
		t.Fatalf("the receiver's welcome = %+v; want the cast's host", welcome)
	}
	remote := harness.dialCast(t, started.MembershipCredential, together.RoleVoice)
	remote.awaitWelcome()
	putOn(receiver, remote, 1)
	remote.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandPause,
	})
	receiver.awaitGroup(together.GroupPaused)
	var mine MyCastsOutput
	listed := harness.do(t, http.MethodGet, "/me/casts", tvToken, nil)
	if err := json.Unmarshal([]byte(listed.body), &mine.Body); err != nil || len(mine.Body.Casts) != 1 ||
		mine.Body.Casts[0].IsSender || mine.Body.Casts[0].MembershipCredential == "" {
		t.Errorf("GET /me/casts on the TV = %s; want the one it receives, with a credential", listed.body)
	}
}

// The last cast wins: a cast from another device ends the earlier one for its remote, which is told
// it was replaced; the receiver taking control back ends it for the remote, told so, and says where
// it stood (D337 §4, §6).
func TestCast_TheLastCastWinsAndTheReceiverTakesControlBack(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	laptopToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	harness.dialLive(t, tvToken).awaitDevices(tv)
	first := startedCast(t, harness.startCast(t, phoneToken, tv, 0))
	phoneRemote := harness.dialCast(t, first.MembershipCredential, together.RoleVoice)
	phoneRemote.awaitWelcome()
	second := startedCast(t, harness.startCast(t, laptopToken, tv, 5_000))
	if reason := phoneRemote.awaitEnded(); reason != together.CastReplaced {
		t.Errorf("the phone's remote was told %q, want %q", reason, together.CastReplaced)
	}
	laptopRemote := harness.dialCast(t, second.MembershipCredential, together.RoleVoice)
	laptopRemote.awaitWelcome()
	wantError(t, harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", second.CastID), phoneToken,
		map[string]string{"reason": "stopped"}), http.StatusNotFound, CodeNotFound)
	ended := harness.do(t, http.MethodPost, fmt.Sprintf("/me/casts/%d/end", second.CastID), tvToken,
		map[string]string{"reason": "takenBack"})
	if ended.status != http.StatusOK || !strings.Contains(ended.body, `"positionMs":`) {
		t.Errorf("the TV's take-back = %d %s; want where it stood", ended.status, ended.body)
	}
	if reason := laptopRemote.awaitEnded(); reason != together.CastTakenBack {
		t.Errorf("the laptop's remote was told %q, want %q", reason, together.CastTakenBack)
	}
}

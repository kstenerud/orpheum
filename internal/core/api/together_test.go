// SPDX-License-Identifier: GPL-3.0-or-later

package api

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

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// socket is one device's session WebSocket, in a test.
type socket struct {
	t          *testing.T
	connection *websocket.Conn
}

// dialTogether opens a device's session WebSocket and says hello with credential.
func (harness testHarness) dialTogether(t *testing.T, credential string, role together.DeviceRole) socket {
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
		V: together.ProtocolVersion, Type: together.MessageHello, Credential: credential,
		Role: role,
	})
	return opened
}

// send sends one message.
func (opened socket) send(message together.Inbound) {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, opened.connection, message); err != nil {
		opened.t.Fatal(err)
	}
}

// awaitGroup reads until a group message in state, which fails the test when none comes in time.
func (opened socket) awaitGroup(state together.GroupState) together.Outbound {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	for {
		var message together.Outbound
		if err := wsjson.Read(ctx, opened.connection, &message); err != nil {
			opened.t.Fatalf("no group message in state %s: %v", state, err)
		}
		if message.Type == together.MessageGroup && message.Group.State == state {
			return message
		}
	}
}

// awaitType reads until a message of kind, which fails the test when none comes in time.
func (opened socket) awaitType(kind together.MessageType) together.Outbound {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	for {
		var message together.Outbound
		if err := wsjson.Read(ctx, opened.connection, &message); err != nil {
			opened.t.Fatalf("no %s message: %v", kind, err)
		}
		if message.Type == kind {
			return message
		}
	}
}

// startTogether starts a session of film 1 as the admin, returning what it answered.
func (harness testHarness) startTogether(t *testing.T, admin string) CreateTogetherOutput {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/together", admin, map[string]any{"fileId": 1, "transport": "group"})
	var created CreateTogetherOutput
	if err := json.Unmarshal([]byte(answer.body), &created.Body); err != nil || answer.status != http.StatusCreated {
		t.Fatalf("POST /together = %d %s", answer.status, answer.body)
	}
	return created
}

// The host starts a session, a guest with no account joins by the link's secret, each device says
// hello on the session's WebSocket, and a play from the host plays for both once both are ready;
// the guest's pause pauses both (cut-a §5.1, D117, D119).
func TestTogether_AHostAndAGuestPlayAndPauseTogether(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	created := harness.startTogether(t, admin)
	joined := harness.do(t, http.MethodPost, "/together/join", "",
		map[string]any{"secret": created.Body.JoinSecret, "name": "partner"})
	var membership JoinTogetherOutput
	if err := json.Unmarshal([]byte(joined.body), &membership.Body); err != nil || joined.status != http.StatusOK {
		t.Fatalf("POST /together/join = %d %s", joined.status, joined.body)
	}
	host := harness.dialTogether(t, created.Body.MembershipCredential, together.RolePlayback)
	if welcome := host.awaitType(together.MessageWelcome); welcome.SessionID != created.Body.SessionID {
		t.Fatalf("the host's welcome = %+v", welcome)
	}
	guest := harness.dialTogether(t, membership.Body.MembershipCredential, together.RolePlayback)
	guest.awaitType(together.MessageWelcome)
	host.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand,
		Command: together.CommandPlay,
	})
	if playing := guest.awaitGroup(together.GroupPlaying); playing.Group.FileID != 1 {
		t.Errorf("the guest's group = %+v; want film 1 playing", playing.Group)
	}
	guest.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand,
		Command: together.CommandPause,
	})
	if paused := host.awaitGroup(together.GroupPaused); paused.Group.ByMemberID != membership.Body.MemberID {
		t.Errorf("the host's group = %+v; want paused by the guest", paused.Group)
	}
}

// Only a role that may host Watch Together starts a session, only of a film it reaches, and only
// the host rotates, ejects or ends it; a link that changed admits no one (D119, D125, D243 §4).
func TestTogether_OnlyAHostStartsRotatesAndEnds(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	created := harness.startTogether(t, admin)
	made := harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "watchers", "capabilities": []string{}, "libraries": []map[string]any{{"libraryId": 1}},
	})
	if made.status != http.StatusCreated {
		t.Fatalf("POST /roles = %d %s", made.status, made.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "watcher", Password: "watcher-password", Role: "watchers"}); err != nil {
		t.Fatal(err)
	}
	watcher := harness.signIn(t, "watcher", "watcher-password")
	film := map[string]any{"fileId": 1, "transport": "group"}
	wantError(t, harness.do(t, http.MethodPost, "/together", watcher, film), http.StatusForbidden, CodeForbidden)
	path := fmt.Sprintf("/together/%d", created.Body.SessionID)
	wantError(t, harness.do(t, http.MethodPost, path+"/rotate", watcher, nil), http.StatusForbidden, CodeForbidden)
	rotated := harness.do(t, http.MethodPost, path+"/rotate", admin, nil)
	if rotated.status != http.StatusOK {
		t.Fatalf("POST %s/rotate = %d %s", path, rotated.status, rotated.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/together/join", "",
		map[string]any{"secret": created.Body.JoinSecret, "name": "stranger"}), http.StatusNotFound, CodeNotFound)
	if ended := harness.do(t, http.MethodDelete, path, admin, nil); ended.status != http.StatusNoContent {
		t.Errorf("DELETE %s = %d %s", path, ended.status, ended.body)
	}
}

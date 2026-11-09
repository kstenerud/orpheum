// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/together"
)

// awaitRejected reads until the device is refused something, and returns the refusal; a pong
// first fails the test, as a ping acted on.
func (opened socket) awaitRejected() together.Outbound {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	for {
		var message together.Outbound
		if err := wsjson.Read(ctx, opened.connection, &message); err != nil {
			opened.t.Fatalf("not refused: %v", err)
		}
		if message.Type == together.MessageRejected {
			return message
		}
		if message.Type == together.MessagePong || message.Type == together.MessageWelcome {
			opened.t.Fatalf("answered %+v; want the message refused, nothing in it acted on, attached to nothing",
				message)
		}
	}
}

// A hello of another version than the core's is refused, versionMismatch, and the socket closed:
// the device is attached to nothing, as a page left open across an update must not misread the
// session (a group's state was "Idle" in version 2), and reloads for the client the core serves.
func TestTogether_AHelloOfAnotherVersionIsRefusedAndAttachesNothing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	old := harness.dialSocket(t)
	old.send(together.Inbound{
		V: together.ProtocolVersion - 1, Type: together.MessageHello, Channel: "together",
		Credential: opened.Body.MembershipCredential, Role: together.RoleVideo,
	})
	if refused := old.awaitRejected(); refused.Code != "versionMismatch" || refused.V != together.ProtocolVersion {
		t.Errorf("the hello was answered %+v; want rejected, versionMismatch, in version %d", refused,
			together.ProtocolVersion)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var after together.Outbound
	err := wsjson.Read(ctx, old.connection, &after)
	if status := websocket.CloseStatus(err); status != websocket.StatusPolicyViolation {
		t.Errorf("after the refusal the socket said %+v (%v), status %d; want it closed 1008", after, err, status)
	}
	harness.logs.AwaitEvent(t, "versionMismatch")
	if logged := harness.logs.String(); strings.Contains(logged, "deviceConnected") {
		t.Errorf("a device connected by a hello of another version:\n%s", logged)
	}
}

// A message of another version from a device attached is refused, versionMismatch, and nothing in
// it is acted on: a ping of the old version gets no pong.
func TestTogether_AMessageOfAnotherVersionIsRefusedAndActedOnNot(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	host := harness.dialTogetherSignedIn(t, opened.Body.MembershipCredential, admin)
	host.awaitWelcome()
	host.send(together.Inbound{V: together.ProtocolVersion - 1, Type: together.MessagePing, TMono: 1})
	if refused := host.awaitRejected(); refused.Code != "versionMismatch" {
		t.Errorf("the ping of another version was answered %+v; want rejected, versionMismatch", refused)
	}
}

// The core and the web client speak one version of the protocol, 3, the core's ProtocolVersion and
// the SDK's togetherProtocolVersion: version 3 says a group's state in camelCase (D459).
func TestTogether_TheCoreAndTheClientSpeakVersion3(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "..", "web", "packages", "sdk", "src", "together.ts")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`export const togetherProtocolVersion = (\d+);`).FindSubmatch(source)
	if found == nil {
		t.Fatal("together.ts has no togetherProtocolVersion this test can read; say where the client's version went")
	}
	web, err := strconv.Atoi(string(found[1]))
	if err != nil {
		t.Fatal(err)
	}
	if web != together.ProtocolVersion || together.ProtocolVersion != 3 {
		t.Errorf("the client speaks version %d and the core %d; want both 3", web, together.ProtocolVersion)
	}
}

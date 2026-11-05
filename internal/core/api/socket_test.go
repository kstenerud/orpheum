// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// A client of the last release opens its live connection at /together/ws with a "live" hello, and its
// session's there with a hello naming no channel; both serve for one release (D425 §3, goes next
// batch). On /socket, a hello naming a channel the socket does not serve is refused.
func TestSocket_TheOldPathAndHellosServeForOneRelease(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	for _, path := range []string{api.TogetherSocketPath, api.SocketPath} {
		opened := harness.dialLiveAt(t, path, live.Inbound{Type: live.MessageOldHello, SessionToken: admin})
		opened.await(live.MessageDevices, func(live.Outbound) bool { return true })
	}
	session := harness.openTogether(t, admin)
	old := harness.dialLiveAt(t, api.TogetherSocketPath, together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageHello, Credential: session.Body.MembershipCredential,
		Role: together.RoleVideo,
	})
	host := socket{t: t, connection: old.connection}
	if welcome := host.awaitWelcome(); welcome.SessionID != session.Body.SessionID {
		t.Errorf("the old path's hello naming no channel was welcomed to %+v", welcome)
	}
	stray := harness.dialLiveAt(t, api.SocketPath, together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageHello, Channel: "radio",
		Credential: session.Body.MembershipCredential, Role: together.RoleVideo,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		if _, _, err := stray.connection.Read(ctx); err != nil {
			if status := websocket.CloseStatus(err); status != websocket.StatusPolicyViolation {
				t.Errorf("a hello naming channel radio ended with %v (%v); want a refusal", status, err)
			}
			return
		}
	}
}

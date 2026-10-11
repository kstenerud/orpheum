// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// A signed-in client's offer is answered, and its requests over data channels reach the same API, its
// own sign-in carried in each request's head (D273 §8).
func TestRTCOffer_ASignedInClientReachesTheAPIOverDataChannels(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	rtc := rtcapi.New(t.Context(), harness.server.Handler(), slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = rtc.Close() }) //nolint:errcheck // the test is over
	harness.server.SetRTC(rtc)
	token := harness.createAdminAndSignIn(t)
	wantError(t, harness.do(t, http.MethodPost, "/rtc/offer", "", map[string]any{"type": "offer", "sdp": "v=0"}),
		http.StatusUnauthorized, CodeAuthRequired)

	//nolint:contextcheck // the harness's requests carry the test's context
	link := rtcapitest.Connect(t, func(_ context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		answered := harness.do(t, http.MethodPost, "/rtc/offer", token, map[string]any{
			"type": offer.Type, "sdp": offer.SDP,
		})
		var answer SessionDescriptionBody
		if answered.status != http.StatusOK || json.Unmarshal([]byte(answered.body), &answer) != nil {
			return rtcapi.Description{}, fmt.Errorf("POST /rtc/offer = %d %s", answered.status, answered.body)
		}
		return rtcapi.Description{Type: answer.Type, SDP: answer.SDP}, nil
	})
	signedIn := link.Do(rtcapi.RequestHead{
		Method: http.MethodGet, Path: Prefix + "/libraries",
		Headers: http.Header{"Authorization": {"Bearer " + token}},
	}, nil)
	if signedIn.Head.Status != http.StatusOK || !strings.Contains(string(signedIn.Body), `"libraries"`) {
		t.Errorf("GET /libraries over a channel = %d %s; want the libraries", signedIn.Head.Status, signedIn.Body)
	}
	anonymous := link.Do(rtcapi.RequestHead{Method: http.MethodGet, Path: Prefix + "/libraries"}, nil)
	if anonymous.Head.Status != http.StatusUnauthorized {
		t.Errorf("GET /libraries over a channel with no sign-in = %d; want 401: a connection grants nothing",
			anonymous.Head.Status)
	}
}

// A device reaches its Watch Together session over a data channel as over a WebSocket: welcomed after
// its hello, and refused with the WebSocket's close code when its first message is not one (D273 §8).
func TestRTCOffer_AWatchTogetherDeviceConnectsOverADataChannel(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	rtc := rtcapi.New(t.Context(), harness.server.Handler(), slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = rtc.Close() }) //nolint:errcheck // the test is over
	harness.server.SetRTC(rtc)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	link := rtcapitest.Connect(t, rtc.Answer)

	device, head := link.Upgrade(togetherSocketPath)
	if head.Status != http.StatusSwitchingProtocols {
		t.Fatalf("the upgrade was answered %d; want 101", head.Status)
	}
	//nolint:gosec // G117: a test's hello, with no session token in it
	hello, err := json.Marshal(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageHello,
		Credential: opened.Body.MembershipCredential, Role: together.RolePlayback,
	})
	if err != nil {
		t.Fatal(err)
	}
	device.Send(string(hello))
	for {
		var message together.Outbound
		if err := json.Unmarshal([]byte(device.Next()), &message); err != nil {
			t.Fatal(err)
		}
		if message.Type == together.MessageWelcome {
			break
		}
	}

	stranger, _ := link.Upgrade(togetherSocketPath)
	stranger.Send(`{"type":"state"}`)
	if closing := stranger.Closed(); closing.Code != int(websocket.StatusPolicyViolation) {
		t.Errorf("a first message that is no hello closed with %d %q; want %d", closing.Code, closing.Reason,
			websocket.StatusPolicyViolation)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// firefox is a browser's user agent, as a page puts it in a request's head.
const firefox = "Mozilla/5.0 (X11; Linux x86_64; rv:158.0) Gecko/20100101 Firefox/158.0"

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
		http.StatusUnauthorized, api.CodeAuthRequired)

	link := connectedOverRTC(t, harness, token)
	signedIn := link.Do(rtcapi.RequestHead{
		Method: http.MethodGet, Path: api.Prefix + "/libraries",
		Headers: http.Header{"Authorization": {"Bearer " + token}},
	}, nil)
	if signedIn.Head.Status != http.StatusOK || !strings.Contains(string(signedIn.Body), `"libraries"`) {
		t.Errorf("GET /libraries over a channel = %d %s; want the libraries", signedIn.Head.Status, signedIn.Body)
	}
	// A playback asked for over the channel is logged as come over WebRTC, whose head names no host.
	harness.shelveFilms(t)
	asked, err := json.Marshal(chrome(2))
	if err != nil {
		t.Fatal(err)
	}
	started := link.Do(rtcapi.RequestHead{
		Method: http.MethodPost, Path: api.Prefix + "/playback",
		Headers: http.Header{
			"Authorization": {"Bearer " + token}, "Content-Type": {"application/json"}, "User-Agent": {firefox},
		},
	}, asked)
	if started.Head.Status != http.StatusOK {
		t.Fatalf("POST /playback over a channel = %d %s", started.Head.Status, started.Body)
	}
	// It names the browser, whose page puts its user agent in each head, and the device: a playback over
	// WebRTC said userAgent="" (local-path-2026-10-27.md §4 item 7).
	harness.logs.AwaitEvent(t, "playbackStarted", `"over":"webrtc"`, `"userAgent":"`+firefox+`"`,
		fmt.Sprintf(`"deviceId":%d`, harness.thisDeviceID(t, token)))
	anonymous := link.Do(rtcapi.RequestHead{Method: http.MethodGet, Path: api.Prefix + "/libraries"}, nil)
	if anonymous.Head.Status != http.StatusUnauthorized {
		t.Errorf("GET /libraries over a channel with no sign-in = %d; want 401: a connection grants nothing",
			anonymous.Head.Status)
	}
}

// connectedOverRTC is a WebRTC connection to harness's core, its offer sent with token's sign-in.
func connectedOverRTC(t *testing.T, harness testHarness, token string) *rtcapitest.Client {
	t.Helper()
	//nolint:contextcheck // the harness's requests carry the test's context
	return rtcapitest.Connect(t, func(_ context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		answered := harness.do(t, http.MethodPost, "/rtc/offer", token, map[string]any{
			"type": offer.Type, "sdp": offer.SDP,
		})
		var answer api.SessionDescriptionBody
		if answered.status != http.StatusOK || json.Unmarshal([]byte(answered.body), &answer) != nil {
			return rtcapi.Description{}, fmt.Errorf("POST /rtc/offer = %d %s", answered.status, answered.body)
		}
		return rtcapi.Description{Type: answer.Type, SDP: answer.SDP}, nil
	})
}

// Over WebRTC a page's reads come gzipped where it asks, as over HTTP, from the same compressor, and an
// answer that carries a credential still never does (D483, D342): every JSON answer over WebRTC came
// whole, on the links that can least carry it (the slow-link war game, 2026-11-14).
func TestRTCOffer_AReadOverADataChannelIsGzippedWhereTheClientAsks(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	rtc := rtcapi.New(t.Context(), harness.server.Handler(), slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = rtc.Close() }) //nolint:errcheck // the test is over
	harness.server.SetRTC(rtc)
	token := harness.createAdminAndSignIn(t)
	// A data channel's request is not from the house, so the admin reads /config, an answer the core
	// gzips, from away (D302).
	set := harness.do(t, http.MethodPut, "/me/admin-away", token, map[string]any{"adminAway": "always"})
	if set.status != http.StatusNoContent && set.status != http.StatusOK {
		t.Fatalf("PUT /me/admin-away = %d %s", set.status, set.body)
	}
	link := connectedOverRTC(t, harness, token)
	read := func(encoding string) rtcapitest.Reply {
		return link.Do(rtcapi.RequestHead{
			Method: http.MethodGet, Path: api.Prefix + "/config",
			Headers: http.Header{"Authorization": {"Bearer " + token}, "Accept-Encoding": {encoding}},
		}, nil)
	}
	plain, zipped := read("identity"), read("gzip")
	if plain.Head.Status != http.StatusOK || plain.Head.Headers.Get("Content-Encoding") != "" {
		t.Fatalf("GET /config over a channel, taking no gzip = %d %v %s; want it as it is",
			plain.Head.Status, plain.Head.Headers, plain.Body)
	}
	if zipped.Head.Headers.Get("Content-Encoding") != "gzip" || len(zipped.Body) >= len(plain.Body) {
		t.Fatalf("GET /config over a channel, taking gzip = %v, %d bytes of %d; want it gzipped",
			zipped.Head.Headers, len(zipped.Body), len(plain.Body))
	}
	if unzipped := gunzipped(t, string(zipped.Body)); unzipped != string(plain.Body) {
		t.Errorf("GET /config over a channel unzips to\n%s\nwant\n%s", unzipped, plain.Body)
	}
	signIn, err := json.Marshal(map[string]any{"name": "owner", "password": "owner-password", "device": testDevice})
	if err != nil {
		t.Fatal(err)
	}
	signedIn := link.Do(rtcapi.RequestHead{
		Method: http.MethodPost, Path: api.Prefix + "/session",
		Headers: http.Header{"Content-Type": {"application/json"}, "Accept-Encoding": {"gzip"}},
	}, signIn)
	if signedIn.Head.Status != http.StatusCreated || signedIn.Head.Headers.Get("Content-Encoding") != "" {
		t.Errorf("a sign-in over a channel, taking gzip = %d %v; want it as it is",
			signedIn.Head.Status, signedIn.Head.Headers)
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

	device, head := link.Upgrade(api.TogetherSocketPath)
	if head.Status != http.StatusSwitchingProtocols {
		t.Fatalf("the upgrade was answered %d; want 101", head.Status)
	}
	//nolint:gosec // G117: a test's hello, with no session token in it
	hello, err := json.Marshal(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageHello,
		Credential: opened.Body.MembershipCredential, Role: together.RoleVideo,
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

	stranger, _ := link.Upgrade(api.TogetherSocketPath)
	stranger.Send(`{"type":"state"}`)
	if closing := stranger.Closed(); closing.Code != int(websocket.StatusPolicyViolation) {
		t.Errorf("a first message that is no hello closed with %d %q; want %d", closing.Code, closing.Reason,
			websocket.StatusPolicyViolation)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// A page on the discovery's client origin may call the API with its sign-in by header and read what
// the player needs; no other origin may, nor "null", no answer allows credentials, and every answer
// varies by origin (the discovery protocol's §3.4; discovery-core.md K10).
func TestCrossOrigin_LetsTheDiscoverysClientCallTheAPIByHeaderAlone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	client := "https://app.example"
	harness.server.NamedClientOrigin().Store(&client)
	for origin, isAllowed := range map[string]bool{client: true, "https://evil.example": false, "null": false} {
		preflight := harness.doWithHeader(t, http.MethodOptions, "/me", "", http.Header{
			"Origin": {origin}, "Access-Control-Request-Method": {"GET"},
			"Access-Control-Request-Headers": {"authorization"},
		}, nil)
		allowed := preflight.header.Get("Access-Control-Allow-Origin")
		if (allowed == origin) != isAllowed || preflight.header.Get("Vary") != "Origin" ||
			preflight.header.Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("a preflight from %q allowed %q (Vary %q); want allowed %v, varying by origin, no credentials",
				origin, allowed, preflight.header.Get("Vary"), isAllowed)
		}
		allowedHeaders := preflight.header.Get("Access-Control-Allow-Headers")
		if isAllowed && (!strings.Contains(allowedHeaders, "Authorization") ||
			!strings.Contains(allowedHeaders, api.ViewerHeaderName)) {
			t.Errorf("the preflight allows headers %q; want Authorization and the viewer's among them",
				allowedHeaders)
		}
	}
	fromClient := harness.doWithHeader(t, http.MethodGet, "/me", token, http.Header{"Origin": {client}}, nil)
	exposed := fromClient.header.Get("Access-Control-Expose-Headers")
	if fromClient.status != http.StatusOK || fromClient.header.Get("Access-Control-Allow-Origin") != client ||
		!strings.Contains(exposed, "Content-Range") || !strings.Contains(exposed, api.RequestIDHeader) ||
		strings.Contains(exposed, api.ClientScriptHeader) {
		t.Errorf("GET /me from the client's origin = %d, allowing %q and exposing %q; want it allowed, the "+
			"range and the request's ID exposed and the core's own client script not", fromClient.status,
			fromClient.header.Get("Access-Control-Allow-Origin"), exposed)
	}
	nobody := ""
	harness.server.NamedClientOrigin().Store(&nobody)
	other := harness.doWithHeader(t, http.MethodGet, "/me", token, http.Header{"Origin": {client}}, nil)
	if other.header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("with no discovery naming the client's origin, a page there may still read the API")
	}
}

// The Watch Together socket opens from the discovery's client too, and from no other origin.
func TestCrossOrigin_TheSessionSocketOpensFromTheDiscoverysClientAlone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	client := "https://app.example"
	harness.server.NamedClientOrigin().Store(&client)
	address := "ws" + strings.TrimPrefix(harness.httpServer.URL, "http") + api.TogetherSocketPath
	for origin, isAllowed := range map[string]bool{client: true, "https://evil.example": false} {
		connection, answer, err := websocket.Dial(t.Context(), address, &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": {origin}},
		})
		if answer != nil && answer.Body != nil {
			_ = answer.Body.Close() //nolint:errcheck // the upgrade's answer is read
		}
		if (err == nil) != isAllowed {
			t.Errorf("the socket from %s: %v; want it opened %v", origin, err, isAllowed)
		}
		if connection != nil {
			_ = connection.CloseNow() //nolint:errcheck // the test is over
		}
	}
}

// The loader's origin, where no core's client runs, may read the health its path race probes and
// ask for a hand-off's code, and nothing else: no session, no library (D308 §3–4).
func TestCrossOrigin_TheLoadersOriginReachesOnlyWhatFindingTheCoreTakes(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	client, loader := "https://core1.client.example", "https://app.example"
	harness.server.NamedClientOrigin().Store(&client)
	harness.server.NamedLoaderOrigin().Store(&loader)
	for path, isAllowed := range map[string]bool{
		"/health": true, "/session/handoff": true, "/session/redeem": false, "/me": false, "/items": false,
	} {
		answer := harness.doWithHeader(t, http.MethodGet, path, token, http.Header{"Origin": {loader}}, nil)
		if allowed := answer.header.Get("Access-Control-Allow-Origin"); (allowed == loader) != isAllowed {
			t.Errorf("GET %s from the loader's origin allowed %q; want allowed %v", path, allowed, isAllowed)
		}
	}
	if answer := harness.doWithHeader(t, http.MethodGet, "/me", token, http.Header{"Origin": {client}}, nil); answer.
		header.Get("Access-Control-Allow-Origin") != client {
		t.Error("the client's origin may not read /me beside a loader's")
	}
}

// The client origin reads the core's own client, its page and its files, over the core's names as
// over WebRTC, for its loader to check and run (D308 §6); the loader's origin reads none of it. Over
// the core's names the page was refused, and a browser whose LAN waited on its local-network prompt
// ran out of paths (staging, 2026-10-17).
func TestCrossOrigin_TheClientOriginReadsTheCoresClientAndTheLoadersDoesNot(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	client, loader := "https://core1.client.example", "https://app.example"
	harness.server.NamedClientOrigin().Store(&client)
	harness.server.NamedLoaderOrigin().Store(&loader)
	for _, path := range []string{"/", "/assets/client-main.js", "/sw.js"} {
		for origin, isAllowed := range map[string]bool{client: true, loader: false, "https://evil.example": false} {
			address := harness.httpServer.URL + path
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Origin", origin)
			answer, err := harness.httpServer.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_ = answer.Body.Close() //nolint:errcheck // only the headers are read
			if allowed := answer.Header.Get("Access-Control-Allow-Origin"); (allowed == origin) != isAllowed ||
				answer.Header.Get("Vary") != "Origin" {
				t.Errorf("GET %s from %s allowed %q (Vary %q); want allowed %v, varying by origin", path, origin,
					allowed, answer.Header.Get("Vary"), isAllowed)
			}
		}
	}
}

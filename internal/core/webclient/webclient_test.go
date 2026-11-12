// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kstenerud/orpheum/internal/core/webclient"
)

func builtClientFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html><title>Orpheum</title>")},
		"assets/app-1a2b.js": {Data: []byte("console.log(1)")},
		"favicon.svg":        {Data: []byte("<svg/>")},
	}
}

func get(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	return recorder
}

func TestHandler_ServesFilesAndFallsBackToIndex(t *testing.T) {
	t.Parallel()
	handler := webclient.Handler(builtClientFS(), "/api/v1")
	for target, want := range map[string]string{
		"/":                   "<!doctype html><title>Orpheum</title>",
		"/films":              "<!doctype html><title>Orpheum</title>",
		"/titles/42":          "<!doctype html><title>Orpheum</title>",
		"/favicon.svg":        "<svg/>",
		"/assets/app-1a2b.js": "console.log(1)",
	} {
		response := get(t, handler, target)
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Errorf("%s: %d %q, want 200 %q", target, response.Code, response.Body.String(), want)
		}
		if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options %q, want nosniff", target, got)
		}
	}
	if response := get(t, handler, "/assets/gone-9z.js"); response.Code != http.StatusNotFound {
		t.Errorf("a missing asset: %d, want 404 rather than the client", response.Code)
	}
}

// Every page of the client says what it may load and that it may not be framed, sends no referrer, and
// over HTTPS tells the browser to come back only over HTTPS (the second audit's W14, K19,
// 2026-10-13: the core sent none of these).
func TestHandler_EveryAnswerCarriesTheSecurityHeaders(t *testing.T) {
	t.Parallel()
	handler := webclient.Handler(builtClientFS(), "/api/v1")
	plain := get(t, handler, "/films")
	policy := plain.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'",
		"base-uri 'none'"} {
		if !strings.Contains(policy, want) {
			t.Errorf("the content-security policy %q lacks %q", policy, want)
		}
	}
	if got := plain.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy %q, want no-referrer", got)
	}
	if got := plain.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("over plain HTTP, Strict-Transport-Security %q; want none", got)
	}
	overTLS := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/films", nil)
	overTLS.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, overTLS)
	if got := recorder.Header().Get("Strict-Transport-Security"); !strings.HasPrefix(got, "max-age=") {
		t.Errorf("over HTTPS, Strict-Transport-Security %q; want a max-age", got)
	}
}

func TestHandler_NeverAnswersAnAPIPathWithTheClient(t *testing.T) {
	t.Parallel()
	handler := webclient.Handler(builtClientFS(), "/api/v1")
	for _, target := range []string{"/api/v1", "/api/v1/nothing", "/api/v1/bytes/art/x/y.webp", "/debug/pprof/", "/x/../api/v1/z"} {
		if response := get(t, handler, target); response.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", target, response.Code)
		}
	}
	if response := get(t, handler, "/apiary"); response.Code != http.StatusOK {
		t.Errorf("/apiary is the client's, not the API's: %d", response.Code)
	}
}

func TestHandler_HashedAssetsAreImmutableAndIndexIsRevalidated(t *testing.T) {
	t.Parallel()
	handler := webclient.Handler(builtClientFS(), "/api/v1")
	for target, want := range map[string]string{
		"/assets/app-1a2b.js": "public, max-age=31536000, immutable",
		"/":                   "no-cache",
		"/films":              "no-cache",
		"/favicon.svg":        "no-cache",
	} {
		if got := get(t, handler, target).Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control %q, want %q", target, got, want)
		}
	}
}

func TestHandler_OnlyReads(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	webclient.Handler(builtClientFS(), "/api/v1").ServeHTTP(recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/films", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /films: %d, want 405", recorder.Code)
	}
}

func TestHandler_WithoutTheClientSaysSo(t *testing.T) {
	t.Parallel()
	response := get(t, webclient.Handler(nil, "/api/v1"), "/films")
	if response.Code != http.StatusNotFound || response.Body.Len() == 0 {
		t.Errorf("no client: %d %q, want 404 with a reason", response.Code, response.Body.String())
	}
}

// Only the client's screens and the cast receiver may load Google's Cast SDK: every screen the
// sender's, which the Cast menu's Chromecast loads in Chrome at its first opening (D465 §3), the
// client moving between screens in place, and the lab's cast probe with them; the receiver under
// /cast/ CAF's and the Cast platform's socket on the device. The client's files and the lab's other
// probes load nothing from elsewhere, and no screen reaches the device's socket.
func TestHandler_OnlyTheScreensAndTheReceiverMayLoadGooglesCastSDK(t *testing.T) {
	t.Parallel()
	files := builtClientFS()
	files["cast/index.html"] = &fstest.MapFile{Data: []byte("<!doctype html><title>Receiver</title>")}
	handler := webclient.Handler(files, "/api/v1")
	policyOf := func(target string) string {
		return get(t, handler, target).Header().Get("Content-Security-Policy")
	}
	for _, target := range []string{"/lab", "/lab/device", "/lab/cast/x", "/favicon.svg", "/assets/app-1a2b.js",
		"/assets/missing-9z.js"} {
		if policy := policyOf(target); strings.Contains(policy, "gstatic") || strings.Contains(policy, "localhost") {
			t.Errorf("%s: the policy %q lets the page load from elsewhere", target, policy)
		}
	}
	for _, target := range []string{"/", "/index.html", "/films", "/play/4", "/castle", "/lab/cast"} {
		sender := policyOf(target)
		for _, want := range []string{"script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com;",
			"connect-src 'self' https://www.gstatic.com;", "frame-ancestors 'none'", "object-src 'none'"} {
			if !strings.Contains(sender, want) {
				t.Errorf("%s: the screen's policy %q lacks %q", target, sender, want)
			}
		}
		if strings.Contains(sender, "localhost") {
			t.Errorf("%s: the screen's policy %q reaches the Cast device's socket", target, sender)
		}
	}
	for _, target := range []string{"/cast", "/cast/", "/cast/assets/receiver-1a.js"} {
		receiver := policyOf(target)
		for _, want := range []string{"script-src 'self' https://www.gstatic.com;",
			"connect-src 'self' ws://localhost:8008 https://www.gstatic.com;", "frame-ancestors 'none'"} {
			if !strings.Contains(receiver, want) {
				t.Errorf("%s: the receiver's policy %q lacks %q", target, receiver, want)
			}
		}
	}
}

// The client's pages report, not refuse, every string reaching a script sink without a Trusted
// Types policy (the 11-22 batch's item 6); the cast pages, whose Google SDK is not the client's,
// are sent no such policy.
func TestHandler_TheClientsPagesReportWhatTrustedTypesWouldRefuse(t *testing.T) {
	t.Parallel()
	files := builtClientFS()
	files["cast/index.html"] = &fstest.MapFile{Data: []byte("<!doctype html><title>Receiver</title>")}
	handler := webclient.Handler(files, "/api/v1")
	for _, target := range []string{"/", "/films", "/assets/app-1a2b.js"} {
		headers := get(t, handler, target).Header()
		if got := headers.Get("Content-Security-Policy-Report-Only"); got != "require-trusted-types-for 'script'" {
			t.Errorf("%s: the report-only policy %q, want require-trusted-types-for 'script'", target, got)
		}
		if enforced := headers.Get("Content-Security-Policy"); strings.Contains(enforced, "trusted-types") {
			t.Errorf("%s: the enforced policy %q holds Trusted Types, still to be reported on first", target, enforced)
		}
	}
	for _, target := range []string{"/cast/", "/lab/cast"} {
		if got := get(t, handler, target).Header().Get("Content-Security-Policy-Report-Only"); got != "" {
			t.Errorf("%s: the report-only policy %q, want none", target, got)
		}
	}
}

// The cast receiver, where it is built in, answers its own addresses under /cast/ with its own page,
// never the web client's; where it is not, they are the client's as any other address is.
func TestHandler_ServesTheCastReceiverUnderCast(t *testing.T) {
	t.Parallel()
	const receiverPage = "<!doctype html><title>Receiver</title>"
	files := builtClientFS()
	files["cast/index.html"] = &fstest.MapFile{Data: []byte(receiverPage)}
	files["cast/assets/receiver-1a.js"] = &fstest.MapFile{Data: []byte("console.log(2)")}
	handler := webclient.Handler(files, "/api/v1")
	for target, want := range map[string]string{
		"/cast/":                      receiverPage,
		"/cast":                       receiverPage,
		"/cast/lab":                   receiverPage,
		"/cast/assets/receiver-1a.js": "console.log(2)",
		"/castle":                     "<!doctype html><title>Orpheum</title>",
	} {
		response := get(t, handler, target)
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Errorf("%s: %d %q, want 200 %q", target, response.Code, response.Body.String(), want)
		}
	}
	unbuilt := webclient.Handler(builtClientFS(), "/api/v1")
	if response := get(t, unbuilt, "/cast/"); response.Body.String() != "<!doctype html><title>Orpheum</title>" {
		t.Errorf("/cast/ with no receiver built: %q, want the client", response.Body.String())
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package webclient_test

import (
	"net/http"
	"net/http/httptest"
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

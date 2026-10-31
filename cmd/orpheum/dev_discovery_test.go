// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// The development discovery serves its page saying it has realms when its operator names one, as a
// paid discovery's does, and saying nothing of realms when none is named, as an owner's own (D298 §1).
func TestDevDiscovery_ThePageSaysItHasRealmsOnlyWhenOneIsNamed(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><html><head></head><body></body></html>")}}
	for realm, want := range map[string]bool{"karlnas": true, "": false} {
		handler, err := devDiscoveryHandler(devDiscoverySite{
			base: "http://127.0.0.1:1", recordPath: filepath.Join(t.TempDir(), "single.json"), realm: realm,
			clientFiles: files, published: t.TempDir(),
		}, slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))
		page, err := io.ReadAll(recorder.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(page), `name="orpheum-discovery-realms"`); got != want {
			t.Errorf("with the realm %q the page says it has realms: %t; want %t", realm, got, want)
		}
	}
}

// The development discovery's page names its loader's configuration (D308): the client API at the
// page's own name, the loader at localhost and each core's client origin at <coreId>.localhost, on
// the port it serves at.
func TestDevDiscovery_ThePageNamesTheLoadersOriginsOnItsPort(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><html><head></head><body></body></html>")}}
	handler, err := devDiscoveryHandler(devDiscoverySite{
		base: "http://127.0.0.1:4321", recordPath: filepath.Join(t.TempDir(), "single.json"), clientFiles: files,
		published: t.TempDir(),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/films", http.NoBody))
	page := recorder.Body.String()
	for _, want := range []string{
		`<meta name="orpheum-discovery-client-api" content="/client/v1">`,
		`<meta name="orpheum-client-origins" content="http://{core}.localhost:4321">`,
		`<meta name="orpheum-loader-origin" content="http://localhost:4321">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page %q does not name %s", page, want)
		}
	}
}

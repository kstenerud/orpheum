// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// The development discovery serves, at its issuer's well-known path, an address document it signed
// at its start: the core API, client API and origins it serves at, which a core enrolling by the
// discovery's issuer trusts at first use (D391 §3).
func TestDevDiscovery_ServesTheAddressDocumentItSignedAtItsIssuer(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><html><head></head><body></body></html>")}}
	const base = "http://127.0.0.1:4321"
	handler, err := devDiscoveryHandler(devDiscoverySite{
		base: base, recordPath: filepath.Join(t.TempDir(), "single.json"), clientFiles: files,
		documentPath: filepath.Join(t.TempDir(), "address-document.json"),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, addressdoc.WellKnownPath, http.NoBody))
	var served addressdoc.Served
	if err = json.Unmarshal(recorder.Body.Bytes(), &served); err != nil {
		t.Fatalf("the well-known path answered %d %q: %v", recorder.Code, recorder.Body.String(), err)
	}
	pinned, err := trust.FirstUse(served.RootSets)
	if err != nil {
		t.Fatal(err)
	}
	// A root key signs the document itself, with no delegation, so it holds at any time.
	document, _, err := addressdoc.Open(served, pinned, base, time.UnixMilli(0))
	if err != nil {
		t.Fatal(err)
	}
	if document.CoreAPI != base+"/core/v1" || document.LoaderOrigin != "http://localhost:4321" ||
		document.ClientOrigin("k7") != "http://k7.localhost:4321" {
		t.Errorf("the document names %+v; want the discovery's own API and origins", document)
	}
}

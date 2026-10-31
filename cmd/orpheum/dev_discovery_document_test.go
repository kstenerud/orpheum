// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// The development discovery publishes, at its issuer's well-known paths, a root chain of a key made
// for the run and an address document it signed: the core API, client API and origins it serves at,
// which a core enrolling by the discovery's issuer and identity opens (D393 §4).
func TestDevDiscovery_ServesTheAddressDocumentItSignedAtItsIssuer(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><html><head></head><body></body></html>")}}
	const base = "http://127.0.0.1:4321"
	handler, err := devDiscoveryHandler(devDiscoverySite{
		base: base, recordPath: filepath.Join(t.TempDir(), "single.json"), clientFiles: files, published: t.TempDir(),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) trust.Signed {
		t.Helper()
		var signed trust.Signed
		parts := map[*string]string{&signed.Payload: path, &signed.Signature: path + addressdoc.SignatureEnd}
		for into, at := range parts {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, at, http.NoBody))
			if recorder.Code != http.StatusOK {
				t.Fatalf("%s answered %d", at, recorder.Code)
			}
			*into = recorder.Body.String()
		}
		return signed
	}
	chain, err := trust.Genesis(trust.ChainDiscovery, get(addressdoc.RootPath(1)))
	if err != nil {
		t.Fatal(err)
	}
	document, err := addressdoc.Open(get(addressdoc.DocumentPath), chain, base)
	if err != nil {
		t.Fatal(err)
	}
	if document.CoreAPI != base+"/core/v1" || document.LoaderOrigin != "http://localhost:4321" ||
		document.ClientOrigin("k7") != "http://k7.localhost:4321" || document.ID != chain.ID() {
		t.Errorf("the document names %+v; want the discovery's own API and origins", document)
	}
}

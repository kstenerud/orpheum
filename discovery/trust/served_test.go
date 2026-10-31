// SPDX-License-Identifier: AGPL-3.0-or-later

package trust_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/discovery/trust"
)

func TestServed_ServesThePublishedDirectoryAsItIsAndNothingElse(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := trust.Publish(directory, []trust.Published{{
		Path:   "/.well-known/orpheum/root/1",
		Signed: trust.Signed{Payload: "orpheum-signed root 1\n", Signature: "sig"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(directory), "secret"), []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := trust.Served(directory, slog.New(slog.DiscardHandler))
	serve := func(method, path string) (int, string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody))
		body, err := io.ReadAll(recorder.Result().Body)
		if err != nil {
			t.Fatal(err)
		}
		return recorder.Code, string(body)
	}
	if status, body := serve(http.MethodGet, "/.well-known/orpheum/root/1"); status != http.StatusOK || body != "orpheum-signed root 1\n" {
		t.Fatalf("the statement: %d %q, want it byte for byte", status, body)
	}
	for _, refused := range []struct{ method, path string }{
		{http.MethodPost, "/.well-known/orpheum/root/1"},
		{http.MethodGet, "/.well-known/orpheum/root"},
		{http.MethodGet, "/.well-known/../secret"},
		{http.MethodGet, "/orpheum/root/1"},
		{http.MethodGet, "/.well-known/orpheum/root/2"},
	} {
		if status, _ := serve(refused.method, refused.path); status != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", refused.method, refused.path, status)
		}
	}
}

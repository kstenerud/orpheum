// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Every answer names the web client the server serves, so a page left open across an update knows
// to reload; a server serving no client names none (the owner, 2026-10-08).
func TestHandler_NamesTheServedClientsScriptOnEveryAnswer(t *testing.T) {
	t.Parallel()
	server := NewServer(slog.New(slog.DiscardHandler))
	for _, name := range []string{"", "index-DCLi77_b.js"} {
		server.SetClientScript(name)
		recorded := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, Prefix+"/health", nil)
		server.Handler().ServeHTTP(recorded, request)
		if got := recorded.Header().Get(ClientScriptHeader); got != name {
			t.Errorf("with the client %q served, an answer names %q", name, got)
		}
	}
}

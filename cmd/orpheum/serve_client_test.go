// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// The server serves its built client at "/", and every API answer names the client's script, so a
// page left open across an update reloads itself; a binary built without one names none.
func TestServeClient_ServesTheClientAndNamesItsScriptOnEveryAnswer(t *testing.T) {
	t.Parallel()
	page := `<script type="module" crossorigin src="/assets/index-DCLi77_b.js"></script>`
	client := fstest.MapFS{"index.html": {Data: []byte(page)}}
	for _, built := range []struct {
		hasClient bool
		want      string
	}{{true, "index-DCLi77_b.js"}, {false, ""}} {
		server := api.NewServer(slog.New(slog.DiscardHandler))
		serveClient(server, client, built.hasClient)
		answer := httptest.NewRecorder()
		server.Handler().ServeHTTP(answer,
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, api.Prefix+"/health", nil))
		if got := answer.Header().Get(api.ClientScriptHeader); got != built.want {
			t.Errorf("built with a client %v, an API answer names %q; want %q", built.hasClient, got, built.want)
		}
		if !built.hasClient {
			continue
		}
		home := httptest.NewRecorder()
		server.Handler().ServeHTTP(home, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), "index-DCLi77_b.js") {
			t.Errorf("/ answered %d %q; want the client's page", home.Code, home.Body.String())
		}
	}
}

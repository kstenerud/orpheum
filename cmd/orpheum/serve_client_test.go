// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"io"
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
		served := httptest.NewServer(server.Handler())
		answer, err := served.Client().Get(served.URL + api.Prefix + "/health")
		if err != nil {
			t.Fatal(err)
		}
		_ = answer.Body.Close()
		if got := answer.Header.Get(api.ClientScriptHeader); got != built.want {
			t.Errorf("built with a client %v, an API answer names %q; want %q", built.hasClient, got, built.want)
		}
		if built.hasClient {
			home, err := served.Client().Get(served.URL + "/")
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(home.Body)
			_ = home.Body.Close()
			if err != nil || home.StatusCode != http.StatusOK || !strings.Contains(string(body), "index-DCLi77_b.js") {
				t.Errorf("/ answered %d %q, %v; want the client's page", home.StatusCode, body, err)
			}
		}
		served.Close()
	}
}

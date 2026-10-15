// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/waiting"
)

// Every request the server serves is marked as one a person waits on, so its writes take the
// database's writer before background work's (D303, DL11).
func TestHandler_MarksEveryRequestAsAPersons(t *testing.T) {
	t.Parallel()
	server := NewServer(slog.New(slog.DiscardHandler))
	var isAPerson bool
	server.mux.HandleFunc("GET "+Prefix+"/probe-person", func(_ http.ResponseWriter, request *http.Request) {
		isAPerson = waiting.IsAPerson(request.Context())
	})
	server.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, Prefix+"/probe-person", nil))
	if !isAPerson {
		t.Error("a request reached its handler without the mark of a person waiting on it")
	}
}

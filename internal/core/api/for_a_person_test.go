// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"log/slog"
	"maps"
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

// A GET or a HEAD is marked as only reading; any other method is not (D303, DL10).
func TestHandler_MarksAGetOrAHeadAsReading(t *testing.T) {
	t.Parallel()
	server := NewServer(slog.New(slog.DiscardHandler))
	isReading := map[string]bool{}
	server.mux.HandleFunc(Prefix+"/probe-reading", func(_ http.ResponseWriter, request *http.Request) {
		isReading[request.Method] = waiting.IsAReader(request.Context())
	})
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut} {
		server.Handler().ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequestWithContext(t.Context(), method, Prefix+"/probe-reading", nil))
	}
	want := map[string]bool{http.MethodGet: true, http.MethodHead: true, http.MethodPost: false, http.MethodPut: false}
	if !maps.Equal(isReading, want) {
		t.Errorf("marked as reading: %v; want %v", isReading, want)
	}
}

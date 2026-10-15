// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// An API request a page waits on longer than a second is logged with its path and time, so a slow
// page says which request made it so; a quick one, a file's bytes and a socket are not (the owner's
// nas.example, 2026-10-15).
func TestLoggingSlow_NamesEveryAPIRequestThatTookOverASecond(t *testing.T) {
	t.Parallel()
	logs := &testutil.Log{}
	server := NewServer(slog.New(slog.NewJSONHandler(logs, nil)))
	now := clocktest.NewFake(time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC))
	server.dependencies.Store(&Dependencies{Clock: now})
	taking := func(took time.Duration) http.Handler {
		return server.loggingSlow(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { now.Advance(took) }))
	}
	serve := func(handler http.Handler, path string, header http.Header) {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		maps.Copy(request.Header, header)
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
	serve(taking(2500*time.Millisecond), Prefix+"/users/2", nil)
	serve(taking(300*time.Millisecond), Prefix+"/me", nil)
	serve(taking(5*time.Second), Prefix+"/bytes/media/x.m4s", nil)
	serve(taking(5*time.Second), Prefix+"/together/socket", http.Header{"Upgrade": {"websocket"}})
	logged := logs.String()
	if !strings.Contains(logged, `"method":"GET","path":"/api/v1/users/2","tookMs":2500`) {
		t.Errorf("the slow request is not logged with its path and time:\n%s", logged)
	}
	if strings.Count(logged, "a request was slow") != 1 {
		t.Errorf("logged more than the one slow request a page waits on:\n%s", logged)
	}
}

// The server's own handler times its requests: a slow one through it is logged.
func TestHandler_LogsASlowRequest(t *testing.T) {
	t.Parallel()
	logs := &testutil.Log{}
	server := NewServer(slog.New(slog.NewJSONHandler(logs, nil)))
	now := clocktest.NewFake(time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC))
	server.dependencies.Store(&Dependencies{Clock: now})
	server.Mux().HandleFunc(Prefix+"/slow-for-the-test", func(http.ResponseWriter, *http.Request) {
		now.Advance(3 * time.Second)
	})
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, Prefix+"/slow-for-the-test", nil)
	server.Handler().ServeHTTP(httptest.NewRecorder(), request)
	if !strings.Contains(logs.String(), `"path":"/api/v1/slow-for-the-test","tookMs":3000`) {
		t.Errorf("the server's handler logged no slow request:\n%s", logs.String())
	}
}

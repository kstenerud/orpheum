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
	"github.com/kstenerud/orpheum/internal/platform/querytally"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// An API request a page waits on longer than a second is logged with its path and time, so a slow
// page says which request made it so; a quick one, a file's bytes and a socket are not (the owner's
// the production NAS, 2026-10-15).
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

// A slow request's line names the database statements it ran and its writes (D303, DL17).
func TestLoggingSlow_NamesTheStatementsASlowRequestRan(t *testing.T) {
	t.Parallel()
	logs := &testutil.Log{}
	server := NewServer(slog.New(slog.NewJSONHandler(logs, nil)))
	now := clocktest.NewFake(time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC))
	server.dependencies.Store(&Dependencies{Clock: now})
	handler := server.loggingSlow(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		tally := querytally.Of(request.Context())
		tally.Count("-- name: GetLiveGrant :one")
		tally.Count("-- name: ListTitles :many")
		tally.CountWrite()
		now.Advance(2 * time.Second)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		Prefix+"/items", nil))
	if logged := logs.String(); !strings.Contains(logged, `"statements":"GetLiveGrant×1 ListTitles×1","writes":1`) {
		t.Errorf("the slow request's line does not name its statements and writes:\n%s", logged)
	}
}

// An API response of more than a mebibyte is logged with its path and size, however quickly it
// came; a small one, and a file's bytes, large by nature, are not (the owner's production NAS, 2026-10-16:
// a series' page of 5.3 MB that nothing named).
func TestLoggingSlow_NamesEveryAPIResponseOverAMebibyte(t *testing.T) {
	t.Parallel()
	logs := &testutil.Log{}
	server := NewServer(slog.New(slog.NewJSONHandler(logs, nil)))
	server.dependencies.Store(&Dependencies{Clock: clocktest.NewFake(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))})
	writing := func(size int) http.Handler {
		return server.loggingSlow(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(make([]byte, size)) //nolint:errcheck // a recorder
		}))
	}
	for path, size := range map[string]int{
		Prefix + "/items/606": 5 << 20, Prefix + "/me": 2 << 10, Prefix + "/bytes/media/x.m4s": 8 << 20,
	} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		writing(size).ServeHTTP(httptest.NewRecorder(), request)
	}
	logged := logs.String()
	if !strings.Contains(logged, `"path":"/api/v1/items/606","bytes":5242880`) ||
		strings.Count(logged, "a response was large") != 1 {
		t.Errorf("want the one large response named with its size:\n%s", logged)
	}
}

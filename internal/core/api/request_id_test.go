// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// Every answer names its request's ID, the core's own and not the one a client sent, each request
// its own; and the lines the request caused in the log carry it (D347): here, the error answered
// while the server is starting.
func TestRequestID_EveryAnswerNamesItAndTheLinesItCausedCarryIt(t *testing.T) {
	t.Parallel()
	logs := &testutil.Log{}
	logger, err := slogx.New(logs, slogx.FormatJSON, slog.LevelDebug)
	if err != nil {
		t.Fatal(err)
	}
	served := httptest.NewServer(NewServer(logger).Handler())
	t.Cleanup(served.Close)
	ids := map[string]bool{}
	for range 2 {
		request, requestErr := http.NewRequestWithContext(t.Context(), http.MethodGet, served.URL+Prefix+"/me",
			http.NoBody)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		request.Header.Set(RequestIDHeader, "chosen-by-the-client")
		answer, doErr := served.Client().Do(request)
		if doErr != nil {
			t.Fatal(doErr)
		}
		if closeErr := answer.Body.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		id := answer.Header.Get(RequestIDHeader)
		if len(id) != requestIDLength || id == "chosen-by-the-client" {
			t.Fatalf("the answer's %s is %q; want the core's own ID", RequestIDHeader, id)
		}
		ids[id] = true
	}
	if len(ids) != 2 {
		t.Errorf("two requests had IDs %v; want one each", ids)
	}
	logged := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		if err = json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if id, isString := record[slogx.RequestIDKey].(string); isString {
			logged[id] = true
		}
	}
	for id := range ids {
		if !logged[id] {
			t.Errorf("no line of the log carries request %s:\n%s", id, logs)
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// The plain listener answers the names a household reaches it by and refuses any other with 421, so a
// page that rebinds a public name of its own to the core's address reaches nothing (the discovery
// protocol's §7 item 10; S15).
func TestLANHandler_AnswersTheHousesNamesAndRefusesAnyOther(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	handler := harness.server.LANHandler([]string{"media.orpheum-app.com", ""})
	for host, isAnswered := range map[string]bool{
		"192.168.1.20:8440": true, "[fd00::20]:8440": true, "localhost:8440": true, "nas:8440": true,
		"nas.example:8440": true, "NAS.Local": true, "server.home.arpa": true, "box.internal": true,
		"media.orpheum-app.com": true, "media.orpheum-app.com.": true,
		"rebind.attacker.example": false, "orpheum-app.com": false,
		// No Host is a container's HTTP/1.0 health check; a browser always sends one.
		"": true,
	} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, api.Prefix+"/health", http.NoBody)
		request.Host = host
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if (recorder.Code != http.StatusMisdirectedRequest) != isAnswered {
			t.Errorf("a request naming %q answered %d; want it answered %v", host, recorder.Code, isAnswered)
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// Every signed-in client reads how often a screen asks again, as the admin set it (D237); someone
// signed out reads nothing.
func TestClientSettings_EveryClientReadsHowOftenAScreenAsksAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	wantError(t, harness.do(t, http.MethodGet, "/client-settings", "", nil), http.StatusUnauthorized, CodeAuthRequired)
	settingstest.Set(t, harness.server.dependencies.Load().Settings, settings.ScreenRefreshLongestWaitSeconds, "45")
	served := harness.do(t, http.MethodGet, "/client-settings", token, nil)
	var got ClientSettingsBody
	if err := json.Unmarshal([]byte(served.body), &got); err != nil || served.status != http.StatusOK {
		t.Fatalf("GET /client-settings = %d %s (%v)", served.status, served.body, err)
	}
	if got.ScreenRefreshFirstWaitSeconds != 2 || got.ScreenRefreshLongestWaitSeconds != 45 ||
		got.ScreenRefreshFailedWaitSeconds != 5 {
		t.Errorf("client settings = %+v, want the first wait's starting 2 s, the 45 s the admin set and "+
			"the failed wait's starting 5 s", got)
	}
}

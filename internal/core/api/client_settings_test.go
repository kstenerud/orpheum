// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// Every signed-in client reads how often a screen asks again, as the admin set it (D237); someone
// signed out reads nothing.
func TestClientSettings_EveryClientReadsHowOftenAScreenAsksAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	wantError(t, harness.do(t, http.MethodGet, "/client-settings", "", nil), http.StatusUnauthorized,
		api.CodeAuthRequired)
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ScreenRefreshLongestWaitSeconds, "45")
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.InterestSaidEverySeconds, "20")
	served := harness.do(t, http.MethodGet, "/client-settings", token, nil)
	var got api.ClientSettingsBody
	if err := json.Unmarshal([]byte(served.body), &got); err != nil || served.status != http.StatusOK {
		t.Fatalf("GET /client-settings = %d %s (%v)", served.status, served.body, err)
	}
	if got.ScreenRefreshFirstWaitSeconds != 2 || got.ScreenRefreshLongestWaitSeconds != 45 ||
		got.ScreenRefreshFailedWaitSeconds != 5 {
		t.Errorf("client settings = %+v, want the first wait's starting 2 s, the 45 s the admin set and "+
			"the failed wait's starting 5 s", got)
	}
	if got.InterestSaidEverySeconds != 20 {
		t.Errorf("a page says it is shown every %d s; want the 20 s the admin set (D324 §1)",
			got.InterestSaidEverySeconds)
	}
}

// The pages for development are offered only once the admin turns them on (the owner, 2026-10-17).
func TestClientSettings_TheDeveloperPagesAreOfferedOnlyWhenTurnedOn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	read := func() api.ClientSettingsBody {
		served := harness.do(t, http.MethodGet, "/client-settings", token, nil)
		var got api.ClientSettingsBody
		if err := json.Unmarshal([]byte(served.body), &got); err != nil || served.status != http.StatusOK {
			t.Fatalf("GET /client-settings = %d %s (%v)", served.status, served.body, err)
		}
		return got
	}
	if read().ShouldShowDeveloperPages {
		t.Error("the developer pages are offered before the admin turned them on")
	}
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldShowDeveloperPages, "true")
	if !read().ShouldShowDeveloperPages {
		t.Error("the developer pages are not offered once the admin turned them on")
	}
}

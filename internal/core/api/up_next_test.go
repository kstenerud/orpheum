// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// Autoplay is the person's own and on until they turn it off (plans/up-next.md): their settings read
// it, and every playback of theirs starts with it; turned off, it is off in every file, and someone
// else's stays on.
func TestPreferences_AutoplayIsOnUntilThePersonTurnsItOff(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	other := harness.signIn(t, "partner", "partner-password")
	harness.shelveFilms(t)
	autoplayOf := func(t *testing.T, token string, fileID int64) (inSettings, inPlayback bool) {
		t.Helper()
		var settings struct {
			IsAutoplayOn bool `json:"isAutoplayOn"`
		}
		harness.readJSON(t, "/me/preferences", token, &settings)
		answer := harness.do(t, http.MethodPost, "/playback", token, chrome(fileID))
		var started struct {
			Preferences struct {
				IsAutoplayOn bool `json:"isAutoplayOn"`
			} `json:"preferences"`
		}
		if err := json.Unmarshal([]byte(answer.body), &started); err != nil || answer.status != http.StatusOK {
			t.Fatalf("POST /playback = %d %s (%v)", answer.status, answer.body, err)
		}
		return settings.IsAutoplayOn, started.Preferences.IsAutoplayOn
	}
	if inSettings, inPlayback := autoplayOf(t, user, 1); !inSettings || !inPlayback {
		t.Errorf("before any choice: autoplay %v in settings, %v in a playback; want on in both", inSettings,
			inPlayback)
	}
	turnOff := map[string]bool{"isAutoplayOn": false}
	answer := harness.do(t, http.MethodPut, "/me/preferences", user, turnOff)
	if answer.status != http.StatusNoContent {
		t.Fatalf("PUT autoplay off: %d %s", answer.status, answer.body)
	}
	for _, fileID := range []int64{1, 2} {
		if inSettings, inPlayback := autoplayOf(t, user, fileID); inSettings || inPlayback {
			t.Errorf("turned off, file %d: autoplay %v in settings, %v in a playback; want off", fileID, inSettings,
				inPlayback)
		}
	}
	if inSettings, inPlayback := autoplayOf(t, other, 1); !inSettings || !inPlayback {
		t.Errorf("someone else: autoplay %v in settings, %v in a playback; want theirs, on", inSettings, inPlayback)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

func TestFilePreferences_ASubtitleOffsetIsKeptPerPersonAndFileAndTheNextPlaybackStartsWithIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	owner := harness.createAdminAndSignIn(t)
	_, err := harness.authService.CreateUser(t.Context(), auth.NewUser{
		Name: "guest", Password: "guest-password", Role: auth.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	guest := harness.signIn(t, "guest", "guest-password")
	harness.shelveFilms(t)
	if first := harness.play(t, owner, chrome(2)); first.Preferences.SubtitleOffsetMs != 0 {
		t.Errorf("before any choice: %+v, want no offset", first.Preferences)
	}
	set := func(token, fileID string, offset int) int {
		return harness.do(t, http.MethodPut, "/files/"+fileID+"/preferences", token,
			map[string]int{"subtitleOffsetMs": offset}).status
	}
	if status := set(owner, "2", -1500); status != http.StatusNoContent {
		t.Fatalf("PUT the offset: %d, want 204", status)
	}
	if again := harness.play(t, owner, chrome(2)); again.Preferences.SubtitleOffsetMs != -1500 {
		t.Errorf("the next playback: %+v, want the offset -1500 kept", again.Preferences)
	}
	if other := harness.play(t, guest, chrome(2)); other.Preferences.SubtitleOffsetMs != 0 {
		t.Errorf("another person's playback: %+v, want their own, none", other.Preferences)
	}
	if otherFile := harness.play(t, owner, chrome(1)); otherFile.Preferences.SubtitleOffsetMs != 0 {
		t.Errorf("another file: %+v, want its own, none", otherFile.Preferences)
	}
	if status := set(owner, "2", 250); status != http.StatusNoContent {
		t.Fatalf("PUT a second offset: %d", status)
	}
	if changed := harness.play(t, owner, chrome(2)); changed.Preferences.SubtitleOffsetMs != 250 {
		t.Errorf("after a change: %+v, want 250", changed.Preferences)
	}
	if status := set(owner, "3", 100); status != http.StatusNotFound {
		t.Errorf("a file that is not played: %d, want 404", status)
	}
	if status := set(owner, "2", 700000); status < 400 || status >= 500 {
		t.Errorf("an offset over ten minutes: %d, want refused", status)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
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

// A person's Show debug tools is off until they turn it on, kept on the server, said by GET /me on
// every device they sign in on, and theirs alone (the owner, 2026-10-29: "the per-user debug
// setting").
func TestPreferences_ShowDebugToolsIsOffUntilThePersonTurnsItOnAndFollowsThem(t *testing.T) {
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
	showsDebugTools := func(token string) bool {
		t.Helper()
		var person struct {
			ShowsDebugTools *bool `json:"showsDebugTools"`
		}
		harness.readJSON(t, "/me", token, &person)
		if person.ShowsDebugTools == nil {
			t.Fatal("GET /me says nothing of showsDebugTools")
		}
		return *person.ShowsDebugTools
	}
	if showsDebugTools(owner) {
		t.Error("before any choice: debug tools shown; want hidden")
	}
	set := func(isShown bool) {
		t.Helper()
		answer := harness.do(t, http.MethodPut, "/me/preferences", owner, map[string]bool{"showsDebugTools": isShown})
		if answer.status != http.StatusNoContent {
			t.Fatalf("PUT showsDebugTools %v: %d %s", isShown, answer.status, answer.body)
		}
	}
	set(true)
	if !showsDebugTools(owner) {
		t.Error("turned on: debug tools hidden; want shown")
	}
	if anotherDevice := harness.signIn(t, "owner", "owner-password"); !showsDebugTools(anotherDevice) {
		t.Error("turned on, another device's session: debug tools hidden; want them to follow the person")
	}
	if showsDebugTools(guest) {
		t.Error("another person: debug tools shown; want their own choice, none")
	}
	set(false)
	if showsDebugTools(owner) {
		t.Error("turned off again: debug tools shown; want hidden")
	}
}

// The sort a person chooses for a library is kept for that library and that person, and GET
// /libraries gives it back on every device they sign in on; another library and another person
// have none until they choose, and a library the person does not reach is none of theirs to sort
// (X5, the owner, 2026-11-12).
func TestLibraryPreferences_ASortChosenIsKeptPerLibraryAndPerson(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	owner := harness.createAdminAndSignIn(t)
	harness.shelve(t)            // library 1, films
	harness.shelveOtherVideos(t) // library 2, other videos
	storetest.ExecRaw(t, harness.databasePath,
		"DELETE FROM role_library WHERE library_id = 2 AND role_id = (SELECT id FROM role WHERE name = 'user')")
	for _, person := range []auth.NewUser{
		{Name: "partner", Password: "partner-password", Role: auth.RoleAdmin},
		{Name: "guest", Password: "guest-password", Role: auth.RoleUser},
	} {
		if _, err := harness.authService.CreateUser(t.Context(), person); err != nil {
			t.Fatal(err)
		}
	}
	partner := harness.signIn(t, "partner", "partner-password")
	guest := harness.signIn(t, "guest", "guest-password")
	choose := func(token string, libraryID int64, sort, order string, wantStatus int, what string) {
		t.Helper()
		answer := harness.do(t, http.MethodPut, fmt.Sprintf("/libraries/%d/preferences", libraryID), token,
			map[string]any{"sortChoice": map[string]string{"sort": sort, "order": order}})
		if answer.status != wantStatus {
			t.Fatalf("%s: PUT library %d's sort = %d %s; want %d",
				what, libraryID, answer.status, answer.body, wantStatus)
		}
	}
	if got := harness.librarySorts(t, owner); got != "map[]" {
		t.Errorf("before any choice: sorts %s; want none", got)
	}
	choose(owner, 1, "released", "ascending", http.StatusNoContent, "the first choice")
	if got, want := harness.librarySorts(t, owner), "map[1:released ascending]"; got != want {
		t.Errorf("library 1 sorted: sorts %s; want %s, library 2 still none", got, want)
	}
	choose(owner, 1, "added", "descending", http.StatusNoContent, "library 1 again")
	choose(owner, 2, "fileName", "descending", http.StatusNoContent, "library 2")
	want := "map[1:added descending 2:fileName descending]"
	if got := harness.librarySorts(t, owner); got != want {
		t.Errorf("both sorted, library 1 twice: sorts %s; want %s, the last choice each", got, want)
	}
	if got := harness.librarySorts(t, harness.signIn(t, "owner", "owner-password")); got != want {
		t.Errorf("another device's session: sorts %s; want %s, the person's", got, want)
	}
	if got := harness.librarySorts(t, partner); got != "map[]" {
		t.Errorf("another person: sorts %s; want their own, none", got)
	}
	choose(guest, 1, "name", "ascending", http.StatusNoContent, "a library the guest's role reaches")
	choose(guest, 2, "name", "ascending", http.StatusNotFound, "a library the person does not reach")
	if got, wantGuests := harness.librarySorts(t, guest), "map[1:name ascending]"; got != wantGuests {
		t.Errorf("the guest's sorts = %s; want %s, theirs alone", got, wantGuests)
	}
	if got := harness.librarySorts(t, owner); got != want {
		t.Errorf("the guest sorted library 1: the owner's sorts %s; want %s, unchanged", got, want)
	}
	choose(owner, 1, "rating", "ascending", http.StatusUnprocessableEntity, "a sort no list has")
}

// librarySorts are the sorts GET /libraries says the person chose, by library id: "sort order".
func (harness testHarness) librarySorts(t *testing.T, token string) string {
	t.Helper()
	var listed struct {
		Libraries []struct {
			ID         int64 `json:"id"`
			SortChoice *struct {
				Sort  string `json:"sort"`
				Order string `json:"order"`
			} `json:"sortChoice"`
		} `json:"libraries"`
	}
	harness.readJSON(t, "/libraries", token, &listed)
	sorts := map[int64]string{}
	for _, library := range listed.Libraries {
		if library.SortChoice != nil {
			sorts[library.ID] = library.SortChoice.Sort + " " + library.SortChoice.Order
		}
	}
	return fmt.Sprint(sorts)
}

// Switching Level loudness or the dialogue boost is logged with the device that switched it, as a
// switch made during a film changes its sound's route and the playback's own line says only how it
// started (the owner's TCL TV, 2026-11-06).
func TestPreferences_SwitchingTheSoundsLevellingIsLogged(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	owner := harness.createAdminAndSignIn(t)
	answer := harness.do(t, http.MethodPut, "/me/preferences", owner,
		map[string]bool{"isLevellingOn": true, "isDialogueBoostOn": false})
	if answer.status != http.StatusNoContent {
		t.Fatalf("PUT the sound's switches: %d %s", answer.status, answer.body)
	}
	logged := harness.logs.String()
	for _, want := range []string{
		testutil.EventInJSON("levellingSwitched"), `"isLevellingOn":true`,
		testutil.EventInJSON("dialogueBoostSwitched"), `"isDialogueBoostOn":false`, `"deviceId":`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log has no %s:\n%s", want, logged)
		}
	}
}

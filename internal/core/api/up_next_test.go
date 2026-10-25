// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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

// upNextSeen is GET /files/{id}/up-next's answer in short: "title:group:number file still ordering",
// or "none".
func (harness testHarness) upNextSeen(t *testing.T, token string, fileID int64) string {
	t.Helper()
	var answer struct {
		UpNext *struct {
			Episode struct {
				TitleID       int64 `json:"titleId"`
				GroupNumber   int64 `json:"groupNumber"`
				NumberInGroup int64 `json:"numberInGroup"`
				FileID        int64 `json:"fileId"`
			} `json:"episode"`
			Ordering    string `json:"ordering"`
			StillSha256 string `json:"stillSha256"`
		} `json:"upNext"`
	}
	harness.readJSON(t, fmt.Sprintf("/files/%d/up-next", fileID), token, &answer)
	if answer.UpNext == nil {
		return "none"
	}
	next := answer.UpNext
	return fmt.Sprintf("%d:%d:%d file %d still %.4s %s", next.Episode.TitleID, next.Episode.GroupNumber,
		next.Episode.NumberInGroup, next.Episode.FileID, cmp.Or(next.StillSha256, "none"), next.Ordering)
}

// The player's Up next is the episode after the one playing, in the ordering its person watches the
// series in, with its still, and its Play's file: the one they are partway through (D160, D159 §1;
// plans/up-next.md). An episode the ordering leaves out, the last, and a film have none.
func TestUpNext_IsTheNextEpisodeInThePersonsOrdering(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t) // aired: 21 (file 2), 22 (3), 23 (4), then 24 (5); DVD: 23, 21, 24
	for fileID, want := range map[int64]string{
		2: "22:1:2 file 3 still none aired",
		4: "24:2:1 file 5 still none aired",
		5: "none",
		1: "none", // Metropolis, a film
	} {
		if got := harness.upNextSeen(t, user, fileID); got != want {
			t.Errorf("aired, after file %d: %s; want %s", fileID, got, want)
		}
	}
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO preference (user_id, scope, scope_id, axis, value_json,
		set_at) VALUES (1, 'work', 20, 'episode_ordering', '"tmdb:dvd"', 1)`)
	for fileID, want := range map[int64]string{
		4: "21:1:2 file 2 still dddd tmdb:dvd",
		2: "24:2:1 file 5 still none tmdb:dvd",
		3: "none", // 22, which the DVD order leaves out
	} {
		if got := harness.upNextSeen(t, user, fileID); got != want {
			t.Errorf("DVD order, after file %d: %s; want %s", fileID, got, want)
		}
	}
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO version (id, title_id, created_at) VALUES (6, 21, 3)",
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id, first_seen_at,
		 last_seen_at) VALUES (6, 1, 1, '/m/BB/S01E01 again.mkv', 10, X'06', 'listed', 6, 1, 1)`,
		"INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (6, '{}', 1000)")
	harness.setPositionReadAt(t, 6, 500, 9)
	if got, want := harness.upNextSeen(t, user, 4), "21:1:2 file 6 still dddd tmdb:dvd"; got != want {
		t.Errorf("partway through the next one's second file: %s; want %s, which its Play resumes", got, want)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
)

// The admin sees each playback heard from lately, who plays what and how, and stops one with a
// message: its player is told in the answer to its next report, and its bytes stop (cut-a §5, D243
// §9).
func TestNowPlaying_TheAdminSeesAPlaybackAndStopsItWithAMessage(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	playing := harness.play(t, partner, chrome(1))
	shown := harness.onlyPlaying(t, admin)
	if shown.UserName != "partner" || shown.FileID != 1 || shown.Delivery != "direct" ||
		!strings.Contains(shown.Name, "Film (2001)") || shown.DeviceName == "" {
		t.Errorf("now playing shows %+v; want the partner, on their device, playing film 1 directly", shown)
	}
	wantError(t, harness.do(t, http.MethodGet, "/now-playing", partner, nil), http.StatusForbidden, api.CodeForbidden)

	harness.clock.Advance(31 * time.Second)
	if quiet := harness.do(t, http.MethodGet, "/now-playing", admin, nil); strings.Contains(quiet.body, `"fileId":1`) {
		t.Errorf("now playing after half a minute unheard = %s; want it gone", quiet.body)
	}
	if sent := harness.report(t, partner, playing, reportBody(1, 60_000, 0)); sent.status != http.StatusOK ||
		strings.Contains(sent.body, "stop") {
		t.Fatalf("a report = %d %s; want it taken", sent.status, sent.body)
	}
	stopped := harness.do(t, http.MethodPost, fmt.Sprintf("/now-playing/%d/stop", shown.ID), admin,
		map[string]any{"message": "The server is going down for a minute."})
	if stopped.status != http.StatusNoContent {
		t.Fatalf("POST /now-playing/%d/stop = %d %s", shown.ID, stopped.status, stopped.body)
	}
	if bytes := harness.getBytePath(t, playing.URL, partner, true, ""); bytes.status != http.StatusNotFound {
		t.Errorf("a stopped playback's bytes = %d; want refused", bytes.status)
	}
	told := harness.report(t, partner, playing, reportBody(2, 70_000, 0))
	if told.status != http.StatusOK || !strings.Contains(told.body, `"stop":{"message":"The server is going down`) {
		t.Errorf("the next report's answer = %d %s; want the stop and its message", told.status, told.body)
	}
	wantError(t, harness.report(t, partner, playing, reportBody(3, 80_000, 0)), http.StatusNotFound,
		api.CodePlaybackGone)
	if after := harness.do(t, http.MethodGet, "/now-playing", admin, nil); strings.Contains(after.body, `"fileId":1`) {
		t.Errorf("now playing after the stop = %s; want it gone", after.body)
	}
}

// onlyPlaying is the one playback GET /now-playing lists, which fails the test when there is not one.
func (harness testHarness) onlyPlaying(t *testing.T, admin string) api.NowPlayingBody {
	t.Helper()
	listed := harness.do(t, http.MethodGet, "/now-playing", admin, nil)
	var nowPlaying api.NowPlayingOutput
	if err := json.Unmarshal([]byte(listed.body), &nowPlaying.Body); err != nil || len(nowPlaying.Body.Playbacks) != 1 {
		t.Fatalf("GET /now-playing = %d %s; want one playback", listed.status, listed.body)
	}
	return nowPlaying.Body.Playbacks[0]
}

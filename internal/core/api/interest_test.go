// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// A player that moves to another quality stops the run of the one it left: one player wants one
// quality at a time (D309 §1, §2). On nas.example two runs of 720p left from before the player moved to
// 360p went on, one measured slow, and the 360p run it needed was evicted in their place (the owner's
// Wings, 2026-10-17). Its word that it paused, by its own token, is taken; another's is refused.
func TestInterest_ARunOfAQualityThePlayerLeftStops(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	token := harness.play(t, user, chrome(1)).MediaToken
	harness.keepFilmOnesTimeline(t)
	harness.measureFilmOnesLoudness(t)
	base := api.Prefix + "/bytes/media/chunks/" + token + "/"
	for _, stream := range []string{"q700k432p", "q400k360p"} {
		if answer := harness.getBytePath(t, base+stream+"/00001.m4s", user, true, ""); answer.status !=
			http.StatusTooEarly {
			t.Fatalf("%s's chunk 1, not made yet: %d %s; want 425", stream, answer.status, answer.body)
		}
	}
	harness.logs.AwaitLine(t, `"msg":"stopped a run of chunks no player wants"`,
		`"storeDir":"1/chunks-q700k432p/`, `"firstChunk":1,`)

	said := map[string]any{"pictureStream": "q400k360p", "positionMs": 10_000, "isPaused": true}
	if answer := harness.do(t, http.MethodPut, "/playback/"+token+"/interest", user, said); answer.status !=
		http.StatusNoContent {
		t.Errorf("the player's word: %d %s; want 204", answer.status, answer.body)
	}
	if answer := harness.do(t, http.MethodPut, "/playback/not-a-token/interest", user, said); answer.status !=
		http.StatusNotFound {
		t.Errorf("a word on a playback not its own: %d %s; want 404", answer.status, answer.body)
	}
	// Leaving, it says why: its page, or another film (D309 §7); nothing else.
	if answer := harness.do(t, http.MethodDelete, "/playback/"+token+"?reason=sideways", user, nil); answer.status !=
		http.StatusUnprocessableEntity {
		t.Errorf("a leave for no reason it knows: %d %s; want 422", answer.status, answer.body)
	}
	if answer := harness.do(t, http.MethodDelete, "/playback/"+token+"?reason=replaced", user, nil); answer.status !=
		http.StatusNoContent {
		t.Errorf("a leave for another film: %d %s; want 204", answer.status, answer.body)
	}
	harness.logs.AwaitLine(t, `"msg":"a player left its playback"`, `"reason":"replaced"`)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A player that moves to another quality stops the run of the one it left: one player wants one
// quality at a time (D309 §1, §2). On the production NAS two runs of 720p left from before the player moved to
// 360p went on, one measured slow, and the 360p run it needed was evicted in their place (the owner's
// Arkons, 2026-10-17). Its word that it paused, by its own token, is taken; another's is refused.
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
	harness.logs.AwaitEvent(t, "unwantedChunkRunStopped",
		`"storeDir":"1/chunks-q700k432p/`, `"firstChunk":1,`)

	said := map[string]any{"pictureStream": "q400k360p", "positionMs": 10_000, "isPaused": true}
	if answer := harness.do(t, http.MethodPut, "/playback/"+token+"/wants", user, said); answer.status !=
		http.StatusNoContent {
		t.Errorf("the player's word: %d %s; want 204", answer.status, answer.body)
	}
	if answer := harness.do(t, http.MethodPut, "/playback/not-a-token/wants", user, said); answer.status !=
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
	harness.logs.AwaitEvent(t, "playerLeft", `"reason":"replaced"`)
}

// A player paused where a seek took it wants the chunk it asks for there, though the chunk starts
// before where it paused: hls.js, sought onto a chunk's very start, steps back to the chunk before
// when what it loaded starts a moment after its position. A Watch Together group waiting on a jump
// had paused its players exactly on chunk 205's start, its run had made 205 to 210 and stopped at
// their keep-ahead, and every player then asked for 204 where the seek took it, planned no run as
// behind where each wanted, for good (staging, 2026-10-30, a cast receiver leading).
func TestInterest_APausedPlayersSoughtAskBehindWhereItPausedStartsARunThere(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	token := harness.play(t, user, chrome(1)).MediaToken
	harness.keepFilmOnesTimeline(t)
	grid, _, err := harness.chunks.Grid(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	// Chunk 2, where the seek landed, is made, and the player loaded it.
	harness.nodeBytes.mutex.Lock()
	storeKey := harness.filmOnesStoreKey(t, "q700k432p", grid.ID())
	harness.nodeBytes.files[nodeproto.PathBytesChunks+"1/chunks-q700k432p/"+storeKey+"/00002.m4s"] = "chunk two"
	harness.nodeBytes.mutex.Unlock()
	base := api.Prefix + "/bytes/media/chunks/" + token + "/q700k432p/"
	if made := harness.getSought(t, base+"00002.m4s", user); made != http.StatusOK {
		t.Fatalf("the seek's chunk, made: %d; want 200", made)
	}
	// Its group waits, its player paused exactly on chunk 2's start.
	said := map[string]any{"pictureStream": "q700k432p", "positionMs": grid.StartsMs[2], "isPaused": true}
	if answer := harness.do(t, http.MethodPut, "/playback/"+token+"/wants", user, said); answer.status !=
		http.StatusNoContent {
		t.Fatalf("the player's word, paused: %d %s; want 204", answer.status, answer.body)
	}
	if before := harness.getSought(t, base+"00001.m4s", user); before != http.StatusTooEarly {
		t.Fatalf("the chunk before, not made yet: %d; want 425", before)
	}
	// The run is queued before the player is answered.
	runs := harness.openJobs(t, jobs.KindMakeChunks)
	if len(runs) != 1 || runs[0].ArgsJSON != `{"firstChunk":1,"lastChunk":1}` {
		t.Errorf("the runs queued: %+v; want one of chunk 1 alone:\n%s", runs, harness.logs.String())
	}
}

// getSought asks for a store's file at path as a player loading where a seek took it says it does
// (Orpheum-Sought, D284 §2), and returns its answer's status.
func (harness testHarness) getSought(t *testing.T, path, user string) int {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, harness.httpServer.URL+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+user)
	request.Header.Set("Orpheum-Sought", "true")
	answer, err := harness.httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = answer.Body.Close() //nolint:errcheck // its status is all that is read
	return answer.StatusCode
}

// A player that started before the core's restart says its wants at the old path until its page
// reloads, so the path is served for one release, outside the document (D425 §3); it goes in the
// next batch.
func TestPlaybackWants_TheOldPathIsServedOutsideTheDocumentForOneRelease(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	token := harness.play(t, user, chrome(1)).MediaToken
	said := map[string]any{"pictureStream": "q400k360p", "positionMs": 10_000, "isPaused": true}
	if answer := harness.do(t, http.MethodPut, "/playback/"+token+"/interest", user, said); answer.status !=
		http.StatusNoContent {
		t.Errorf("the player's word at the old path: %d %s; want 204", answer.status, answer.body)
	}
	if answer := harness.do(t, http.MethodPut, "/playback/not-a-token/interest", user, said); answer.status !=
		http.StatusNotFound {
		t.Errorf("a word at the old path on a playback not its own: %d %s; want 404", answer.status, answer.body)
	}
	for _, operation := range api.OperationsIn(harness.server.OpenAPI()) {
		if operation.Path() == "/playback/{token}/interest" {
			t.Errorf("the document lists the old path as %s; want it served outside the document", operation.ID())
		}
	}
}

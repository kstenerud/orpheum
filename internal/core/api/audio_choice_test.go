// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

// chooseAudio chooses an audio track in a file.
func (harness testHarness) chooseAudio(t *testing.T, sessionToken string, fileID, streamID int64) response {
	t.Helper()
	return harness.do(t, http.MethodPut, fmt.Sprintf("/files/%d/preferences", fileID), sessionToken,
		map[string]any{"audioChoice": map[string]any{"streamId": streamID}})
}

// Audio follows by the same rule (D230 §5): the language chosen in one episode is the one an
// unwatched episode starts with, which a browser plays only prepared, and a started one plays again
// with what it used.
func TestAudioChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveEpisodes(t)
	if got := harness.startIn(t, user, 5); got.audio != 82 || slices.Contains(got.reasons, "audio_track") {
		t.Errorf("before any choice, episode 2 starts with audio %d (%v), want its default English, 82", got.audio,
			got.reasons)
	}
	harness.watch(t, user, 6) // episode 3 is started, with its default English
	if answer := harness.chooseAudio(t, user, 2, 81); answer.status != http.StatusNoContent {
		t.Fatalf("choosing French sound in the pilot: %d %s", answer.status, answer.body)
	}
	unwatched := harness.startIn(t, user, 5)
	if unwatched.audio != 83 || !slices.Contains(unwatched.reasons, "audio_track") {
		t.Errorf("after French in the pilot, episode 2 starts with audio %d (%v), want French, 83, prepared "+
			"because it is not the track the original starts with", unwatched.audio, unwatched.reasons)
	}
	harness.addEpisode(t, 24, 7, "(90, 7, 7, 1, 'audio', 'aac', 'en', 1), (91, 7, 7, 4, 'audio', 'aac', 'fr', 0)")
	if got := harness.startIn(t, user, 7).audio; got != 91 {
		t.Errorf("an episode added after the choice starts with audio %d, want its French, 91", got)
	}
	if got := harness.startIn(t, user, 6).audio; got != 84 {
		t.Errorf("episode 3, started with English, starts again with audio %d, want its English, 84", got)
	}
	if answer := harness.chooseAudio(t, user, 2, 84); answer.status != http.StatusBadRequest {
		t.Errorf("choosing another file's audio track: %d, want 400", answer.status)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// segmentsSeen is GET /files/{id}/segments's answer.
func (harness testHarness) segmentsSeen(t *testing.T, token string, fileID int64) []api.SegmentBody {
	t.Helper()
	var answer struct {
		Segments []api.SegmentBody `json:"segments"`
	}
	harness.readJSON(t, fmt.Sprintf("/files/%d/segments", fileID), token, &answer)
	return answer.Segments
}

// shelveSegments gives film 1 an intro and credits, and film 2 a recap, with a range of no known type
// and an episode boundary on film 1, which are not sent.
func (harness testHarness) shelveSegments(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO media_segment (file_id, type, start_ms, end_ms, provider, confidence) VALUES "+
			"(1, 'credits', 1100000, 1200000, 'chapters', 1), (1, 'intro', 0, 90000, 'chapters', 1), "+
			"(1, 'unknown', 300000, 310000, 'chapters', 1), "+
			"(1, 'episode_boundary', 600000, 600001, 'chapters', 1), (2, 'recap', 0, 30000, 'chapters', 1)")
}

// A file's segments are the ones its playback's decision carries, to anyone reaching it, so a cast's
// remote offers the skips and Up next from the credits as the player does (D546); a file with none
// has an empty list, and no one signed in is refused. A file out of a person's reach is gone for them
// (TestReach_ALibraryTakenBackIsGoneAtTheNextRequestEverywhere).
func TestFileSegments_AreThePlaybacksToAnyoneReachingTheFile(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	if none := harness.segmentsSeen(t, user, 1); none == nil || len(none) != 0 {
		t.Errorf("a file with no segments has %+v; want an empty list", none)
	}
	harness.shelveSegments(t)
	want := []api.SegmentBody{
		{Type: "intro", StartMs: 0, EndMs: 90000}, {Type: "credits", StartMs: 1100000, EndMs: 1200000},
	}
	if got := harness.segmentsSeen(t, user, 1); !slices.Equal(got, want) {
		t.Errorf("the segments = %+v; want %+v", got, want)
	}
	if played := harness.play(t, user, chrome(1)).Segments; !slices.Equal(played, want) {
		t.Errorf("the playback's segments = %+v; want the file's, %+v", played, want)
	}
	wantError(t, harness.do(t, http.MethodGet, "/files/1/segments", "", nil), http.StatusUnauthorized,
		api.CodeAuthRequired)
}

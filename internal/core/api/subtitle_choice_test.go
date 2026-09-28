// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelveEpisodes adds two more episodes to shelve's series: the pilot (file 2) has English and
// Spanish subtitles, episode 2 (file 5) English and French, episode 3 (file 6) French only.
func (harness testHarness) shelveEpisodes(t *testing.T) {
	t.Helper()
	const probe = `'{"format":{"names":["matroska"]},"streams":[{"index":0,"kind":"video","codec":"h264",` +
		`"profile":"High","video":{"pixelFormat":"yuv420p"}}]}'`
	storetest.ExecRaw(t, harness.database,
		"INSERT INTO item (id, kind, library_id, created_at) VALUES (22, 'title', 1, 3), (23, 'title', 1, 3)",
		`INSERT INTO title (id, kind, identification)
		 VALUES (22, 'episode', 'identified'), (23, 'episode', 'identified')`,
		"INSERT INTO version (id, title_id, created_at) VALUES (5, 22, 3), (6, 23, 3)",
		`INSERT INTO file (id, library_id, node_id, path, path_key, size_bytes, filing, version_id, first_seen_at,
		 last_seen_at) VALUES
		 (5, 1, 1, '/m/BB/S01E02.mkv', CAST('/m/BB/S01E02.mkv' AS BLOB), 10, 'listed', 5, 1, 1),
		 (6, 1, 1, '/m/BB/S01E03.mkv', CAST('/m/BB/S01E03.mkv' AS BLOB), 10, 'listed', 6, 1, 1)`,
		"UPDATE file SET payload_hash = X'0102', probe = "+probe+" WHERE id IN (2, 5, 6)",
		"UPDATE file SET path_key = CAST(path AS BLOB) WHERE id = 2",
		"INSERT INTO library_root (library_id, node_id, path) VALUES (1, 1, '/m')",
		"INSERT INTO collection_member (collection_id, member_title, position) VALUES (20, 22, 10002), (20, 23, 10003)",
		`INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, lang) VALUES
		 (50, 2, 2, 2, 'subtitle', 'subrip', 'en'), (51, 2, 2, 3, 'subtitle', 'subrip', 'es'),
		 (52, 5, 5, 2, 'subtitle', 'ass', 'en'), (53, 5, 5, 3, 'subtitle', 'ass', 'fr'),
		 (54, 6, 6, 2, 'subtitle', 'subrip', 'fr')`)
}

// choose makes a subtitle choice in a file: a track, or off when streamID is 0.
func (harness testHarness) choose(t *testing.T, session string, fileID, streamID int64) response {
	t.Helper()
	choice := map[string]any{"isOff": true}
	if streamID != 0 {
		choice = map[string]any{"streamId": streamID}
	}
	return harness.do(t, http.MethodPut, fmt.Sprintf("/files/%d/prefs", fileID), session,
		map[string]any{"subtitle": choice})
}

// choiceIn is the subtitle choice a playback of a file starts with: "none", "off", or the track
// and whether it was carried.
func (harness testHarness) choiceIn(t *testing.T, session string, fileID int64) string {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/playback", session, chrome(fileID))
	var decided struct {
		Prefs FilePrefsBody `json:"prefs"`
	}
	if err := json.Unmarshal([]byte(answer.body), &decided); err != nil || answer.status != http.StatusOK {
		t.Fatalf("POST /playback of file %d = %d %s", fileID, answer.status, answer.body)
	}
	choice := decided.Prefs.Subtitle
	switch {
	case choice == nil:
		return "none"
	case choice.IsOff:
		return fmt.Sprintf("off carried=%v", choice.IsCarried)
	}
	if choice.FetchedID != 0 {
		return fmt.Sprintf("fetched %d carried=%v", choice.FetchedID, choice.IsCarried)
	}
	return fmt.Sprintf("%d carried=%v", choice.StreamID, choice.IsCarried)
}

func TestSubtitleChoice_IsKeptPerFileAndCarriedToTheSeriesFilesWithNoneOfTheirOwn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveEpisodes(t)
	check := func(when string, want map[int64]string) {
		t.Helper()
		for fileID, wanted := range want {
			if got := harness.choiceIn(t, user, fileID); got != wanted {
				t.Errorf("%s, file %d starts with %q, want %q", when, fileID, got, wanted)
			}
		}
	}
	if answer := harness.choose(t, user, 2, 50); answer.status != http.StatusNoContent {
		t.Fatalf("choosing English in the pilot: %d %s", answer.status, answer.body)
	}
	check("after English in the pilot", map[int64]string{
		2: "50 carried=false", 5: "52 carried=true", 6: "none", // episode 3 has no English: no choice
	})
	harness.choose(t, user, 6, 54) // French in episode 3: its own
	harness.choose(t, user, 2, 51) // Spanish in the pilot
	check("after Spanish in the pilot", map[int64]string{
		2: "51 carried=false", 5: "none", 6: "54 carried=false", // no Spanish in 2; 3's own French stands
	})
	harness.choose(t, user, 5, 0) // off in episode 2: its own
	harness.choose(t, user, 2, 50)
	check("after English again", map[int64]string{5: "off carried=false", 6: "54 carried=false"})
	harness.choose(t, user, 6, 0)
	harness.choose(t, user, 2, 0) // off in the pilot carries as off, but not over the others' own
	check("after off in the pilot", map[int64]string{2: "off carried=false", 5: "off carried=false"})
	if answer := harness.choose(t, user, 2, 52); answer.status != http.StatusBadRequest {
		t.Errorf("choosing another file's track: %d, want 400", answer.status)
	}
}

func TestSubtitleChoice_IsFoundAgainWhenARescanRewritesTheFilesTracks(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	session := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveEpisodes(t)
	if status := harness.choose(t, session, 5, 53).status; status != http.StatusNoContent {
		t.Fatalf("choosing French in episode 2: %d", status)
	}
	// A rescan writes the file's streams again under new ids; the French track is still there.
	storetest.ExecRaw(t, harness.database,
		"DELETE FROM stream WHERE bytes_file = 5",
		`INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, lang) VALUES
		 (62, 5, 5, 2, 'subtitle', 'ass', 'en'), (63, 5, 5, 3, 'subtitle', 'ass', 'fr')`)
	if got := harness.choiceIn(t, session, 5); got != "63 carried=false" {
		t.Errorf("after the rescan: %s, want the French track under its new id", got)
	}
	// Episode 3 had French carried to it; its French track goes, and English comes.
	storetest.ExecRaw(t, harness.database,
		"DELETE FROM stream WHERE bytes_file = 6",
		`INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, lang)
		 VALUES (64, 6, 6, 2, 'subtitle', 'subrip', 'en')`)
	if got := harness.choiceIn(t, session, 6); got != "none" {
		t.Errorf("episode 3 without French: %s, want no choice rather than a track that is gone", got)
	}
}

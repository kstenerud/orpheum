// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// episodeProbe is an episode's stored probe: a picture, and English and French sound, the English
// marked default; it has a length, so a player that cannot play it as it is plays it cut on time.
const episodeProbe = `{"format":{"formatNames":["matroska"],"durationMs":1200000},"streams":[` +
	`{"streamIndex":0,"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
	`{"streamIndex":1,"kind":"audio","codec":"aac","language":"en","disposition":["default"],` +
	`"audio":{"channelCount":2}},` +
	`{"streamIndex":4,"kind":"audio","codec":"aac","language":"fr","audio":{"channelCount":2}}]}`

// shelveEpisodes adds two more episodes to shelve's series: the pilot (file 2) has English and
// Spanish subtitles, episode 2 (file 5) English and French, episode 3 (file 6) French only. Each
// has English sound, its default, and French.
func (harness testHarness) shelveEpisodes(t *testing.T) {
	t.Helper()
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 22, Library: 1, Kind: "episode", AddedAt: 3})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 23, Library: 1, Kind: "episode", AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 5, TitleID: 22, CreatedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 6, TitleID: 23, CreatedAt: 3})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 5, LibraryID: 1, NodeID: 1, Path: "/m/BB/S01E02.mkv", SizeBytes: 10, Filing: "listed", VersionID: 5,
		PayloadHash: []byte{0x01, 0x02},
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 6, LibraryID: 1, NodeID: 1, Path: "/m/BB/S01E03.mkv", SizeBytes: 10, Filing: "listed", VersionID: 6,
		PayloadHash: []byte{0x01, 0x02},
	})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 2, ProbeJSON: episodeProbe})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 5, ProbeJSON: episodeProbe})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 6, ProbeJSON: episodeProbe})
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE file SET payload_hash = X'0102' WHERE id = 2",
		"UPDATE file SET reported_path_raw = CAST(path AS BLOB) WHERE id = 2",
		"INSERT INTO sequence_member (sequence_id, member_work, position) VALUES (20, 22, 10002), (20, 23, 10003)")
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 50, BytesFile: 2, StreamIndex: 2, Kind: "subtitle", Codec: "subrip", Language: "en",
	})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 51, BytesFile: 2, StreamIndex: 3, Kind: "subtitle", Codec: "subrip", Language: "es",
	})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 52, BytesFile: 5, StreamIndex: 2, Kind: "subtitle", Codec: "ass", Language: "en",
	})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 53, BytesFile: 5, StreamIndex: 3, Kind: "subtitle", Codec: "ass", Language: "fr",
	})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 54, BytesFile: 6, StreamIndex: 2, Kind: "subtitle", Codec: "subrip", Language: "fr",
	})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 80, BytesFile: 2, StreamIndex: 1, Kind: "audio", Codec: "aac", Language: "en", DispositionFlags: 1,
	})
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{ID: 81, BytesFile: 2, StreamIndex: 4, Kind: "audio", Codec: "aac", Language: "fr"})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 82, BytesFile: 5, StreamIndex: 1, Kind: "audio", Codec: "aac", Language: "en", DispositionFlags: 1,
	})
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{ID: 83, BytesFile: 5, StreamIndex: 4, Kind: "audio", Codec: "aac", Language: "fr"})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 84, BytesFile: 6, StreamIndex: 1, Kind: "audio", Codec: "aac", Language: "en", DispositionFlags: 1,
	})
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{ID: 85, BytesFile: 6, StreamIndex: 4, Kind: "audio", Codec: "aac", Language: "fr"})
}

// addEpisode adds an episode to shelve's series after the others, as a scan that finds it later
// does: its title, its file, and the file's tracks.
func (harness testHarness) addEpisode(t *testing.T, titleID, fileID int64, tracks string) {
	t.Helper()
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: titleID, Library: 1, Kind: "episode", AddedAt: 4})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: fileID, TitleID: titleID, CreatedAt: 4})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: fileID, LibraryID: 1, NodeID: 1, Path: fmt.Sprintf("/m/BB/E%d.mkv", fileID), SizeBytes: 10,
		Filing: "listed", VersionID: fileID, FirstSeenAt: 4, LastSeenAt: 4, PayloadHash: []byte{0x01, 0x02},
	})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: fileID, ProbeJSON: episodeProbe})
	storetest.ExecRaw(t, harness.databasePath,
		fmt.Sprintf("INSERT INTO sequence_member (sequence_id, member_work, position) VALUES (20, %d, %d)",
			titleID, 10000+titleID),
		// tracks is each caller's own VALUES fragment, of however many tracks it wants, under
		// chosen ids: left as raw SQL rather than Stream, one call per row.
		"INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, language, disposition_flags) "+
			"VALUES "+tracks)
}

// choose makes a subtitle choice in a file: a track, or off when streamID is 0.
func (harness testHarness) choose(t *testing.T, sessionToken string, fileID, streamID int64) response {
	t.Helper()
	choice := map[string]any{"isOff": true}
	if streamID != 0 {
		choice = map[string]any{"streamId": streamID}
	}
	return harness.do(t, http.MethodPut, fmt.Sprintf("/files/%d/preferences", fileID), sessionToken,
		map[string]any{"subtitleChoice": choice})
}

// started is what a playback of a file starts with: its subtitle track, "none" or the stream, and
// its audio track, with the decision's media token and reasons.
type started struct {
	subtitle   string
	audio      int64
	mediaToken string
	reasons    []string
	answer     decision
}

// startIn starts a playback of a file.
func (harness testHarness) startIn(t *testing.T, sessionToken string, fileID int64) started {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/playback", sessionToken, chrome(fileID))
	var decided struct {
		decision
		StartingAudioStreamID int64                       `json:"startingAudioStreamId"`
		Preferences           api.PlaybackPreferencesBody `json:"preferences"`
	}
	if err := json.Unmarshal([]byte(answer.body), &decided); err != nil || answer.status != http.StatusOK {
		t.Fatalf("POST /playback of file %d = %d %s", fileID, answer.status, answer.body)
	}
	result := started{
		subtitle: "none", audio: decided.StartingAudioStreamID, mediaToken: decided.MediaToken,
		reasons: decided.NotDirectReasons, answer: decided.decision,
	}
	if choice := decided.Preferences.SubtitleChoice; choice != nil {
		switch {
		case choice.IsOff:
			t.Errorf("file %d starts with subtitles off as a track; want no track", fileID)
		case choice.FetchedSubtitleID != 0:
			result.subtitle = fmt.Sprintf("fetched %d", choice.FetchedSubtitleID)
		default:
			result.subtitle = fmt.Sprint(choice.StreamID)
		}
	}
	return result
}

// watch starts a playback of a file and reports it playing: the file is started (D230 §2).
func (harness testHarness) watch(t *testing.T, sessionToken string, fileID int64) {
	t.Helper()
	playing := harness.startIn(t, sessionToken, fileID)
	if answer := harness.report(t, sessionToken, playing.answer, reportBody(1, 5000, 10)); answer.status !=
		http.StatusOK {
		t.Fatalf("reporting file %d playing: %d %s", fileID, answer.status, answer.body)
	}
}

// subtitlesIn are the subtitle tracks playbacks of files start with, by file.
func (harness testHarness) subtitlesIn(t *testing.T, sessionToken string, fileIDs ...int64) map[int64]string {
	t.Helper()
	found := map[int64]string{}
	for _, fileID := range fileIDs {
		found[fileID] = harness.startIn(t, sessionToken, fileID).subtitle
	}
	return found
}

// The owner's scenario (D230 §1): English chosen in the first episode plays in every unwatched one
// that has it, episodes added later and a file whose tracks changed included; French chosen later
// in another makes the unwatched ones French; an episode already started plays again with what it
// used.
func TestSubtitleChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveEpisodes(t)
	check := func(when string, want map[int64]string) {
		t.Helper()
		for fileID, wanted := range want {
			if got := harness.subtitlesIn(t, user, fileID)[fileID]; got != wanted {
				t.Errorf("%s, file %d starts with %q, want %q", when, fileID, got, wanted)
			}
		}
	}
	check("before any choice", map[int64]string{2: "none", 5: "none"}) // subtitles start off (D223 §3)
	if answer := harness.choose(t, user, 2, 50); answer.status != http.StatusNoContent {
		t.Fatalf("choosing English in the pilot: %d %s", answer.status, answer.body)
	}
	// Episode 3 has no English, so it plays without subtitles: a fallback is not a choice (D24).
	check("after English in the pilot", map[int64]string{2: "50", 5: "52", 6: "none"})
	harness.watch(t, user, 5) // episode 2 is started with English
	// Episode 4 arrives after the choice, and episode 3's file gains an English track.
	harness.addEpisode(t, 24, 7,
		"(55, 7, 7, 2, 'subtitle', 'subrip', 'en', 0), (56, 7, 7, 3, 'subtitle', 'subrip', 'fr', 0)")
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{ID: 57, BytesFile: 6, StreamIndex: 3, Kind: "subtitle", Codec: "subrip", Language: "en"})
	check("after episode 4 came and episode 3 gained English", map[int64]string{7: "55", 6: "57"})
	if answer := harness.choose(t, user, 6, 54); answer.status != http.StatusNoContent {
		t.Fatalf("choosing French in episode 3: %d %s", answer.status, answer.body)
	}
	check("after French in episode 3", map[int64]string{
		6: "54", 7: "56", // chosen there, and unwatched
		5: "52", // started with English, so it plays again with English
		2: "50", // chosen there
	})
	harness.choose(t, user, 2, 0) // off in the pilot follows as off
	check("after off in the pilot", map[int64]string{2: "none", 7: "none", 6: "54", 5: "52"})
	if answer := harness.choose(t, user, 2, 52); answer.status != http.StatusBadRequest {
		t.Errorf("choosing another file's track: %d, want 400", answer.status)
	}
}

// A language, not a track (D230 §3): of the tracks in the chosen language, the one of the same
// kind is taken, then the one of the same title.
func TestSubtitleChoice_AnUnwatchedEpisodeTakesTheSameKindThenTheSameTitle(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveEpisodes(t)
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE stream SET container_title = 'Full', disposition_flags = 128 WHERE id = 50`) // for the deaf
	harness.addEpisode(t, 24, 7, `(60, 7, 7, 2, 'subtitle', 'subrip', 'en', 64), (61, 7, 7, 3, 'subtitle',
		'subrip', 'en', 0), (62, 7, 7, 4, 'subtitle', 'subrip', 'en', 128), (63, 7, 7, 5, 'subtitle', 'subrip',
		'en', 128)`)
	storetest.ExecRaw(t, harness.databasePath, "UPDATE stream SET container_title = 'Full' WHERE id = 63")
	harness.choose(t, user, 2, 50)
	if got := harness.subtitlesIn(t, user, 7)[7]; got != "63" {
		t.Errorf("English for the deaf titled Full, chosen: episode 4 starts with %s, want 63, of that kind and title",
			got)
	}
}

// A started file's track is found again by what it is when a rescan writes its streams anew, and
// one whose track went follows the work again.
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
	storetest.ExecRaw(t, harness.databasePath, "DELETE FROM stream WHERE bytes_file = 5 AND kind = 'subtitle'")
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{ID: 62, BytesFile: 5, StreamIndex: 2, Kind: "subtitle", Codec: "ass", Language: "en"})
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{ID: 63, BytesFile: 5, StreamIndex: 3, Kind: "subtitle", Codec: "ass", Language: "fr"})
	if got := harness.subtitlesIn(t, session, 5)[5]; got != "63" {
		t.Errorf("after the rescan: %s, want the French track under its new id", got)
	}
	harness.choose(t, session, 2, 50) // English in the pilot is the work's language now
	storetest.ExecRaw(t, harness.databasePath, "DELETE FROM stream WHERE bytes_file = 5 AND kind = 'subtitle'")
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{ID: 64, BytesFile: 5, StreamIndex: 2, Kind: "subtitle", Codec: "ass", Language: "en"})
	if got := harness.subtitlesIn(t, session, 5)[5]; got != "64" {
		t.Errorf("episode 2 without its French: %s, want the work's English rather than a track that is gone", got)
	}
}

// What is kept is read from the axes D230 names: a work's subtitle_language, as a language, kind
// and containerTitle, and a file's subtitle_used, a track and what it is.
func TestSubtitleChoice_IsReadFromTheWorksLanguageAndTheFilesUsedTrack(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveEpisodes(t)
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE stream SET container_title = 'Signs' WHERE id = 53",
		`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at) VALUES
		 (1, 'item', 20, 'subtitle_language', '{"language":"fr","kind":"plain","containerTitle":"Signs"}', 1),
		 (1, 'file', 2, 'subtitle_used', '{"streamId":99,"track":{"language":"es","kind":"plain"}}', 1)`)
	if got := harness.subtitlesIn(t, user, 2, 5); got[5] != "53" || got[2] != "51" {
		t.Errorf("stored as subtitle_language and subtitle_used: files start with %v, want 5 French (53) "+
			"and 2 its used Spanish found again (51)", got)
	}
}

func TestFilePreferences_ARefusedChoiceNamesItsField(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveEpisodes(t)
	put := func(body map[string]any) response {
		return harness.do(t, http.MethodPut, "/files/2/preferences", user, body)
	}
	wantInvalidAt(t, put(map[string]any{"subtitleChoice": map[string]any{"isOff": true, "streamId": 51}}),
		http.StatusBadRequest, "body.subtitleChoice")
	wantInvalidAt(t, put(map[string]any{"subtitleChoice": map[string]any{"streamId": 999}}),
		http.StatusBadRequest, "body.subtitleChoice")
	wantInvalidAt(t, put(map[string]any{"audioChoice": map[string]any{"streamId": 999}}),
		http.StatusBadRequest, "body.audioChoice")
}

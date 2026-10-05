// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fakeNodeBytes answers for the node's byte socket from files held by target.
type fakeNodeBytes struct {
	mutex    sync.Mutex
	files    map[string]string // guarded by mutex
	isSilent bool              // guarded by mutex
	status   int               // guarded by mutex: answer this instead, when set
	// asksBeforeFound is, by target, how many more asks for it are answered 404 before it is found:
	// a chunk its node makes while the core waits for it. Guarded by mutex.
	asksBeforeFound map[string]int
}

func (fake *fakeNodeBytes) Do(
	ctx context.Context, _ int64, method, target string, header http.Header,
) (*http.Response, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if fake.isSilent {
		return nil, nodes.ErrNodeSilent
	}
	recorder := httptest.NewRecorder()
	contents, found := fake.files[target]
	if fake.asksBeforeFound[target] > 0 {
		fake.asksBeforeFound[target]--
		found = false
	}
	switch {
	case fake.status != 0:
		recorder.WriteHeader(fake.status)
	case !found:
		recorder.WriteHeader(http.StatusNotFound)
	default:
		request := httptest.NewRequestWithContext(ctx, method, target, http.NoBody)
		request.Header = header
		http.ServeContent(recorder, request, "", time.Time{}, strings.NewReader(contents))
	}
	return recorder.Result(), nil
}

// shelveFilms adds a library with two films on node 1 under /media: an MP4 a browser plays as
// it is, and an MKV with TrueHD it does not.
func (harness testHarness) shelveFilms(t *testing.T) {
	t.Helper()
	mp4 := `{"format":{"formatNames":["mov","mp4","m4a"],"bitRateBps":2400000},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		`{"kind":"audio","codec":"aac","codecProfile":"LC","audio":{"channelCount":2}}]}`
	mkv := `{"format":{"formatNames":["matroska","webm"]},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		`{"streamIndex":1,"kind":"audio","codec":"truehd","audio":{"channelCount":8}},` +
		`{"streamIndex":2,"kind":"subtitle","codec":"ass","language":"en","disposition":["default"]},` +
		`{"streamIndex":3,"kind":"subtitle","codec":"hdmv_pgs_subtitle"}]}`
	librarytest.NodeWithLibrary(t, harness.database,
		librarytest.Library{Name: "Films", Kind: "mixed", MetadataSource: "nfo", Root: "/media"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, payload_hash, filing,
		 probe_json,
		 first_seen_at,
		 last_seen_at) VALUES
		 (1, 1, 1, '/media/Film (2001).mp4', CAST('/media/Film (2001).mp4' AS BLOB), 10, X'0a0b', 'listed', '`+
			mp4+`', 1, 1),
		 (2, 1, 1, '/media/Film (2002).mkv', CAST('/media/Film (2002).mkv' AS BLOB), 10, X'0c0d', 'listed', '`+
			mkv+`', 1, 1),
		 (3, 1, 1, '/media/notes.txt', CAST('/media/notes.txt' AS BLOB), 5, X'0e0f', 'skipped', NULL, 1, 1)`)
	harness.nodeBytes.mutex.Lock()
	defer harness.nodeBytes.mutex.Unlock()
	film := nodeproto.BytesFile{RootPath: "/media", Path: []byte("Film (2001).mp4"), PayloadHash: "0a0b"}
	harness.nodeBytes.files[film.Target()] = "the film's bytes"
}

// chrome is a start-playback body for fileID with the playable formats a desktop Chrome sends.
func chrome(fileID int64) map[string]any {
	return map[string]any{"fileId": fileID, "playableFormats": map[string]any{
		"videoCodecs": []string{"hevc-sdr", "h264-sdr"}, "audioCodecs": []string{"aac-stereo", "opus", "flac"},
		"subtitleFormats": []string{"ass", "webvtt"}, "containers": []string{"mp4"},
	}}
}

type decision struct {
	Delivery         string   `json:"delivery"`
	URL              string   `json:"url"`
	Profile          string   `json:"profile"`
	NotDirectReasons []string `json:"notDirectReasons"`
	IsPreparing      bool     `json:"isPreparing"`
	FileBitRateKbps  int64    `json:"fileBitRateKbps"`
	Audio            []struct {
		AudioTrackIndex   int           `json:"audioTrackIndex"`
		Language          string        `json:"language"`
		IsDialogueBoosted bool          `json:"isDialogueBoosted"`
		Levelling         LevellingBody `json:"levelling"`
		CarriedGainDB     float64       `json:"carriedGainDb"`
	} `json:"audioRenditions"`
	Subtitles   []subtitleTrack `json:"subtitles"`
	Preferences struct {
		SubtitleOffsetMs int  `json:"subtitleOffsetMs"`
		IsLevellingOn    bool `json:"isLevellingOn"`
	} `json:"preferences"`
	ResumePositionMs int64         `json:"resumePositionMs"`
	MediaToken       string        `json:"mediaToken"`
	Levelling        LevellingBody `json:"levelling"`
}

type subtitleTrack struct {
	StreamID          int64  `json:"streamId"`
	FetchedSubtitleID int64  `json:"fetchedSubtitleId"`
	Language          string `json:"language"`
	IsDefault         bool   `json:"isDefault"`
	ASSURL            string `json:"assUrl"`
	VTTURL            string `json:"vttUrl"`
}

func (harness testHarness) play(t *testing.T, token string, body map[string]any) decision {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/playback", token, body)
	var decided decision
	if err := json.Unmarshal([]byte(answer.body), &decided); err != nil || answer.status != http.StatusOK {
		t.Fatalf("POST /playback = %d %s", answer.status, answer.body)
	}
	return decided
}

// byteAnswer is a byte path's answer, read.
type byteAnswer struct {
	status int
	header http.Header
	body   string
}

// getBytePath GETs a byte path, by the header or the media cookie, with a Range when one is given.
func (harness testHarness) getBytePath(t *testing.T, url, token string, byCookie bool, byteRange string) byteAnswer {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, harness.httpServer.URL+url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if byCookie {
		cookie := mediaCookie(token, false)
		request.AddCookie(&cookie)
	} else {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	answer, err := harness.httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = answer.Body.Close() }() //nolint:errcheck // read to the end below
	text, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	return byteAnswer{status: answer.StatusCode, header: answer.Header, body: string(text)}
}

func TestPlayback_AFileThatFitsPlaysDirectlyThroughTheCore(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(1))
	if decided.Delivery != "direct" || len(decided.NotDirectReasons) != 0 ||
		!strings.HasPrefix(decided.URL, Prefix+"/bytes/media/direct/") {
		t.Fatalf("the decision = %+v, want direct with a byte path", decided)
	}
	if decided.FileBitRateKbps != 2400 {
		t.Errorf("the file's bit rate = %d kb/s, want 2400, by which the player measures its link",
			decided.FileBitRateKbps)
	}
	whole := harness.getBytePath(t, decided.URL, user, true, "")
	if whole.status != http.StatusOK || whole.body != "the film's bytes" {
		t.Errorf("GET by the media cookie = %d %q, want the film", whole.status, whole.body)
	}
	if got := whole.header.Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type %q, want video/mp4", got)
	}
	ranged := harness.getBytePath(t, decided.URL, user, false, "bytes=4-7")
	if ranged.status != http.StatusPartialContent || ranged.body != "film" ||
		ranged.header.Get("Content-Range") != "bytes 4-7/16" {
		t.Errorf("a range = %d %q %q, want 206 \"film\"",
			ranged.status, ranged.body, ranged.header.Get("Content-Range"))
	}
}

func TestPlayback_TheAcceptanceFileIsPreparedForItsContainerAndItsAudio(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(2))
	if decided.Delivery != "hls" || decided.Profile != "remux" ||
		strings.Join(decided.NotDirectReasons, ",") != "container,audio_codec" {
		t.Errorf("the decision = %+v, want hls, remux, for container and audio_codec", decided)
	}
}

func TestPlayback_AMediaAddressPlaysOnlyForItsSessionAndOnlyWhileInUse(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(1))
	if answer := harness.getBytePath(t, decided.URL, "", false, ""); answer.status != http.StatusUnauthorized {
		t.Errorf("with no credential: %d, want 401: the address alone plays for no one", answer.status)
	}
	other := harness.signIn(t, "owner", "owner-password")
	if answer := harness.getBytePath(t, decided.URL, other, false, ""); answer.status != http.StatusNotFound {
		t.Errorf("under another session: %d, want 404", answer.status)
	}
	harness.clock.Advance(3 * time.Hour)
	if answer := harness.getBytePath(t, decided.URL, user, false, ""); answer.status != http.StatusOK {
		t.Errorf("used within its idle time: %d, want 200", answer.status)
	}
	harness.clock.Advance(3 * time.Hour)
	if answer := harness.getBytePath(t, decided.URL, user, false, ""); answer.status != http.StatusOK {
		t.Errorf("used again within its idle time of the last use: %d, want 200", answer.status)
	}
	harness.clock.Advance(5 * time.Hour)
	if answer := harness.getBytePath(t, decided.URL, user, false, ""); answer.status != http.StatusNotFound {
		t.Errorf("idle past media_token_idle_expiry_hours: %d, want 404", answer.status)
	}
}

func TestPlayback_SaysWhichLegFailed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(1))
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.isSilent = true
	harness.nodeBytes.mutex.Unlock()
	silent := harness.getBytePath(t, decided.URL, user, false, "")
	if silent.status != http.StatusServiceUnavailable ||
		!strings.Contains(silent.body, `"code":"node_silent"`) || !strings.Contains(silent.body, `"failedHop":"node"`) {
		t.Errorf("a silent node: %d %s, want 503 node_silent at hop node", silent.status, silent.body)
	}
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.isSilent, harness.nodeBytes.status = false, http.StatusConflict
	harness.nodeBytes.mutex.Unlock()
	changed := harness.getBytePath(t, decided.URL, user, false, "")
	if changed.status != http.StatusNotFound || !strings.Contains(changed.body, `"code":"file_unavailable"`) {
		t.Errorf("a file the node no longer has as it was: %d %s, want file_unavailable", changed.status, changed.body)
	}
}

func TestPlayback_RefusesWhatCannotBePlayed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	noBaseline := chrome(1)
	noBaseline["playableFormats"] = map[string]any{
		"videoCodecs": []string{"hevc-sdr"}, "audioCodecs": []string{"aac-stereo"},
		"subtitleFormats": []string{}, "containers": []string{},
	}
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/playback", user, noBaseline), http.StatusBadRequest,
		"body.playableFormats")
	wantError(t, harness.do(t, http.MethodPost, "/playback", user, chrome(3)), http.StatusNotFound, CodeNotFound)
	wantError(t, harness.do(t, http.MethodPost, "/playback", user, chrome(99)), http.StatusNotFound, CodeNotFound)
	wantError(t, harness.do(t, http.MethodPost, "/playback", "", chrome(1)), http.StatusUnauthorized, CodeAuthRequired)
}

func TestPlayback_AFileRemovedWhilePlayingIsNoFileToPlayAsItIsToStart(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(1))
	storetest.ExecRaw(t, harness.databasePath, "UPDATE file SET removed_at = 2 WHERE id = 1")
	playing := harness.getBytePath(t, decided.URL, user, false, "")
	wantErrorSaying(t, response{status: playing.status, body: playing.body}, http.StatusNotFound, CodeNotFound,
		"no such file to play")
	wantErrorSaying(t, harness.do(t, http.MethodPost, "/playback", user, chrome(1)), http.StatusNotFound,
		CodeNotFound, "no such file to play")
}

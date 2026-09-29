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

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fakeNodeBytes answers for the node's byte socket from files held by target.
type fakeNodeBytes struct {
	mutex  sync.Mutex
	files  map[string]string // guarded by mutex
	silent bool              // guarded by mutex
	status int               // guarded by mutex: answer this instead, when set
}

func (fake *fakeNodeBytes) Do(
	ctx context.Context, _ int64, method, target string, header http.Header,
) (*http.Response, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if fake.silent {
		return nil, nodes.ErrNodeSilent
	}
	recorder := httptest.NewRecorder()
	contents, found := fake.files[target]
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
	mp4 := `{"format":{"names":["mov","mp4","m4a"]},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		`{"kind":"audio","codec":"aac","codecProfile":"LC","audio":{"channelCount":2}}]}`
	mkv := `{"format":{"names":["matroska","webm"]},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		`{"streamIndex":1,"kind":"audio","codec":"truehd","audio":{"channelCount":8}},` +
		`{"streamIndex":2,"kind":"subtitle","codec":"ass","languageTag":"en","disposition":["default"]},` +
		`{"streamIndex":3,"kind":"subtitle","codec":"hdmv_pgs_subtitle"}]}`
	storetest.ExecRaw(t, harness.database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'mixed', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO library_root (library_id, node_id, path) VALUES (1, 1, '/media')",
		`INSERT INTO file (id, library_id, node_id, path, reported_path_bytes, size_bytes, payload_hash, filing,
		 probe_json,
		 first_seen_at,
		 last_seen_at) VALUES
		 (1, 1, 1, '/media/Film (2001).mp4', CAST('/media/Film (2001).mp4' AS BLOB), 10, X'0a0b', 'listed', '`+
			mp4+`', 1, 1),
		 (2, 1, 1, '/media/Film (2002).mkv', CAST('/media/Film (2002).mkv' AS BLOB), 10, X'0c0d', 'listed', '`+
			mkv+`', 1, 1),
		 (3, 1, 1, '/media/notes.txt', CAST('/media/notes.txt' AS BLOB), 5, X'0e0f', 'skipped', NULL, 1, 1)`)
	harness.bytes.mutex.Lock()
	defer harness.bytes.mutex.Unlock()
	film := nodeproto.BytesFile{Root: "/media", Path: []byte("Film (2001).mp4"), PayloadHash: "0a0b"}
	harness.bytes.files[film.Target()] = "the film's bytes"
}

// chrome is the menu a desktop Chrome sends.
func chrome(fileID int64) map[string]any {
	return map[string]any{"fileId": fileID, "link": "local", "menu": map[string]any{
		"video": []string{"hevc-sdr", "h264-sdr"}, "audio": []string{"aac-stereo", "opus", "flac"},
		"subtitle": []string{"ass", "webvtt"}, "containers": []string{"mp4"},
	}}
}

type decision struct {
	Kind        string   `json:"kind"`
	URL         string   `json:"url"`
	Profile     string   `json:"profile"`
	Reasons     []string `json:"reasons"`
	IsPreparing bool     `json:"isPreparing"`
	Audio       []struct {
		SourceTrack     int           `json:"sourceTrack"`
		Language        string        `json:"language"`
		IsDialogueBoost bool          `json:"isDialogueBoost"`
		Levelling       LevellingBody `json:"levelling"`
		CarriedGainDB   float64       `json:"carriedGainDb"`
	} `json:"audio"`
	Subtitles []subtitleTrack `json:"subtitles"`
	Prefs     struct {
		SubtitleOffsetMs int  `json:"subtitleOffsetMs"`
		IsLevelling      bool `json:"isLevelling"`
	} `json:"prefs"`
	ResumePositionMs int64         `json:"resumePositionMs"`
	MediaToken       string        `json:"mediaToken"`
	Levelling        LevellingBody `json:"levelling"`
}

type subtitleTrack struct {
	StreamID  int64  `json:"streamId"`
	FetchedID int64  `json:"fetchedId"`
	Language  string `json:"language"`
	IsDefault bool   `json:"isDefault"`
	ASSURL    string `json:"assUrl"`
	VTTURL    string `json:"vttUrl"`
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

// fetch GETs a byte path, by the header or the media cookie, with a Range when one is given.
func (harness testHarness) fetch(t *testing.T, url, token string, byCookie bool, byteRange string) byteAnswer {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, harness.http.URL+url, http.NoBody)
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
	answer, err := harness.http.Client().Do(request)
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
	if decided.Kind != "direct" || len(decided.Reasons) != 0 ||
		!strings.HasPrefix(decided.URL, Prefix+"/bytes/media/direct/") {
		t.Fatalf("the decision = %+v, want direct with a byte path", decided)
	}
	whole := harness.fetch(t, decided.URL, user, true, "")
	if whole.status != http.StatusOK || whole.body != "the film's bytes" {
		t.Errorf("GET by the media cookie = %d %q, want the film", whole.status, whole.body)
	}
	if got := whole.header.Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type %q, want video/mp4", got)
	}
	ranged := harness.fetch(t, decided.URL, user, false, "bytes=4-7")
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
	if decided.Kind != "hls" || decided.Profile != "remux" ||
		strings.Join(decided.Reasons, ",") != "container,audio_codec" {
		t.Errorf("the decision = %+v, want hls, remux, for container and audio_codec", decided)
	}
}

func TestPlayback_AMediaAddressPlaysOnlyForItsSessionAndOnlyWhileInUse(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(1))
	if answer := harness.fetch(t, decided.URL, "", false, ""); answer.status != http.StatusUnauthorized {
		t.Errorf("with no credential: %d, want 401: the address alone plays for no one", answer.status)
	}
	other := harness.signIn(t, "owner", "owner-password")
	if answer := harness.fetch(t, decided.URL, other, false, ""); answer.status != http.StatusNotFound {
		t.Errorf("under another session: %d, want 404", answer.status)
	}
	harness.clock.Advance(3 * time.Hour)
	if answer := harness.fetch(t, decided.URL, user, false, ""); answer.status != http.StatusOK {
		t.Errorf("used within its idle time: %d, want 200", answer.status)
	}
	harness.clock.Advance(3 * time.Hour)
	if answer := harness.fetch(t, decided.URL, user, false, ""); answer.status != http.StatusOK {
		t.Errorf("used again within its idle time of the last use: %d, want 200", answer.status)
	}
	harness.clock.Advance(5 * time.Hour)
	if answer := harness.fetch(t, decided.URL, user, false, ""); answer.status != http.StatusNotFound {
		t.Errorf("idle past media_token_idle_expiry_h: %d, want 404", answer.status)
	}
}

func TestPlayback_SaysWhichLegFailed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(1))
	harness.bytes.mutex.Lock()
	harness.bytes.silent = true
	harness.bytes.mutex.Unlock()
	silent := harness.fetch(t, decided.URL, user, false, "")
	if silent.status != http.StatusServiceUnavailable ||
		!strings.Contains(silent.body, `"code":"node_silent"`) || !strings.Contains(silent.body, `"hop":"node"`) {
		t.Errorf("a silent node: %d %s, want 503 node_silent at hop node", silent.status, silent.body)
	}
	harness.bytes.mutex.Lock()
	harness.bytes.silent, harness.bytes.status = false, http.StatusConflict
	harness.bytes.mutex.Unlock()
	changed := harness.fetch(t, decided.URL, user, false, "")
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
	noBaseline["menu"] = map[string]any{
		"video": []string{"hevc-sdr"}, "audio": []string{"aac-stereo"},
		"subtitle": []string{}, "containers": []string{},
	}
	wantError(t, harness.do(t, http.MethodPost, "/playback", user, noBaseline), http.StatusBadRequest,
		CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodPost, "/playback", user, chrome(3)), http.StatusNotFound, CodeNotFound)
	wantError(t, harness.do(t, http.MethodPost, "/playback", user, chrome(99)), http.StatusNotFound, CodeNotFound)
	wantError(t, harness.do(t, http.MethodPost, "/playback", "", chrome(1)), http.StatusUnauthorized, CodeAuthRequired)
}

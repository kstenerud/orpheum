// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
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
	// stalledRoot, when set, is a root whose disk the node says does not answer (guarded by mutex).
	stalledRoot string
	status      int // guarded by mutex: answer this instead, when set
	// asksBeforeFound is, by target, how many more asks for it are answered 404 before it is found:
	// a chunk its node makes while the core waits for it. Guarded by mutex.
	asksBeforeFound map[string]int
	// asked are the targets asked for, in order. Guarded by mutex.
	asked []string
}

func (fake *fakeNodeBytes) Do(
	ctx context.Context, nodeID int64, method, target string, header http.Header,
) (*http.Response, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if fake.isSilent {
		return nil, nodes.ErrNodeSilent
	}
	if fake.stalledRoot != "" {
		return nil, &nodes.StalledRootError{NodeID: nodeID, RootPath: fake.stalledRoot}
	}
	fake.asked = append(fake.asked, target)
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
	// Film 2 has a length, so a player that cannot play it as it is plays it from its ladder cut on time
	// while its keyframes are not known (D344 §2).
	mkv := `{"format":{"formatNames":["matroska","webm"],"durationMs":1200000},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		`{"streamIndex":1,"kind":"audio","codec":"truehd","audio":{"channelCount":8}},` +
		`{"streamIndex":2,"kind":"subtitle","codec":"ass","language":"en","disposition":["default"]},` +
		`{"streamIndex":3,"kind":"subtitle","codec":"hdmv_pgs_subtitle"}]}`
	nodeID, libraryID := librarytest.NodeWithLibrary(t, harness.database,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/media"})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 1, LibraryID: libraryID, NodeID: nodeID, Path: "/media/Film (2001).mp4", SizeBytes: 10,
		PayloadHash: []byte{0x0a, 0x0b}, Filing: "listed",
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 2, LibraryID: libraryID, NodeID: nodeID, Path: "/media/Film (2002).mkv", SizeBytes: 10,
		PayloadHash: []byte{0x0c, 0x0d}, Filing: "listed",
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 3, LibraryID: libraryID, NodeID: nodeID, Path: "/media/notes.txt", SizeBytes: 5,
		PayloadHash: []byte{0x0e, 0x0f}, Filing: "skipped",
	})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 1, ProbeJSON: mp4})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 2, ProbeJSON: mkv, DurationMs: 1200000})
	harness.nodeBytes.mutex.Lock()
	defer harness.nodeBytes.mutex.Unlock()
	film := nodeproto.BytesFile{RootPath: "/media", Path: []byte("Film (2001).mp4"), PayloadHash: "0a0b"}
	harness.nodeBytes.files[film.Target()] = "the film's bytes"
}

// fileFilmAsTitle files shelveFilms' first film as title 10, named name.
func (harness testHarness) fileFilmAsTitle(t *testing.T, name string) {
	t.Helper()
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 10, Library: 1})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 1, TitleID: 10})
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE file_filing SET version_id = 1 WHERE file_id = 1",
		fmt.Sprintf("INSERT INTO item_text (item_id, language, is_original, name) VALUES (10, 'und', 1, '%s')", name))
}

// chrome is a start-playback body for fileID with the playable formats a desktop Chrome sends.
func chrome(fileID int64) map[string]any {
	return map[string]any{"fileId": fileID, "playableFormats": map[string]any{
		"videoCodecs": []string{"hevc-sdr", "h264-sdr"}, "audioCodecs": []string{"aac-stereo", "opus", "flac"},
		"subtitleFormats": []string{"ass", "webvtt"}, "containers": []string{"mp4"},
	}}
}

type decision struct {
	Delivery         string          `json:"delivery"`
	URL              string          `json:"url"`
	Profile          string          `json:"profile"`
	NotDirectReasons []string        `json:"notDirectReasons"`
	FileBitRateKbps  int64           `json:"fileBitRateKbps"`
	Subtitles        []subtitleTrack `json:"subtitles"`
	Preferences      struct {
		SubtitleOffsetMs int  `json:"subtitleOffsetMs"`
		IsLevellingOn    bool `json:"isLevellingOn"`
	} `json:"preferences"`
	ResumePositionMs int64             `json:"resumePositionMs"`
	MediaToken       string            `json:"mediaToken"`
	Levelling        api.LevellingBody `json:"levelling"`
	TitleName        string            `json:"titleName"`
	OnLadder         *api.LadderBody   `json:"onLadder"`
	Tracks           api.TracksBody    `json:"tracks"`
	Plays            *api.PlaysBody    `json:"plays"`
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
		cookie := api.ByteCookie(api.MediaCookieName, token, false)
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
		!strings.HasPrefix(decided.URL, api.Prefix+"/bytes/media/direct/") {
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

// D363 §8: a direct read whose node has no file at the place read, renamed away there, reads the
// copy at another place its holder reaches, and the reads after it go there first, until a scan
// sees the file at its place again. It answered 404 though the copy stood (staging, 2026-10-27,
// VQ81). With no place left that has it, the file is no longer there as it was.
func TestPlayback_ADirectReadOfAFileGoneFromItsPlaceReadsItsCopy(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	backup := librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Backup", Kind: "movie", MetadataSource: "nfo", Root: "/backup"})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		LibraryID: backup, NodeID: 1, Path: "/backup/Film (2001).mp4", Filing: "copy", CopyOf: 1, FirstSeenAt: 3,
	})
	first := nodeproto.BytesFile{RootPath: "/media", Path: []byte("Film (2001).mp4"), PayloadHash: "0a0b"}.Target()
	copied := nodeproto.BytesFile{RootPath: "/backup", Path: []byte("Film (2001).mp4"), PayloadHash: "0a0b"}.Target()
	harness.nodeBytes.mutex.Lock()
	delete(harness.nodeBytes.files, first) // renamed away
	harness.nodeBytes.files[copied] = "the copy's bytes"
	harness.nodeBytes.mutex.Unlock()
	decided := harness.play(t, user, chrome(1))
	for range 2 {
		if read := harness.getBytePath(t, decided.URL, user, false, ""); read.status != http.StatusOK ||
			read.body != "the copy's bytes" {
			t.Errorf("a read of the film gone from its first place = %d %q; want the copy's bytes", read.status,
				read.body)
		}
	}
	harness.nodeBytes.mutex.Lock()
	asked := slices.Clone(harness.nodeBytes.asked)
	delete(harness.nodeBytes.files, copied)
	harness.nodeBytes.mutex.Unlock()
	if !slices.Equal(asked, []string{first, copied, copied}) {
		t.Errorf("the node was asked for %q; want the first place once, then the copy each time", asked)
	}
	if read := harness.getBytePath(t, decided.URL, user, false, ""); read.status != http.StatusNotFound {
		t.Errorf("a read of the film gone from both places = %d %q; want 404", read.status, read.body)
	}
}

// The decision names the title the file is, for the player to show above the picture, and never
// a file's path in its place: a file filed as no title has no name.
func TestPlayback_NamesTheTitleItPlaysAndNeverAPath(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	if unfiled := harness.play(t, user, chrome(1)); unfiled.TitleName != "" {
		t.Errorf("a file filed as no title is named %q; want no name, never its path", unfiled.TitleName)
	}
	harness.fileFilmAsTitle(t, "Metropolis")
	if named := harness.play(t, user, chrome(1)); named.TitleName != "Metropolis" {
		t.Errorf("the title's name = %q, want Metropolis", named.TitleName)
	}
}

// The decision says what the file plays and where it lives, which the Play page goes up to with no
// page of the app behind it (the owner, 2026-10-27): a film by its title, an episode by its series
// and its place in the ordering its person watches in, an other video by its library; nothing for a
// file filed as no title.
func TestPlayback_SaysWhatItPlaysAndWhereItLives(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t) // Metropolis (10, file 1); episodes 21-24 of series 20, files 2-5
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 30, Library: 1, Kind: "other", AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 6, TitleID: 30, CreatedAt: 3})
	// Metropolis's trailer, an extra, which goes up to Metropolis's page (D367 §2).
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 31, Library: 1, Kind: "extra", AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 8, TitleID: 31, CreatedAt: 3})
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO extra_relation
		(owner_work, extra_work, extra_type, asserted_by)
		VALUES (10, 31, 'trailer', 'scanner')`)
	for _, file := range []librarytest.FileSpec{
		{ID: 6, Path: "/m/Home/Beach.mkv", PayloadHash: []byte{0x06}, Filing: "listed", VersionID: 6},
		{ID: 7, Path: "/m/Unknown.mkv", PayloadHash: []byte{0x07}, Filing: "unidentified"},
		{ID: 8, Path: "/m/Metropolis (1995)/Trailer.mkv", PayloadHash: []byte{0x08}, Filing: "listed", VersionID: 8},
	} {
		file.LibraryID, file.NodeID, file.SizeBytes = 1, 1, 10
		librarytest.File(t, harness.database, file)
	}
	// Each a video a browser plays as it is.
	mp4 := `{"format":{"formatNames":["mov","mp4","m4a"]},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}}]}`
	for fileID := int64(1); fileID <= 8; fileID++ {
		librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: fileID, ProbeJSON: mp4, DurationMs: 1000})
	}
	seen := func(fileID int64) string {
		plays := harness.play(t, user, chrome(fileID)).Plays
		if plays == nil {
			return "nothing"
		}
		text := fmt.Sprintf("%s %d in library %d", plays.Kind, plays.ItemID, plays.LibraryID)
		if plays.LibraryID != 0 && plays.LibraryName == "" {
			text += ", unnamed"
		}
		if episode := plays.Episode; episode != nil {
			text += fmt.Sprintf(", series %d season %d %q", episode.SeriesID, episode.SeasonNumber, episode.Ordering)
			if in := episode.InOrdering; in != nil {
				text += fmt.Sprintf(" group %d", in.GroupNumber)
			}
		}
		if film := plays.FilmOfExtra; film != nil {
			text += fmt.Sprintf(", of film %d %q", film.ID, film.Name)
		}
		return text
	}
	for fileID, want := range map[int64]string{
		1: "film 10 in library 1",
		4: `episode 23 in library 1, series 20 season 1 ""`,
		6: "other 30 in library 1",
		7: "nothing",
		8: `extra 31 in library 1, of film 10 "Metropolis"`,
	} {
		if got := seen(fileID); got != want {
			t.Errorf("file %d plays %s; want %s", fileID, got, want)
		}
	}
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO preference (user_id, scope, scope_id, axis, value_json,
		set_at) VALUES (1, 'item', 20, 'episode_ordering', '"tmdb:dvd"', 1)`)
	for fileID, want := range map[int64]string{
		5: `episode 24 in library 1, series 20 season 2 "tmdb:dvd" group 2`,
		4: `episode 23 in library 1, series 20 season 1 "tmdb:dvd" group 1`,
		3: `episode 22 in library 1, series 20 season 1 "tmdb:dvd"`, // which the DVD order leaves out
	} {
		if got := seen(fileID); got != want {
			t.Errorf("DVD order, file %d plays %s; want %s", fileID, got, want)
		}
	}
}

// A playback names its tracks beside the file's own: played as it is, every one original; converted,
// its sound converted to AAC in stereo from the file's track (the owner, 2026-10-20).
func TestPlayback_NamesEachTrackOriginalOrConvertedBesideTheFilesOwn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	direct := harness.play(t, user, chrome(1)).Tracks
	if len(direct.Pictures) != 1 || !direct.Pictures[0].IsOriginal || direct.Pictures[0].Codec == "" ||
		direct.Pictures[0].Codec != direct.FilePicture.Codec || len(direct.Sounds) == 0 ||
		!direct.Sounds[0].IsOriginal {
		t.Errorf("a file played as it is names %+v; want every track original, as the file's own", direct)
	}
	converted := harness.play(t, user, chrome(2)).Tracks
	if len(converted.Sounds) == 0 || converted.Sounds[0].IsOriginal || converted.Sounds[0].Codec != "aac" ||
		converted.Sounds[0].ChannelCount != 2 || len(converted.FileSounds) == 0 ||
		converted.FileSounds[0].Codec == "aac" {
		t.Errorf("a film converted names %+v; want its sound converted to AAC stereo from another codec", converted)
	}
}

func TestPlayback_TheAcceptanceFileIsConvertedForItsContainerAndItsAudio(t *testing.T) {
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

// Each playback start is logged with whose it is, the file, and how it is delivered and why, never
// with its token; a refused one with why; by which name and from which address it came, which says
// which of a discovery's paths reached the server; and why the page asked, and how many times. On
// staging a film that would not play left no trace of what was chosen for it (2026-10-06).
func TestPlayback_LogsEachStartAndRefusalButNeverTheToken(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	asked := chrome(2)
	asked["cause"], asked["pageStarts"], asked["startsAtMs"] = "subtitleFetched", 3, 166502
	decided := harness.play(t, user, asked)
	harness.logs.AwaitEvent(t, "playbackStarted", `"fileId":2`, `"delivery":"ladder"`,
		`"decidedDelivery":"hls"`, `"profile":"remux"`, `"notDirectReasons":["container","audio_codec"]`,
		// what made the page ask, how many it asked for, and where it goes on from, not the resume
		// position (staging, 2026-10-26)
		`"cause":"subtitleFetched"`, `"pageStarts":3`, `"pageStartsAtMs":166502`,
		// which browser, as an iPhone's player differs
		`"userAgent":"Go-http-client/1.1"`,
		// which way, by which name and from where the request reached the server
		`"over":"http"`, `"host":"127.0.0.1:`, `"clientAddress":"127.0.0.1"`,
		`"notOnLadderBecause":""`) // on its ladder, cut on time: its keyframes are not read yet
	token := decided.URL[strings.LastIndex(strings.TrimSuffix(decided.URL, "/master.m3u8"), "/")+1:]
	if token == "" || strings.Contains(harness.logs.String(), token) {
		t.Errorf("the log holds the playback's token %q, or none was found in %q", token, decided.URL)
	}
	wantError(t, harness.do(t, http.MethodPost, "/playback", user, chrome(999)), http.StatusNotFound, api.CodeNotFound)
	harness.logs.AwaitEvent(t, "playbackRefused", `"fileId":999`)
}

// A page that opens again a playback the server lost, as it restarted, says so, and the start is
// taken and logged with it (the owner's test, 2026-10-18).
func TestPlayback_APlaybackOpenedAgainAfterTheServerLostItSaysSo(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	asked := chrome(2)
	asked["cause"] = "playbackGone"
	harness.play(t, user, asked)
	harness.logs.AwaitEvent(t, "playbackStarted", `"fileId":2`, `"cause":"playbackGone"`)
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
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.status, harness.nodeBytes.stalledRoot = 0, "/media"
	harness.nodeBytes.mutex.Unlock()
	stalled := harness.getBytePath(t, decided.URL, user, false, "")
	if stalled.status != http.StatusServiceUnavailable ||
		!strings.Contains(stalled.body, `"code":"media_unreachable"`) ||
		!strings.Contains(stalled.body, `"reason":"root_missing"`) || !strings.Contains(stalled.body, `"/media"`) {
		t.Errorf("a file on a root whose disk does not answer: %d %s, want media_unreachable, /media missing",
			stalled.status, stalled.body)
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
	wantError(t, harness.do(t, http.MethodPost, "/playback", user, chrome(3)), http.StatusNotFound, api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodPost, "/playback", user, chrome(99)), http.StatusNotFound, api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodPost, "/playback", "", chrome(1)), http.StatusUnauthorized,
		api.CodeAuthRequired)
}

func TestPlayback_AFileRemovedWhilePlayingIsNoFileToPlayAsItIsToStart(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(1))
	storetest.ExecRaw(t, harness.databasePath, "UPDATE place SET removed_at = 2 WHERE id = 1")
	playing := harness.getBytePath(t, decided.URL, user, false, "")
	wantErrorSaying(t, response{status: playing.status, body: playing.body}, http.StatusNotFound, api.CodeNotFound,
		"no such file to play")
	wantErrorSaying(t, harness.do(t, http.MethodPost, "/playback", user, chrome(1)), http.StatusNotFound,
		api.CodeNotFound, "no such file to play")
}

// A player whose film could not be read and can again asks for a new playback saying so, which the
// log keeps (the owner, 2026-10-21).
func TestPlayback_APlaybackAskedForOnceItsFilmIsBackSaysSo(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	asked := chrome(2)
	asked["cause"] = "mediaBack"
	harness.play(t, user, asked)
	harness.logs.AwaitEvent(t, "playbackStarted", `"fileId":2`, `"cause":"mediaBack"`)
}

// A cast's remote playing here from where its receiver stood asks saying so, and from where, which
// the log keeps: a Play here mid-film played from 0:00, and its line said only cause=playPage
// resumePositionMs=0 (staging, 2026-10-27).
func TestPlayback_APlayHereFromACastsEndSaysSoAndFromWhere(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	asked := chrome(2)
	asked["cause"] = "castOver"
	asked["startsAtMs"] = 982680
	harness.play(t, user, asked)
	harness.logs.AwaitEvent(t, "playbackStarted", `"fileId":2`, `"cause":"castOver"`,
		`"pageStartsAtMs":982680`)
}

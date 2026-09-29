// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/playback/opensubtitles"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fakeSubtitler is OpenSubtitles: it records what it was asked, offers one subtitle made for the
// file and one not, and fails as failure says.
type fakeSubtitler struct {
	mutex   sync.Mutex
	queries []opensubtitles.Query
	failure error
}

// fetchedText is the subtitle the fake gives.
const fetchedText = "1\n00:00:01,000 --> 00:00:02,000\nHallo\n"

// resetAt is when the fake's account's count starts again.
var resetAt = time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)

func (fake *fakeSubtitler) Search(_ context.Context, query opensubtitles.Query) ([]opensubtitles.Candidate, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.queries = append(fake.queries, query)
	if fake.failure != nil {
		return nil, fake.failure
	}
	// The hash match comes second, so only the core's order can put it first (D224 §3).
	return []opensubtitles.Candidate{
		{ProviderFileID: 8, ReleaseName: "Film.2002.720p", Language: "de", IsHearingImpaired: true},
		{
			ProviderFileID: 7, ReleaseName: "Film.2002.1080p", Language: "de", IsOpensubtitlesHashMatch: true,
			DownloadCount: 120,
		},
	}, nil
}

func (fake *fakeSubtitler) Download(context.Context, int64) (opensubtitles.Downloaded, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if fake.failure != nil {
		return opensubtitles.Downloaded{}, fake.failure
	}
	return opensubtitles.Downloaded{
		Body: []byte(fetchedText), FileName: "Film.2002.1080p.de.srt", RemainingDownloads: 19,
		DownloadsResetAt: resetAt,
	}, nil
}

func (fake *fakeSubtitler) Allowance(context.Context) (opensubtitles.Allowance, error) {
	return opensubtitles.Allowance{RemainingDownloads: 20, AllowedDownloads: 20}, nil
}

// identifyFilm makes film 2 a version of a film TMDB and IMDb name, with its OpenSubtitles hash.
func (harness testHarness) identifyFilm(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO item (id, kind, library_id, added_at) VALUES (30, 'title', 1, 1)",
		"INSERT INTO title (id, kind, year, identification_state) VALUES (30, 'movie', 2002, 'identified')",
		"INSERT INTO version (id, title_id, created_at) VALUES (3, 30, 1)",
		"UPDATE file SET version_id = 3, opensubtitles_hash = '8e245d9679d31e12' WHERE id = 2",
		`INSERT INTO item_provider_id (item_id, provider, provider_id)
		 VALUES (30, 'tmdb', '4801'), (30, 'imdb', 'tt0123456')`)
}

func TestSubtitleCandidates_AreFoundByTheWorksIDsAndTheFilesHashWithTheDownloadsLeft(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.identifyFilm(t)
	answer := harness.do(t, http.MethodGet, "/files/2/subtitle-candidates?language=de", user, nil)
	offered := offeredCandidates(t, answer)
	first := offered.Candidates
	if len(first) != 2 || !first[0].IsOpensubtitlesHashMatch || first[0].ProviderFileID != 7 ||
		offered.Allowance.RemainingDownloads != 20 ||
		!offered.DidAskByIDs || !offered.DidAskByOpensubtitlesHash {
		t.Errorf("offered %+v; want both candidates, the hash match first, the downloads left, and that it "+
			"asked by the ids and the hash (D224)", offered)
	}
	asked := harness.subtitler.queries[0]
	if asked.IMDbID != "tt0123456" || asked.TMDBID != "4801" || asked.MovieHash != "8e245d9679d31e12" ||
		asked.Language != "de" {
		t.Errorf("asked OpenSubtitles %+v; want the film's ids, the file's hash and the language (D224 §3)", asked)
	}
	nothing := harness.do(t, http.MethodGet, "/files/1/subtitle-candidates?language=de", user, nil)
	wantError(t, nothing, http.StatusConflict, CodeConflict)

	// A file not identified yet is asked for by its hash alone, and the answer says so.
	storetest.ExecRaw(t, harness.databasePath, "UPDATE file SET opensubtitles_hash = '43c6c64ddb8e4766' WHERE id = 1")
	hashOnly := offeredCandidates(t, harness.do(t, http.MethodGet, "/files/1/subtitle-candidates?language=de", user,
		nil))
	if hashOnly.DidAskByIDs || !hashOnly.DidAskByOpensubtitlesHash {
		t.Errorf("for an unidentified file: searched by ids %t, by hash %t; want the hash alone",
			hashOnly.DidAskByIDs, hashOnly.DidAskByOpensubtitlesHash)
	}
}

// candidatesAnswer is what GET subtitle-candidates answers.
type candidatesAnswer struct {
	Candidates                []SubtitleCandidateBody `json:"candidates"`
	Allowance                 AllowanceBody           `json:"allowance"`
	DidAskByIDs               bool                    `json:"didAskByIds"`
	DidAskByOpensubtitlesHash bool                    `json:"didAskByOpensubtitlesHash"`
}

// offeredCandidates reads a 200 answer of GET subtitle-candidates.
func offeredCandidates(t *testing.T, answer response) candidatesAnswer {
	t.Helper()
	var body candidatesAnswer
	if err := json.Unmarshal([]byte(answer.body), &body); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET subtitle-candidates = %d %s", answer.status, answer.body)
	}
	return body
}

// Fetching a subtitle is for everyone who may watch, not only an admin (D224 §1).
func TestSubtitleCandidates_APersonWhoIsNotAnAdminListsAndFetchesThem(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	_, err := harness.authService.CreateUser(t.Context(), auth.NewUser{
		Name: "guest", Password: "guest-password", Role: auth.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	guest := harness.signIn(t, "guest", "guest-password")
	harness.shelveFilms(t)
	harness.identifyFilm(t)
	if offered := offeredCandidates(t, harness.do(t, http.MethodGet, "/files/2/subtitle-candidates?language=de",
		guest, nil)); len(offered.Candidates) != 2 {
		t.Errorf("a person who is not an admin was offered %+v, want both candidates", offered)
	}
	fetchedID := fetchGerman(t, harness, guest)
	if choice := harness.choiceIn(t, guest, 2); choice != fmt.Sprintf("fetched %d carried=false", fetchedID) {
		t.Errorf("the guest's choice is %s, want the subtitle the guest fetched", choice)
	}
}

func TestSubtitleCandidates_ForAnEpisodeAskByTheSeriesAndItsPlace(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveEpisodes(t)
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO item_provider_id (item_id, provider, provider_id) VALUES (20, 'tmdb', '1396')")
	if answer := harness.do(t, http.MethodGet, "/files/2/subtitle-candidates?language=en", user, nil); answer.status !=
		http.StatusOK {
		t.Fatalf("GET subtitle-candidates = %d %s", answer.status, answer.body)
	}
	asked := harness.subtitler.queries[0]
	if asked.SeriesTMDBID != "1396" || asked.Season != 1 || asked.Episode != 1 || asked.TMDBID != "" {
		t.Errorf("asked %+v; want the series' id and the episode's place (D224 §3)", asked)
	}
}

func TestSubtitleCandidates_SayWhyInTheProvidersTerms(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.identifyFilm(t)
	for failure, want := range map[error]string{
		opensubtitles.ErrNoLogin:      `"reason":"no_login"`,
		opensubtitles.ErrLoginRefused: `"reason":"login_refused"`,
		fmt.Errorf("opensubtitles: %w: status 503", outbound.ErrUnavailable): `"reason":"unavailable"`,
		&opensubtitles.QuotaSpentError{DownloadsResetAt: resetAt}:            `"downloadsResetAt":"1790661600000"`,
		&outbound.RateLimitedError{Provider: "OpenSubtitles", RetryAfter: 2 * time.Second}: `"message":` +
			`"OpenSubtitles asks us to wait","detail":{"retryAfterMs":"2000"}`,
	} {
		harness.subtitler.mutex.Lock()
		harness.subtitler.failure = failure
		harness.subtitler.mutex.Unlock()
		answer := harness.do(t, http.MethodPost, "/files/2/subtitles", user,
			map[string]any{"providerFileId": 7, "language": "de"})
		if answer.status < 400 || !strings.Contains(answer.body, want) ||
			!strings.Contains(answer.body, `"failedHop":"provider"`) {
			t.Errorf("when OpenSubtitles says %v: %d %s, want %s (D224 §4)", failure, answer.status, answer.body, want)
		}
	}
}

func TestFetchSubtitle_KeepsItChoosesItAndItPlaysOnceTheNodeConvertsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.identifyFilm(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	fetchedID := fetchGerman(t, harness, user)
	checkKeptGzipped(t, harness, fetchedID)

	// The next playback offers it, chosen, and has it converted.
	track := offeredFetched(t, harness.play(t, user, chrome(2)), fetchedID)
	if choice := harness.choiceIn(t, user, 2); choice != fmt.Sprintf("fetched %d carried=false", fetchedID) {
		t.Errorf("the file's choice is %s, want the fetched track (D224 §4)", choice)
	}
	convertFetched(t, harness, fetchedID)
	if served := harness.getBytePath(t, track.VTTURL, user, true, ""); served.status != http.StatusOK ||
		!strings.Contains(served.body, "Hallo") {
		t.Errorf("the fetched track: %d %q, want its converted text", served.status, served.body)
	}
}

// A fetched subtitle's language is kept in the one form the library stores, BCP 47 (D206), however
// the request spelled it.
func TestFetchSubtitle_KeepsItsLanguageAsBCP47(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.identifyFilm(t)
	answer := harness.do(t, http.MethodPost, "/files/2/subtitles", user,
		map[string]any{"providerFileId": 7, "language": "ger"})
	var fetched struct {
		FetchedSubtitleID int64 `json:"fetchedSubtitleId"`
	}
	if err := json.Unmarshal([]byte(answer.body), &fetched); err != nil || answer.status != http.StatusOK {
		t.Fatalf("POST /files/2/subtitles = %d %s", answer.status, answer.body)
	}
	kept, err := harness.database.Read().ListFetchedSubtitlesOfVideo(t.Context(), 2)
	if err != nil || len(kept) != 1 || kept[0].ID != fetched.FetchedSubtitleID || kept[0].Language != "de" {
		t.Errorf("the fetched subtitles are %+v, %v; want the one fetched, its language ger kept as de", kept, err)
	}
}

// fetchGerman fetches the fake's first candidate for film 2 and returns the new track.
func fetchGerman(t *testing.T, harness testHarness, user string) int64 {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/files/2/subtitles", user,
		map[string]any{"providerFileId": 7, "language": "de"})
	var fetched struct {
		FetchedSubtitleID  int64 `json:"fetchedSubtitleId"`
		RemainingDownloads int   `json:"remainingDownloads"`
		DownloadsResetAt   int64 `json:"downloadsResetAt"`
	}
	if err := json.Unmarshal([]byte(answer.body), &fetched); err != nil || answer.status != http.StatusOK ||
		fetched.FetchedSubtitleID == 0 || fetched.RemainingDownloads != 19 ||
		fetched.DownloadsResetAt != resetAt.UnixMilli() {
		t.Fatalf("POST /files/2/subtitles = %d %s; want the new track and what is left", answer.status, answer.body)
	}
	return fetched.FetchedSubtitleID
}

// offeredFetched is the fetched track a playback offers, which fails the test when it is not there.
func offeredFetched(t *testing.T, decided decision, fetchedID int64) subtitleTrack {
	t.Helper()
	wantURL := fmt.Sprintf("/f%d.vtt", fetchedID)
	for _, track := range decided.Subtitles {
		if track.FetchedSubtitleID == fetchedID && track.Language == "de" && strings.HasSuffix(track.VTTURL, wantURL) {
			return track
		}
	}
	t.Fatalf("the playback offers %+v; want the fetched German track at …%s", decided.Subtitles, wantURL)
	return subtitleTrack{}
}

// convertFetched runs the job that converts a fetched subtitle, answering as the node would.
func convertFetched(t *testing.T, harness testHarness, fetchedID int64) {
	t.Helper()
	playback := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	job := jobs.Job{ID: 95, Kind: jobs.KindConvertFetchedSubtitle, Target: jobs.Target{FetchedSubtitleID: fetchedID}}
	go func() { finished <- playback.HandleConvertFetchedSubtitle(t.Context(), job) }()
	command := <-harness.nodes.commanded
	if sent := command.ExtractSubtitle; sent == nil || string(sent.FetchedSubRip) != fetchedText || sent.Video != nil {
		t.Fatalf("the node was sent %+v, want the fetched text to convert (D224 §5)", command)
	}
	playback.TakeSubtitleText(t.Context(), 1, nodeproto.SubtitleText{JobID: 95, VTT: []byte("WEBVTT\n\nHallo")})
	if err := <-finished; err != nil {
		t.Fatalf("the conversion ended %v, want done", err)
	}
}

// checkKeptGzipped fails unless the fetched subtitle is kept in library.db, gzipped (D224 §6).
func checkKeptGzipped(t *testing.T, harness testHarness, fetchedID int64) {
	t.Helper()
	kept, err := harness.database.Read().GetFetchedSubtitle(t.Context(), fetchedID)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(kept.BodyGzip))
	if err != nil {
		t.Fatalf("the kept body is not gzip: %v", err)
	}
	text, err := io.ReadAll(reader)
	if err != nil || string(text) != fetchedText {
		t.Errorf("the kept text is %q, %v; want what was fetched", text, err)
	}
}

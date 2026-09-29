// SPDX-License-Identifier: GPL-3.0-or-later

package opensubtitles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// fakeKeys is the server's OpenSubtitles login, as the secrets store opens it.
type fakeKeys struct{ username, password string }

func (keys fakeKeys) Open(_ context.Context, name secrets.Name) (secrets.Value, error) {
	value := ""
	switch name { //nolint:exhaustive // a fake login answers only for its two names
	case secrets.OpenSubtitlesUsername:
		value = keys.username
	case secrets.OpenSubtitlesPassword:
		value = keys.password
	}
	if value == "" {
		return secrets.Value{}, secrets.ErrNotSet
	}
	return secrets.NewValue(value), nil
}

// fakeProvider is OpenSubtitles, faked: its logins, what it was asked, and a quota.
type fakeProvider struct {
	mutex         sync.Mutex
	loginCount    int
	isTokenIssued map[string]bool
	searched      []url.Values
	remaining     int
	// limitedFor, when set, is a search's answer: 429, with it as the Retry-After unless "none".
	limitedFor string
	server     *httptest.Server
}

func (fake *fakeProvider) serve(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if request.URL.Path == "/files/subtitle.srt" { // the download link: a file, no key asked
		reply(writer, "1\n00:00:01,000 --> 00:00:02,000\nHello\n")
		return
	}
	if request.Header.Get("Api-Key") != apiKey || !strings.HasPrefix(request.Header.Get("User-Agent"), "Orpheum/") {
		http.Error(writer, "who are you", http.StatusForbidden)
		return
	}
	if request.URL.Path == "/api/v1/login" {
		var body map[string]string
		_ = json.NewDecoder(request.Body).Decode(&body) //nolint:errcheck // a test's fake
		if body["password"] != "right" {
			http.Error(writer, `{"message":"Error, invalid username/password"}`, http.StatusUnauthorized)
			return
		}
		fake.loginCount++
		token := fmt.Sprintf("token-%d", fake.loginCount)
		fake.isTokenIssued[token] = true
		host := strings.TrimPrefix(fake.server.URL, "http://")
		reply(writer, fmt.Sprintf(`{"token":%q,"base_url":%q,"status":200}`, token, host))
		return
	}
	if !fake.isTokenIssued[strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")] {
		http.Error(writer, `{"message":"invalid token"}`, http.StatusUnauthorized)
		return
	}
	switch request.URL.Path {
	case "/api/v1/subtitles":
		if fake.limitedFor != "" {
			if fake.limitedFor != "none" {
				writer.Header().Set("Retry-After", fake.limitedFor)
			}
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fake.searched = append(fake.searched, request.URL.Query())
		reply(writer, `{"data":[
			{"attributes":{"language":"en","release":"Film.2020.1080p","moviehash_match":true,
			  "download_count":120,"upload_date":"2024-06-26T07:02:57Z","files":[{"file_id":7}]}},
			{"attributes":{"language":"en","release":"Film.2020.720p","hearing_impaired":true,
			  "files":[{"file_id":8}]}},
			{"attributes":{"language":"en","release":"nothing to download","files":[]}}]}`)
	case "/api/v1/download":
		if fake.remaining == 0 {
			writer.WriteHeader(http.StatusNotAcceptable)
			reply(writer, `{"remaining":-1,"reset_time_utc":"2026-09-29T06:00:00.000Z"}`)
			return
		}
		fake.remaining--
		reply(writer, fmt.Sprintf(`{"link":%q,"file_name":"Film.2020.srt","remaining":%d,`+
			`"reset_time_utc":"2026-09-29T06:00:00.000Z"}`, fake.server.URL+"/files/subtitle.srt", fake.remaining))
	case "/api/v1/infos/user":
		reply(writer, fmt.Sprintf(
			`{"data":{"allowed_downloads":20,"remaining_downloads":%d,"vip":false}}`,
			fake.remaining))
	default:
		http.NotFound(writer, request)
	}
}

// reply writes a fake's answer; a test that stops reading it has already failed.
func reply(writer http.ResponseWriter, text string) {
	_, _ = writer.Write([]byte(text)) //nolint:errcheck // see above
}

func newProvider(t *testing.T, keys LoginSecrets) (*fakeProvider, *Client, *clocktest.Fake) {
	t.Helper()
	fake := &fakeProvider{isTokenIssued: map[string]bool{}, remaining: 2}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC))
	client, err := New(httpClient, fake.server.URL+"/api/v1", keys, fakeClock)
	if err != nil {
		t.Fatal(err)
	}
	return fake, client, fakeClock
}

func TestSearch_LogsInOnceAndAsksByTheWorksIDsAndTheFilesHash(t *testing.T) {
	t.Parallel()
	fake, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	query := Query{IMDbID: "tt0123456", TMDBID: "4801", MovieHash: "8E245D9679D31E12", Language: "EN"}
	found, err := client.Search(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Search(t.Context(), query); err != nil {
		t.Fatal(err)
	}
	if fake.loginCount != 1 {
		t.Errorf("%d logins for two searches, want one, shared (opensubtitles-api.md §3)", fake.loginCount)
	}
	asked := fake.searched[0]
	if asked.Get("imdb_id") != "133093" || asked.Get("tmdb_id") != "603" ||
		asked.Get("moviehash") != "8e245d9679d31e12" || asked.Get("languages") != "en" {
		t.Errorf("searched with %v; want the ids without tt, the hash and language lower-case (D224 §3)", asked)
	}
	if len(found) != 2 || found[0].ProviderFileID != 7 || !found[0].IsHashMatch || found[0].DownloadCount != 120 ||
		found[0].UploadedAt.IsZero() || found[1].ProviderFileID != 8 || !found[1].IsHearingImpaired {
		t.Errorf("found %+v; want the two with a file, the hash match first", found)
	}
}

func TestSearch_AsksForAnEpisodeByItsSeriesAndPlace(t *testing.T) {
	t.Parallel()
	fake, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	_, err := client.Search(t.Context(), Query{SeriesTMDBID: "1399", Season: 1, Episode: 3, Language: "de"})
	if err != nil {
		t.Fatal(err)
	}
	asked := fake.searched[0]
	if asked.Get("parent_tmdb_id") != "1399" || asked.Get("season_number") != "1" ||
		asked.Get("episode_number") != "3" || asked.Has("imdb_id") {
		t.Errorf("searched with %v; want the series and the episode's place", asked)
	}
}

func TestCurrentSession_IsUsedForItsDayAndMadeAgainWhenOpenSubtitlesNoLongerAcceptsIt(t *testing.T) {
	t.Parallel()
	fake, client, now := newProvider(t, fakeKeys{"owner", "right"})
	if _, err := client.Allowance(t.Context()); err != nil {
		t.Fatal(err)
	}
	now.Advance(tokenLife)
	if _, err := client.Allowance(t.Context()); err != nil || fake.loginCount != 2 {
		t.Errorf("a day later: %v, %d logins; want a new login", err, fake.loginCount)
	}
	fake.mutex.Lock()
	fake.isTokenIssued = map[string]bool{} // OpenSubtitles forgets the token
	fake.mutex.Unlock()
	allowance, err := client.Allowance(t.Context())
	if err != nil || fake.loginCount != 3 || allowance.AllowedDownloads != 20 || allowance.RemainingDownloads != 2 {
		t.Errorf("after the token was refused: %+v, %v, %d logins; want one new login and the allowance",
			allowance, err, fake.loginCount)
	}
}

func TestCurrentSession_ARefusedLoginIsNotTriedAgainUntilItChanges(t *testing.T) {
	t.Parallel()
	_, client, _ := newProvider(t, fakeKeys{"owner", "wrong"})
	for range 2 {
		if _, err := client.Allowance(t.Context()); !errors.Is(err, ErrLoginRefused) {
			t.Errorf("with a wrong password: %v, want ErrLoginRefused", err)
		}
	}
	client.loginSecrets = fakeKeys{"owner", "right"}
	if _, err := client.Allowance(t.Context()); err != nil {
		t.Errorf("after the admin changed it: %v, want it tried", err)
	}
	_, unset, _ := newProvider(t, fakeKeys{})
	if _, err := unset.Search(t.Context(), Query{Language: "en"}); !errors.Is(err, ErrNoLogin) {
		t.Errorf("with no login set: %v, want ErrNoLogin", err)
	}
}

func TestDownload_FetchesTheFileAndSaysWhatIsLeftAndASpentQuotaWhenItResets(t *testing.T) {
	t.Parallel()
	_, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	downloaded, err := client.Download(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	reset := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	if !strings.Contains(string(downloaded.Body), "Hello") || downloaded.RemainingDownloads != 1 ||
		!downloaded.ResetAt.Equal(reset) || downloaded.FileName != "Film.2020.srt" {
		t.Errorf("downloaded %+v; want the subtitle, one left, and the provider's reset time (D224 §4)", downloaded)
	}
	if _, err = client.Download(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	_, err = client.Download(t.Context(), 7)
	spent, isSpent := errors.AsType[*QuotaSpentError](err)
	if !isSpent || !spent.ResetAt.Equal(reset) {
		t.Errorf("a download past the quota: %v, want the quota spent until %s", err, reset)
	}
}

func TestSearch_ARateLimitIsTheOneEveryProviderReturns(t *testing.T) {
	t.Parallel()
	fake, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	for limitedFor, want := range map[string]time.Duration{"5": 5 * time.Second, "none": time.Second} {
		fake.mutex.Lock()
		fake.limitedFor = limitedFor
		fake.mutex.Unlock()
		_, err := client.Search(t.Context(), Query{TMDBID: "4801", Language: "en"})
		if limited, isLimited := errors.AsType[*outbound.RateLimitedError](err); !isLimited ||
			limited.RetryAfter != want || limited.Provider != "OpenSubtitles" {
			t.Errorf("a 429 with Retry-After %s: %v, want OpenSubtitles' RateLimitedError after %s",
				limitedFor, err, want)
		}
	}
}

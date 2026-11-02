// SPDX-License-Identifier: AGPL-3.0-or-later

package opensubtitles

import (
	"cmp"
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

	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/providerstest"
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
	refusedLogins int
	isTokenIssued map[string]bool
	searched      []url.Values
	remaining     int
	// limitedFor, when set, is a search's answer: 429, with it as the Retry-After unless "none".
	limitedFor string
	// searchStatus and linkStatus, when set, are what a search and the download link answer;
	// linkFailures, when set, is how many times the link answers linkStatus before it works.
	searchStatus, linkStatus, linkFailures int
	// linkCount counts the link's fetches.
	linkCount int
	// isDroppedTokenA500 answers a token it does not know as OpenSubtitles answers an expired one:
	// 500 "invalid", not 401.
	isDroppedTokenA500 bool
	// refusedLoginStatus, when set, is how a wrong login is answered, in place of 401.
	refusedLoginStatus int
	// loginHost and linkAddress, when set, are the host a login names and the download link.
	loginHost, linkAddress string
	// releaseName and fileName, when set, are the first candidate's release and a download's file
	// name, as a JSON string.
	releaseName, fileName string
	server                *httptest.Server
}

func (fake *fakeProvider) serve(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if request.URL.Path == "/files/subtitle.srt" { // the download link: a file, no key asked
		fake.serveLink(writer)
		return
	}
	if request.Header.Get("Api-Key") != apiKey || !strings.HasPrefix(request.Header.Get("User-Agent"), "Orpheum/") {
		http.Error(writer, "who are you", http.StatusForbidden)
		return
	}
	if request.URL.Path == "/api/v1/login" {
		fake.serveLogin(writer, request)
		return
	}
	if !fake.isTokenIssued[strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")] {
		status := http.StatusUnauthorized
		if fake.isDroppedTokenA500 {
			status = http.StatusInternalServerError
		}
		http.Error(writer, `{"message":"invalid token"}`, status)
		return
	}
	switch request.URL.Path {
	case "/api/v1/subtitles":
		if fake.searchStatus != 0 {
			http.Error(writer, "the database is down", fake.searchStatus)
			return
		}
		if fake.limitedFor != "" {
			if fake.limitedFor != "none" {
				writer.Header().Set("Retry-After", fake.limitedFor)
			}
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fake.searched = append(fake.searched, request.URL.Query())
		reply(writer, `{"data":[
			{"attributes":{"language":"en","release":`+cmp.Or(fake.releaseName, `"Film.2020.1080p"`)+`,
			  "moviehash_match":true,
			  "download_count":120,"upload_date":"2024-06-26T07:02:57Z","files":[{"file_id":7}]}},
			{"attributes":{"language":"en","release":"Film.2020.720p","hearing_impaired":true,
			  "files":[{"file_id":8}]}},
			{"attributes":{"language":"en","release":"nothing to download","files":[]}}]}`)
	case "/api/v1/download":
		fake.serveDownload(writer)
	case "/api/v1/infos/user":
		reply(writer, fmt.Sprintf(
			`{"data":{"allowed_downloads":20,"remaining_downloads":%d,"vip":false}}`,
			fake.remaining))
	default:
		http.NotFound(writer, request)
	}
}

// serveLogin answers a login: a new token and the host to send its requests to, for the right password.
func (fake *fakeProvider) serveLogin(writer http.ResponseWriter, request *http.Request) {
	var body map[string]string
	_ = json.NewDecoder(request.Body).Decode(&body) //nolint:errcheck // a test's fake
	if body["password"] != "right" {
		fake.refusedLogins++
		http.Error(writer, `{"message":"Error, invalid username/password"}`,
			cmp.Or(fake.refusedLoginStatus, http.StatusUnauthorized))
		return
	}
	fake.loginCount++
	token := fmt.Sprintf("token-%d", fake.loginCount)
	fake.isTokenIssued[token] = true
	host := strings.TrimPrefix(fake.server.URL, "http://")
	if fake.loginHost != "" {
		host = fake.loginHost
	}
	reply(writer, fmt.Sprintf(`{"token":%q,"base_url":%q,"user":{"allowed_downloads":20},"status":200}`,
		token, host))
}

// serveDownload answers a download: the link and what is left, or a spent quota.
func (fake *fakeProvider) serveDownload(writer http.ResponseWriter) {
	if fake.remaining == 0 {
		writer.WriteHeader(http.StatusNotAcceptable)
		reply(writer, `{"remaining":-1,"reset_time_utc":"2026-09-29T06:00:00.000Z"}`)
		return
	}
	fake.remaining--
	link := fake.server.URL + "/files/subtitle.srt"
	if fake.linkAddress != "" {
		link = fake.linkAddress
	}
	reply(writer, fmt.Sprintf(`{"link":%q,"file_name":%s,"remaining":%d,`+
		`"reset_time_utc":"2026-09-29T06:00:00.000Z"}`, link, cmp.Or(fake.fileName, `"Film.2020.srt"`), fake.remaining))
}

// serveLink answers the download link: the subtitle, or linkStatus when it is set.
func (fake *fakeProvider) serveLink(writer http.ResponseWriter) {
	fake.linkCount++
	if fake.linkStatus != 0 && (fake.linkFailures == 0 || fake.linkCount <= fake.linkFailures) {
		writer.Header().Set("Retry-After", "3")
		writer.WriteHeader(fake.linkStatus)
		return
	}
	reply(writer, "1\n00:00:01,000 --> 00:00:02,000\nHello\n")
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
	client, err := New(httpClient, fake.server.URL+"/api/v1", keys, fakeClock,
		Service(providerstest.NewHealth(t, fakeClock), fakeClock, providerstest.Settings{}))
	if err != nil {
		t.Fatal(err)
	}
	client.linkRetryDelays = []time.Duration{0, 0} // the back-off's count, not its wall time
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
	if len(found) != 2 || found[0].ProviderFileID != 7 || !found[0].IsOpensubtitlesHashMatch ||
		found[0].DownloadCount != 120 ||
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

func TestCurrentSession_ALoginAnswered400IsRefused(t *testing.T) {
	t.Parallel()
	fake, client, _ := newProvider(t, fakeKeys{"owner@example.com", "wrong"})
	fake.refusedLoginStatus = http.StatusBadRequest
	for range 2 {
		if _, err := client.Allowance(t.Context()); !errors.Is(err, ErrLoginRefused) {
			t.Errorf("a login OpenSubtitles answers 400: %v, want ErrLoginRefused, as it answers a wrong one", err)
		}
	}
	if fake.loginCount != 0 || fake.refusedLogins != 1 {
		t.Errorf("%d logins refused, want one: a refused login is not tried again", fake.refusedLogins)
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
		!downloaded.DownloadsResetAt.Equal(reset) || downloaded.FileName != "Film.2020.srt" {
		t.Errorf("downloaded %+v; want the subtitle, one left, and the provider's reset time (D224 §4)", downloaded)
	}
	if _, err = client.Download(t.Context(), 8); err != nil {
		t.Fatal(err)
	}
	_, err = client.Download(t.Context(), 9)
	spent, isSpent := errors.AsType[*providers.QuotaSpentError](err)
	if !isSpent || !spent.ResetAt.Equal(reset) {
		t.Errorf("a download past the quota: %v, want the quota spent until %s", err, reset)
	}
}

func TestSearch_ARateLimitIsTheOneEveryProviderReturns(t *testing.T) {
	t.Parallel()
	fake, client, fakeClock := newProvider(t, fakeKeys{"owner", "right"})
	for limitedFor, want := range map[string]time.Duration{"5": 5 * time.Second, "none": time.Second} {
		fakeClock.Advance(time.Minute) // past the last wait, which the provider keeps for every caller
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

func TestRequests_AProviderThatFailsOrCannotBeReachedIsUnavailableWhereverItFails(t *testing.T) {
	t.Parallel()
	fake, client, fakeClock := newProvider(t, fakeKeys{"owner", "right"})
	fake.mutex.Lock()
	fake.searchStatus = http.StatusServiceUnavailable
	fake.mutex.Unlock()
	_, err := client.Search(t.Context(), Query{TMDBID: "4801", Language: "en"})
	if !errors.Is(err, outbound.ErrUnavailable) {
		t.Errorf("a search answered 503: %v, want OpenSubtitles unavailable", err)
	}
	fake.mutex.Lock()
	fake.linkStatus = http.StatusBadGateway
	fake.mutex.Unlock()
	if _, err = client.Download(t.Context(), 7); !errors.Is(err, outbound.ErrUnavailable) {
		t.Errorf("a download link answered 502: %v, want OpenSubtitles unavailable", err)
	}
	fake.mutex.Lock()
	fake.linkStatus = http.StatusTooManyRequests
	fake.mutex.Unlock()
	_, err = client.Download(t.Context(), 7)
	if limited, isLimited := errors.AsType[*outbound.RateLimitedError](err); !isLimited ||
		limited.RetryAfter != 3*time.Second || limited.Provider != "OpenSubtitles" {
		t.Errorf("a download link answered 429: %v, want OpenSubtitles' RateLimitedError after 3s", err)
	}
	fake.server.Close()
	fakeClock.Advance(time.Minute) // past the 429's wait, which the provider keeps for every caller
	if _, err := client.Allowance(t.Context()); !errors.Is(err, outbound.ErrUnavailable) ||
		strings.Contains(err.Error(), "infos/user") {
		t.Errorf("OpenSubtitles gone: %v, want it unavailable, without the URL", err)
	}
}

func TestCurrentSession_ATokenAnsweredInvalidWithA500IsMadeAgainOnce(t *testing.T) {
	t.Parallel()
	fake, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	if _, err := client.Allowance(t.Context()); err != nil {
		t.Fatal(err)
	}
	fake.mutex.Lock()
	fake.isDroppedTokenA500 = true
	fake.isTokenIssued = map[string]bool{} // OpenSubtitles drops the token
	fake.mutex.Unlock()
	if _, err := client.Allowance(t.Context()); err != nil || fake.loginCount != 2 {
		t.Errorf("after the token was answered invalid: %v, %d logins; want one new login", err, fake.loginCount)
	}
	fake.mutex.Lock()
	fake.searchStatus = http.StatusInternalServerError
	fake.mutex.Unlock()
	_, err := client.Search(t.Context(), Query{TMDBID: "4801", Language: "en"})
	if err == nil || errors.Is(err, outbound.ErrUnavailable) || fake.loginCount != 2 {
		t.Errorf("a 500 that is not about the token: %v, %d logins; want the search's own failure, no new login",
			err, fake.loginCount)
	}
}

// elsewhere is another host on loopback, which counts what it is asked.
func elsewhere(t *testing.T) (address string, askedCount func() int) {
	t.Helper()
	var mutex sync.Mutex
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		count++
		reply(writer, `{"token":"stolen","data":{}}`)
	}))
	t.Cleanup(server.Close)
	return server.URL, func() int {
		mutex.Lock()
		defer mutex.Unlock()
		return count
	}
}

func TestCurrentSession_ALoginNamingAnotherHostThanOpenSubtitlesKeepsTheBase(t *testing.T) {
	t.Parallel()
	fake, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	other, askedCount := elsewhere(t)
	fake.mutex.Lock()
	fake.loginHost = strings.TrimPrefix(other, "http://")
	fake.mutex.Unlock()
	allowance, err := client.Allowance(t.Context())
	if err != nil || allowance.AllowedDownloads != 20 || askedCount() != 0 {
		t.Errorf("a login naming %s: %+v, %v, %d requests there; want the base asked and the key and token "+
			"sent nowhere else", other, allowance, err, askedCount())
	}
}

func TestDownload_ALinkToAnotherHostThanOpenSubtitlesIsNotFetched(t *testing.T) {
	t.Parallel()
	fake, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	other, askedCount := elsewhere(t)
	fake.mutex.Lock()
	fake.linkAddress = other + "/files/subtitle.srt"
	fake.mutex.Unlock()
	if _, err := client.Download(t.Context(), 7); err == nil || askedCount() != 0 {
		t.Errorf("a link to %s: %v, %d requests there; want it refused unasked", other, err, askedCount())
	}
}

func TestIsOwnHost_IsOpenSubtitlesDomainOrTheBasesHost(t *testing.T) {
	t.Parallel()
	_, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	for host, want := range map[string]bool{
		"api.opensubtitles.com": true, "vip-api.opensubtitles.com": true, "opensubtitles.com": true,
		"www.OpenSubtitles.com": true, client.base.Host: true,
		"": false, "evil.example": false, "opensubtitles.com.evil.example": false, "evilopensubtitles.com": false,
		"api.opensubtitles.com:8443": false, "user@api.opensubtitles.com": false,
		"evil.example#.opensubtitles.com": false,
	} {
		if got := client.isOwnHost(host); got != want {
			t.Errorf("isOwnHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestRequests_TextOpenSubtitlesSendsIsCleanedWhereItEnters(t *testing.T) {
	t.Parallel()
	fake, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	// A right-to-left override makes "Film.2020.srt\u202Egnp.exe" read as ending in .png; a
	// control and a zero-width space hide in the rest (D211 §2).
	fake.releaseName = `"Film.2020\u202E\u200B.1080p\u0007"`
	fake.fileName = `"Film.2020\u202Etrs.exe\u0000"`
	candidates, err := client.Search(t.Context(), Query{IMDbID: "tt0000001", Language: "en"})
	if err != nil || len(candidates) == 0 {
		t.Fatalf("search: %v, %v", candidates, err)
	}
	if got := candidates[0].ReleaseName; got != "Film.2020.1080p" {
		t.Errorf("the release name: %q, want it cleaned to %q", got, "Film.2020.1080p")
	}
	downloaded, err := client.Download(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if downloaded.FileName != "Film.2020trs.exe" {
		t.Errorf("the file name: %q, want it cleaned to %q", downloaded.FileName, "Film.2020trs.exe")
	}
}

// A download is spent when /download answers, and its link lasts three hours
// (opensubtitles-api.md §2): a link that fails is retried, with back-off, and fetching the file
// again while the link works spends nothing.
func TestDownload_ALinkThatFailsIsRetriedWhileItWorksAndSpendsNoSecondDownload(t *testing.T) {
	t.Parallel()
	fake, client, fakeClock := newProvider(t, fakeKeys{"owner", "right"})
	fake.remaining = 10
	spent := func() int {
		fake.mutex.Lock()
		defer fake.mutex.Unlock()
		return 10 - fake.remaining
	}
	fake.mutex.Lock()
	fake.linkStatus, fake.linkFailures = http.StatusServiceUnavailable, 2
	fake.mutex.Unlock()
	downloaded, err := client.Download(t.Context(), 7)
	if err != nil || !strings.Contains(string(downloaded.Body), "Hello") {
		t.Fatalf("a link that answered 503 twice: %v, want the third try to fetch it", err)
	}
	fake.mutex.Lock()
	fake.linkCount, fake.linkFailures = 0, 5 // more than one Download retries
	fake.mutex.Unlock()
	if _, err := client.Download(t.Context(), 8); !errors.Is(err, outbound.ErrUnavailable) {
		t.Fatalf("a link that keeps answering 503: %v, want it unavailable", err)
	}
	fakeClock.Advance(2 * time.Hour)
	if _, err := client.Download(t.Context(), 8); err != nil {
		t.Fatalf("the same file within the link's three hours: %v, want its link fetched", err)
	}
	if got := spent(); got != 2 {
		t.Errorf("%d downloads spent, want one per file: a failed link is fetched again, not bought again", got)
	}
	if _, err := client.Download(t.Context(), 7); err != nil || spent() != 2 {
		t.Errorf("file 7 again within its link's time: %v, %d spent; want no second download", err, spent())
	}
	fakeClock.Advance(time.Hour + time.Second)
	if _, err := client.Download(t.Context(), 8); err != nil || spent() != 3 {
		t.Errorf("file 8 after its link's three hours: %v, %d spent; want a new download", err, spent())
	}
	fake.mutex.Lock()
	fake.linkStatus, fake.linkFailures = http.StatusNotFound, 0
	fake.mutex.Unlock()
	if _, err := client.Download(t.Context(), 8); err == nil {
		t.Fatal("a link answering 404: nothing, want a refusal")
	}
	fake.mutex.Lock()
	fake.linkStatus = 0
	fake.mutex.Unlock()
	if _, err := client.Download(t.Context(), 8); err != nil || spent() != 4 {
		t.Errorf("after a link answered 404: %v, %d spent; want it forgotten and a new download", err, spent())
	}
}

func TestDownload_BackgroundNeverSpendsTheDownloadsHeldBackForPeopleAndAsksNothing(t *testing.T) {
	t.Parallel()
	// The login says a free account's 20 a day; opensubtitles_downloads_reserved_for_people_percent
	// starts at 25: 5 held back.
	const heldBack = 5
	fake, client, _ := newProvider(t, fakeKeys{"owner", "right"})
	fake.remaining = heldBack + 1
	if _, err := client.Download(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	background := providers.WithImportance(t.Context(), providers.Background)
	_, err := client.Download(background, 8)
	if spent, isSpent := errors.AsType[*providers.QuotaSpentError](err); !isSpent || spent == nil {
		t.Errorf("background with %d downloads left: %v; want them held back for people (D231 §5)", heldBack, err)
	}
	fake.mutex.Lock()
	remaining := fake.remaining
	fake.mutex.Unlock()
	if remaining != heldBack {
		t.Errorf("OpenSubtitles has %d left, want %d: background asked for a download it must not spend",
			remaining, heldBack)
	}
	if _, err := client.Download(t.Context(), 8); err != nil {
		t.Errorf("a person's download with the last %d left: %v, want it made", heldBack, err)
	}
}

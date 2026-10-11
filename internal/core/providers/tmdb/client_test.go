// SPDX-License-Identifier: AGPL-3.0-or-later

package tmdb

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/providerstest"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

type fixedKey string

func (key fixedKey) Open(context.Context, secrets.Name) (secrets.Value, error) {
	if key == "" {
		return secrets.Value{}, secrets.ErrNotSet
	}
	return secrets.NewValue(string(key)), nil
}

// fakeTMDB is a TMDB that answers from a table of paths, and records each request.
func fakeTMDB(t *testing.T, answers map[string]string) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request)
		body, found := answers[request.URL.Path]
		if !found {
			http.NotFound(writer, request)
			return
		}
		contentType := "application/json"
		if strings.HasSuffix(request.URL.Path, ".jpg") {
			contentType = "image/jpeg"
		}
		writer.Header().Set("Content-Type", contentType)
		_, _ = writer.Write([]byte(body)) //nolint:errcheck // a test server
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func newClient(t *testing.T, base string, key fixedKey) *Client {
	t.Helper()
	return newClientOn(t, base, key, clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)))
}

// newClientOn is newClient with its provider service on fakeClock.
func newClientOn(t *testing.T, base string, key fixedKey, fakeClock *clocktest.Fake) *Client {
	t.Helper()
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	service := APIService(providerstest.NewHealth(t, fakeClock), fakeClock, providerstest.Settings{})
	return New(httpClient, base, key, service)
}

// newImageFetcher is TMDB's image downloader calling base, with its provider service on a fake
// clock.
func newImageFetcher(t *testing.T, httpClient *http.Client, base string) *ImageFetcher {
	t.Helper()
	return newImageFetcherOn(t, httpClient, base, clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)))
}

// newImageFetcherOn is newImageFetcher with its provider service on fakeClock.
func newImageFetcherOn(t *testing.T, httpClient *http.Client, base string, fakeClock *clocktest.Fake) *ImageFetcher {
	t.Helper()
	service := ImagesService(providerstest.NewHealth(t, fakeClock), fakeClock, providerstest.Settings{})
	return NewImageFetcher(httpClient, base, service)
}

func TestSearch_AsksForTheNameAndYearAndReadsTheResults(t *testing.T) {
	t.Parallel()
	server, seen := fakeTMDB(t, map[string]string{"/search/movie": `{"page":1,"results":[
		{"id":4801,"title":"The Gold Rush","original_title":"The Gold Rush","release_date":"1999-03-31",
		 "poster_path":"/m.jpg"},
		{"id":1,"title":"Gold Rush Reloaded","original_title":"x","release_date":""}]}`})
	query := identify.Query{Kind: "movie", Name: "The Gold Rush", Year: 1999}
	candidates, err := newClient(t, server.URL, "v3key").Search(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	want := identify.Candidate{
		ID: "4801", Name: "The Gold Rush", OriginalName: "The Gold Rush", Year: 1999, ProviderPosterPath: "/m.jpg",
	}
	if len(candidates) != 2 || candidates[0] != want || candidates[1].Year != 0 {
		t.Errorf("candidates = %+v", candidates)
	}
	asked := (*seen)[0].URL.Query()
	if asked.Get("query") != "The Gold Rush" || asked.Get("year") != "1999" || asked.Get("api_key") != "v3key" {
		t.Errorf("the search asked %v", asked)
	}
}

func TestGet_SendsAReadAccessTokenAsABearerToken(t *testing.T) {
	t.Parallel()
	server, seen := fakeTMDB(t, map[string]string{"/search/movie": `{"results":[]}`})
	token := fixedKey("eyJhbGciOiJIUzI1NiJ9.e30.x")
	_, err := newClient(t, server.URL, token).Search(t.Context(), identify.Query{Kind: "movie", Name: "Metropolis"})
	if err != nil {
		t.Fatal(err)
	}
	request := (*seen)[0]
	if request.Header.Get("Authorization") != "Bearer "+string(token) || request.URL.Query().Has("api_key") {
		t.Errorf("header %q, query %v", request.Header.Get("Authorization"), request.URL.Query())
	}
	if request.URL.Query().Has("year") {
		t.Error("a search with no year asked for one")
	}
}

func TestFetchAndParse_AFilmsDetails(t *testing.T) {
	t.Parallel()
	server, seen := fakeTMDB(t, map[string]string{"/movie/9019": `{"id":9019,"title":"Metropolis","original_title":"Metropolis",
		"original_language":"en","overview":"Obsessive...","tagline":"A Los Angeles crime saga",
		"release_date":"1995-12-15",
		"runtime":170,"imdb_id":"tt0234567","poster_path":"/metropolis.jpg","backdrop_path":"/heat-wide.jpg",
		"credits":{"cast":[{"name":"Ansel Vardo"}]}}`})
	client := newClient(t, server.URL, "k")
	body, err := client.Fetch(t.Context(), "movie", "949")
	if err != nil {
		t.Fatal(err)
	}
	if got := (*seen)[0].URL.Query().Get("append_to_response"); got != "credits,external_ids" {
		t.Errorf("append_to_response = %q", got)
	}
	film, err := client.Parse("movie", body)
	if err != nil {
		t.Fatal(err)
	}
	if film.ID != "9019" || film.Name != "Metropolis" || film.Year != 1995 || film.RuntimeMs != 170*60_000 ||
		film.OriginalLang != "en" || film.Tagline == "" ||
		!maps.Equal(film.IDs, map[string]string{"tmdb": "9019", "imdb": "tt0234567"}) ||
		!slices.Equal(film.Artwork, []identify.ImageRef{
			{Kind: library.ArtPoster, Ref: "/metropolis.jpg"}, {Kind: library.ArtBackdrop, Ref: "/heat-wide.jpg"},
		}) {
		t.Errorf("parsed %+v", film)
	}
}

func TestFind_TurnsAnIMDbIDIntoTMDBs(t *testing.T) {
	t.Parallel()
	server, seen := fakeTMDB(t, map[string]string{
		"/find/tt0234567": `{"movie_results":[{"id":9019}]}`,
		"/find/tt0000000": `{"movie_results":[]}`,
	})
	client := newClient(t, server.URL, "k")
	id, err := client.Find(t.Context(), "movie", "imdb", "tt0234567")
	if err != nil || id != "949" {
		t.Errorf("Find = %q, %v", id, err)
	}
	if got := (*seen)[0].URL.Query().Get("external_source"); got != "imdb_id" {
		t.Errorf("external_source = %q", got)
	}
	if _, err := client.Find(t.Context(), "movie", "imdb", "tt0000000"); !errors.Is(err, identify.ErrNoMatch) {
		t.Errorf("Find of an unknown id = %v, want ErrNoMatch", err)
	}
}

func TestGet_TurnsTMDBsRefusalsIntoTheErrorsTheMatcherActsOn(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/movie/1":
			writer.Header().Set("Retry-After", "7")
			writer.WriteHeader(http.StatusTooManyRequests)
		case "/movie/2":
			writer.WriteHeader(http.StatusUnauthorized)
		case "/movie/3":
			writer.WriteHeader(http.StatusInternalServerError)
		case "/movie/5":
			writer.WriteHeader(http.StatusBadGateway)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	client := newClientOn(t, server.URL, "k", fakeClock)
	_, err := client.Fetch(t.Context(), "movie", "1")
	var limited *outbound.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter != 7*time.Second {
		t.Errorf("429 = %v, want RateLimitedError after 7s", err)
	}
	fakeClock.Advance(7 * time.Second) // the provider keeps every caller waiting until then
	if _, err := client.Fetch(t.Context(), "movie", "2"); !errors.Is(err, identify.ErrKeyRefused) {
		t.Errorf("401 = %v, want ErrKeyRefused", err)
	}
	// A 502 is TMDB down; a plain 500 is the request's own failure (D231 §5). Each names its status.
	for id, status := range map[string]string{"3": "500", "5": "502"} {
		_, err := client.Fetch(t.Context(), "movie", id)
		if err == nil || errors.Is(err, outbound.ErrUnavailable) != (status == "502") ||
			!strings.Contains(err.Error(), status) {
			t.Errorf("%s = %v, want ErrUnavailable for a 502 only, naming the status", status, err)
		}
	}
	if _, err := client.Fetch(t.Context(), "movie", "4"); !errors.Is(err, identify.ErrNoMatch) {
		t.Errorf("404 = %v, want ErrNoMatch", err)
	}
	if _, err := newClient(t, server.URL, "").Fetch(t.Context(), "movie", "4"); !errors.Is(err, identify.ErrNoKey) {
		t.Errorf("with no key = %v, want ErrNoKey", err)
	}
}

func TestGet_AFailedConnectionDoesNotPrintTheKey(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	_, err := newClient(t, server.URL, "the-secret-key").Search(t.Context(), identify.Query{Kind: "movie", Name: "x"})
	if !errors.Is(err, outbound.ErrUnavailable) || strings.Contains(err.Error(), "the-secret-key") {
		t.Errorf("error %v; want TMDB unavailable, without showing the key", err)
	}
}

// orbitoramaAtTMDB is a fake TMDB that knows one series and one season of it.
func orbitoramaAtTMDB(t *testing.T) (*Client, *[]*http.Request) {
	t.Helper()
	server, seen := fakeTMDB(t, map[string]string{
		"/search/tv": `{"results":[{"id":7615,"name":"Orbitorama","original_name":"Orbitorama",
			"first_air_date":"1999-03-28","poster_path":"/f.jpg"}]}`,
		"/find/73871": `{"movie_results":[],"tv_results":[{"id":7615}]}`,
		"/tv/7615": `{"id":7615,"name":"Orbitorama","original_name":"Orbitorama","original_language":"en",
			"overview":"The adventures of a delivery boy.","first_air_date":"1999-03-28","episode_run_time":[22],
			"external_ids":{"imdb_id":"tt0900460","tvdb_id":78871}}`,
		"/tv/7615/season/2": `{"season_number":2,"episodes":[
			{"id":1101,"season_number":2,"episode_number":10,"name":"A Comet of My Own",
			 "overview":"The Professor names his successor.","air_date":"2000-04-09","runtime":22,
			 "still_path":"/clone.jpg"}]}`,
	})
	return newClient(t, server.URL, "k"), seen
}

func TestSeries_SearchAndFind(t *testing.T) {
	t.Parallel()
	client, seen := orbitoramaAtTMDB(t)
	query := identify.Query{Kind: identify.KindSeries, Name: "Orbitorama", Year: 1999}
	candidates, err := client.Search(t.Context(), query)
	if err != nil || len(candidates) != 1 || candidates[0].Name != "Orbitorama" || candidates[0].Year != 1999 {
		t.Errorf("Search = %+v, %v", candidates, err)
	}
	if got := (*seen)[0].URL.Query().Get("first_air_date_year"); got != "1999" {
		t.Errorf("a series search asked first_air_date_year=%q", got)
	}
	if id, err := client.Find(t.Context(), identify.KindSeries, "tvdb", "78871"); err != nil || id != "615" {
		t.Errorf("Find by TVDB id = %q, %v", id, err)
	}
}

func TestSeries_FetchParseAndItsSeasons(t *testing.T) {
	t.Parallel()
	client, _ := orbitoramaAtTMDB(t)
	body, err := client.Fetch(t.Context(), identify.KindSeries, "615")
	if err != nil {
		t.Fatal(err)
	}
	series, err := client.Parse(identify.KindSeries, body)
	wantIDs := map[string]string{"tmdb": "7615", "imdb": "tt0900460", "tvdb": "78871"}
	if err != nil || series.Name != "Orbitorama" || series.Year != 1999 || series.RuntimeMs != 0 ||
		!maps.Equal(series.IDs, wantIDs) {
		t.Errorf("Parse = %+v, %v", series, err)
	}
	listing, err := client.FetchSeason(t.Context(), "615", 2)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := client.ParseSeason(listing)
	want := identify.Episode{
		ID: "1101", Season: 2, Number: 10, Name: "A Comet of My Own", Overview: "The Professor names his successor.",
		Year: 2000, RuntimeMs: 22 * 60_000, StillRef: "/clone.jpg",
	}
	if err != nil || len(episodes) != 1 || episodes[0] != want {
		t.Errorf("ParseSeason = %+v, %v", episodes, err)
	}
}

func TestGet_RefusesAnAnswerLargerThanItsBoundRatherThanReadingItCutShort(t *testing.T) {
	t.Parallel()
	// Cut at the bound, this answer still parses: only refusing it shows it was too large.
	padded := `{"results":[]}` + strings.Repeat(" ", maxResponseBytes)
	server, _ := fakeTMDB(t, map[string]string{"/search/movie": padded})
	_, err := newClient(t, server.URL, "key").Search(t.Context(), identify.Query{Kind: "movie", Name: "Metropolis"})
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("an answer past the bound: %v, want it refused", err)
	}
}

func TestGet_A429WithoutRetryAfterIsARateLimitStill(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)
	_, err := newClient(t, server.URL, "k").Fetch(t.Context(), "movie", "1")
	limited, isLimited := errors.AsType[*outbound.RateLimitedError](err)
	if !isLimited || limited.RetryAfter != time.Second || limited.Provider != "TMDB" {
		t.Errorf("429 without Retry-After = %v, want TMDB's RateLimitedError after a second", err)
	}
}

func TestGet_AsksTMDBsProviderFirstAndSendsNothingToAParkedHostForTheBackground(t *testing.T) {
	t.Parallel()
	server, seen := fakeTMDB(t, map[string]string{"/search/movie": `{"results":[]}`})
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	health := providerstest.NewHealth(t, fakeClock)
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	client := New(httpClient, server.URL, fixedKey("k"), APIService(health, fakeClock, providerstest.Settings{}))
	health.Park(t.Context(), jobs.Parking{Destination: jobs.DestinationTMDBAPI, Reason: "unreachable"})
	query := identify.Query{Kind: "movie", Name: "Metropolis"}
	background := providers.WithImportance(t.Context(), providers.Background)
	if _, err := client.Search(background, query); !errors.Is(err, outbound.ErrUnavailable) || len(*seen) != 0 {
		t.Errorf("background asking a parked TMDB: %v, %d requests; want it refused as unavailable, nothing sent",
			err, len(*seen))
	}
	if _, err := client.Search(t.Context(), query); err != nil || len(*seen) != 1 {
		t.Errorf("a person asking: %v, %d requests; want it sent, the host's probe (D231 §6)", err, len(*seen))
	}
	if _, isParked := health.DestinationState(jobs.DestinationTMDBAPI); isParked {
		t.Error("TMDB is still parked after a person's call reached it")
	}
}

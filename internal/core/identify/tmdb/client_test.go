// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

type fixedKey string

func (key fixedKey) Open(context.Context, secrets.Name, *int64) (secrets.Value, error) {
	if key == "" {
		return secrets.Value{}, secrets.ErrNotSet
	}
	return secrets.NewValue(string(key)), nil
}

// fake is a TMDB that answers from a table of paths, and records each request.
func fake(t *testing.T, answers map[string]string) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request)
		body, found := answers[request.URL.Path]
		if !found {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body)) //nolint:errcheck // a test server
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func newClient(t *testing.T, base string, key fixedKey) *Client {
	t.Helper()
	httpClient, err := outbound.NewClient(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return New(httpClient, base, key)
}

func TestSearch_AsksForTheNameAndYearAndReadsTheResults(t *testing.T) {
	t.Parallel()
	server, seen := fake(t, map[string]string{"/search/movie": `{"page":1,"results":[
		{"id":4801,"title":"The Gold Rush","original_title":"The Gold Rush","release_date":"1999-03-31",
		 "poster_path":"/m.jpg"},
		{"id":1,"title":"Gold Rush Reloaded","original_title":"x","release_date":""}]}`})
	query := identify.Query{Kind: "movie", Name: "The Gold Rush", Year: 1999}
	candidates, err := newClient(t, server.URL, "v3key").Search(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	want := identify.Candidate{
		ID: "4801", Name: "The Gold Rush", OriginalName: "The Gold Rush", Year: 1999, PosterRef: "/m.jpg",
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
	server, seen := fake(t, map[string]string{"/search/movie": `{"results":[]}`})
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
	server, seen := fake(t, map[string]string{"/movie/9019": `{"id":9019,"title":"Metropolis","original_title":"Metropolis",
		"original_language":"en","overview":"Obsessive...","tagline":"A Los Angeles crime saga",
		"release_date":"1995-12-15",
		"runtime":170,"imdb_id":"tt0234567","credits":{"cast":[{"name":"Ansel Vardo"}]}}`})
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
		!maps.Equal(film.IDs, map[string]string{"tmdb": "9019", "imdb": "tt0234567"}) {
		t.Errorf("parsed %+v", film)
	}
}

func TestFind_TurnsAnIMDbIDIntoTMDBs(t *testing.T) {
	t.Parallel()
	server, seen := fake(t, map[string]string{
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
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	client := newClient(t, server.URL, "k")
	_, err := client.Fetch(t.Context(), "movie", "1")
	var limited *identify.RateLimitedError
	if !errors.As(err, &limited) || limited.After != 7*time.Second {
		t.Errorf("429 = %v, want RateLimitedError after 7s", err)
	}
	if _, err := client.Fetch(t.Context(), "movie", "2"); !errors.Is(err, identify.ErrKeyRefused) {
		t.Errorf("401 = %v, want ErrKeyRefused", err)
	}
	if _, err := client.Fetch(t.Context(), "movie", "3"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("500 = %v, want an error naming the status", err)
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
	if err == nil || strings.Contains(err.Error(), "the-secret-key") {
		t.Errorf("error %v; want a failure that does not show the key", err)
	}
}

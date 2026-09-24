// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/secrets"
)

// BaseURL is TMDB's API, version 3.
const BaseURL = "https://api.themoviedb.org/3"

// maxResponseBytes bounds a response body: a film's details with its credits are tens of
// kilobytes, so anything near this is not TMDB.
const maxResponseBytes = 8 << 20

const kindMovie = "movie"

// Keys opens the TMDB key; secrets.Store is one.
type Keys interface {
	Open(ctx context.Context, name secrets.Name, owner *int64) (secrets.Value, error)
}

// Client is TMDB behind the provider interface.
type Client struct {
	http *http.Client
	base string
	keys Keys
}

var _ identify.Provider = (*Client)(nil)

// New returns a TMDB client calling base (BaseURL, or a test's fake) through httpClient, which
// comes from internal/platform/outbound.
func New(httpClient *http.Client, base string, keys Keys) *Client {
	return &Client{http: httpClient, base: strings.TrimSuffix(base, "/"), keys: keys}
}

// Name is "tmdb".
func (*Client) Name() string { return "tmdb" }

// searchResponse is the part of /search/movie that is read, in TMDB's names.
//
//nolint:tagliatelle // TMDB's field names
type searchResponse struct {
	Results []struct {
		ID            int64  `json:"id"`
		Title         string `json:"title"`
		OriginalTitle string `json:"original_title"`
		ReleaseDate   string `json:"release_date"`
		PosterPath    string `json:"poster_path"`
	} `json:"results"`
}

// Search asks /search/movie for a name and year.
func (client *Client) Search(ctx context.Context, query identify.Query) ([]identify.Candidate, error) {
	if query.Kind != kindMovie {
		return nil, fmt.Errorf("tmdb: searching for a %s is not built", query.Kind)
	}
	parameters := url.Values{"query": {query.Name}, "include_adult": {"false"}}
	if query.Year > 0 {
		parameters.Set("year", strconv.Itoa(query.Year))
	}
	body, err := client.get(ctx, "/search/movie", parameters)
	if err != nil {
		return nil, err
	}
	var response searchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("tmdb: read the search response: %w", err)
	}
	candidates := make([]identify.Candidate, 0, len(response.Results))
	for _, result := range response.Results {
		candidates = append(candidates, identify.Candidate{
			ID: strconv.FormatInt(result.ID, 10), Name: result.Title, OriginalName: result.OriginalTitle,
			Year: yearOf(result.ReleaseDate), PosterRef: result.PosterPath,
		})
	}
	return candidates, nil
}

// Find turns an IMDb id into TMDB's through /find.
func (client *Client) Find(ctx context.Context, kind, source, id string) (string, error) {
	if kind != kindMovie || source != "imdb" {
		return "", fmt.Errorf("tmdb: finding a %s by its %s id is not built", kind, source)
	}
	body, err := client.get(ctx, "/find/"+url.PathEscape(id), url.Values{"external_source": {"imdb_id"}})
	if err != nil {
		return "", err
	}
	//nolint:tagliatelle // TMDB's field names
	var response struct {
		MovieResults []struct {
			ID int64 `json:"id"`
		} `json:"movie_results"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("tmdb: read the find response: %w", err)
	}
	if len(response.MovieResults) == 0 {
		return "", fmt.Errorf("tmdb: %s %s: %w", source, id, identify.ErrNoMatch)
	}
	return strconv.FormatInt(response.MovieResults[0].ID, 10), nil
}

// Fetch returns a film's details with its credits and external ids, the body that is stored.
func (client *Client) Fetch(ctx context.Context, kind, id string) ([]byte, error) {
	if kind != kindMovie {
		return nil, fmt.Errorf("tmdb: fetching a %s is not built", kind)
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return nil, fmt.Errorf("tmdb: %q is not a TMDB id: %w", id, identify.ErrNoMatch)
	}
	return client.get(ctx, "/movie/"+id, url.Values{"append_to_response": {"credits,external_ids"}})
}

// details is the part of /movie/{id} that is read, in TMDB's names.
//
//nolint:tagliatelle // TMDB's field names
type details struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	OriginalTitle    string `json:"original_title"`
	OriginalLanguage string `json:"original_language"`
	Overview         string `json:"overview"`
	Tagline          string `json:"tagline"`
	ReleaseDate      string `json:"release_date"`
	Runtime          int64  `json:"runtime"`
	IMDbID           string `json:"imdb_id"`
}

// Parse reads a stored /movie/{id} response.
func (*Client) Parse(kind string, body []byte) (identify.Metadata, error) {
	if kind != kindMovie {
		return identify.Metadata{}, fmt.Errorf("tmdb: reading a %s is not built", kind)
	}
	var film details
	if err := json.Unmarshal(body, &film); err != nil {
		return identify.Metadata{}, fmt.Errorf("tmdb: read a stored response: %w", err)
	}
	if film.ID == 0 {
		return identify.Metadata{}, errors.New("tmdb: a stored response has no id")
	}
	id := strconv.FormatInt(film.ID, 10)
	ids := map[string]string{"tmdb": id}
	if film.IMDbID != "" {
		ids["imdb"] = film.IMDbID
	}
	return identify.Metadata{
		ID: id, Name: film.Title, OriginalName: film.OriginalTitle, OriginalLang: film.OriginalLanguage,
		Overview: film.Overview, Tagline: film.Tagline, Year: yearOf(film.ReleaseDate),
		RuntimeMs: film.Runtime * int64(time.Minute/time.Millisecond), IDs: ids,
	}, nil
}

// get calls one endpoint with the key and returns the body of a 200.
func (client *Client) get(ctx context.Context, path string, parameters url.Values) ([]byte, error) {
	key, err := client.keys.Open(ctx, secrets.TMDBAPIKey, nil)
	if errors.Is(err, secrets.ErrNotSet) || (err == nil && key.IsZero()) {
		return nil, fmt.Errorf("tmdb: %w", identify.ErrNoKey)
	}
	if err != nil {
		return nil, fmt.Errorf("tmdb: open the key: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.base+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("tmdb %s: %w", path, err)
	}
	authorise(request, parameters, key)
	request.Header.Set("Accept", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		// url.Error repeats the URL, which may carry the key: keep only the cause.
		if urlErr, isURL := errors.AsType[*url.Error](err); isURL {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("tmdb %s: %w", path, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // a read-only body
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("tmdb %s: read the response: %w", path, err)
	}
	return body, statusError(path, response, body)
}

// authorise presents the key: a v4 read-access token as a bearer token, a v3 key as api_key.
func authorise(request *http.Request, parameters url.Values, key secrets.Value) {
	text := key.Reveal()
	if strings.HasPrefix(text, "eyJ") {
		request.Header.Set("Authorization", "Bearer "+text)
	} else {
		parameters = cloneWith(parameters, "api_key", text)
	}
	request.URL.RawQuery = parameters.Encode()
}

func cloneWith(parameters url.Values, name, value string) url.Values {
	clone := url.Values{}
	maps.Copy(clone, parameters)
	clone.Set(name, value)
	return clone
}

// statusError turns a reply that is not 200 into the provider errors the matcher acts on.
func statusError(path string, response *http.Response, body []byte) error {
	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("tmdb %s: %w", path, identify.ErrNoMatch)
	case http.StatusUnauthorized:
		return fmt.Errorf("tmdb %s: %w", path, identify.ErrKeyRefused)
	case http.StatusTooManyRequests:
		seconds, err := strconv.Atoi(strings.TrimSpace(response.Header.Get("Retry-After")))
		if err == nil && seconds >= 0 {
			limited := &identify.RateLimitedError{After: time.Duration(seconds) * time.Second}
			return fmt.Errorf("tmdb %s: %w", path, limited)
		}
	}
	excerpt := string(body[:min(len(body), 200)])
	return fmt.Errorf("tmdb %s: status %d: %s", path, response.StatusCode, excerpt)
}

func yearOf(date string) int {
	year, err := strconv.Atoi(strings.SplitN(date, "-", 2)[0])
	if err != nil || year < 1 {
		return 0
	}
	return year
}

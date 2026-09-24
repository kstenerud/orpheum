// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
)

// endpoint is where one kind of work lives at TMDB, and what its search is keyed on.
type endpoint struct {
	path       string // "movie" or "tv"
	yearFilter string // the search's year parameter
	findField  string // /find's result list
}

func endpointFor(kind string) (endpoint, error) {
	switch kind {
	case identify.KindMovie:
		return endpoint{path: "movie", yearFilter: "year", findField: "movie_results"}, nil
	case identify.KindSeries:
		return endpoint{path: "tv", yearFilter: "first_air_date_year", findField: "tv_results"}, nil
	}
	return endpoint{}, fmt.Errorf("tmdb: a %s is not a kind TMDB is asked about", kind)
}

// result is one search or find result: a film's fields, or a series' under their own names.
//
//nolint:tagliatelle // TMDB's field names
type result struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Name          string `json:"name"`
	OriginalTitle string `json:"original_title"`
	OriginalName  string `json:"original_name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	PosterPath    string `json:"poster_path"`
}

func (found result) candidate() identify.Candidate {
	return identify.Candidate{
		ID: strconv.FormatInt(found.ID, 10), Name: firstOf(found.Title, found.Name),
		OriginalName: firstOf(found.OriginalTitle, found.OriginalName),
		Year:         yearOf(firstOf(found.ReleaseDate, found.FirstAirDate)), PosterRef: found.PosterPath,
	}
}

// Search asks /search/movie or /search/tv for a name and year.
func (client *Client) Search(ctx context.Context, query identify.Query) ([]identify.Candidate, error) {
	where, err := endpointFor(query.Kind)
	if err != nil {
		return nil, err
	}
	parameters := url.Values{"query": {query.Name}, "include_adult": {"false"}}
	if query.Year > 0 {
		parameters.Set(where.yearFilter, strconv.Itoa(query.Year))
	}
	body, err := client.get(ctx, "/search/"+where.path, parameters)
	if err != nil {
		return nil, err
	}
	var response struct {
		Results []result `json:"results"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("tmdb: read the search response: %w", err)
	}
	candidates := make([]identify.Candidate, 0, len(response.Results))
	for _, found := range response.Results {
		candidates = append(candidates, found.candidate())
	}
	return candidates, nil
}

// Find turns an IMDb id, or a TVDB id for a series, into TMDB's through /find.
func (client *Client) Find(ctx context.Context, kind, source, id string) (string, error) {
	where, err := endpointFor(kind)
	if err != nil {
		return "", err
	}
	external := map[string]string{"imdb": "imdb_id", "tvdb": "tvdb_id"}[source]
	if external == "" {
		return "", fmt.Errorf("tmdb: finding by a %s id is not built", source)
	}
	body, err := client.get(ctx, "/find/"+url.PathEscape(id), url.Values{"external_source": {external}})
	if err != nil {
		return "", err
	}
	var response map[string][]result
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("tmdb: read the find response: %w", err)
	}
	if len(response[where.findField]) == 0 {
		return "", fmt.Errorf("tmdb: %s %s: %w", source, id, identify.ErrNoMatch)
	}
	return strconv.FormatInt(response[where.findField][0].ID, 10), nil
}

// Fetch returns a film's details with its credits and external ids, or a series' own, the body
// that is stored.
func (client *Client) Fetch(ctx context.Context, kind, id string) ([]byte, error) {
	where, err := endpointFor(kind)
	if err != nil {
		return nil, err
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return nil, fmt.Errorf("tmdb: %q is not a TMDB id: %w", id, identify.ErrNoMatch)
	}
	return client.get(ctx, "/"+where.path+"/"+id, url.Values{"append_to_response": {"credits,external_ids"}})
}

// details is the part of /movie/{id} and /tv/{id} that is read, in TMDB's names.
//
//nolint:tagliatelle // TMDB's field names
type details struct {
	result
	OriginalLanguage string  `json:"original_language"`
	Overview         string  `json:"overview"`
	Tagline          string  `json:"tagline"`
	Runtime          int64   `json:"runtime"`
	EpisodeRunTime   []int64 `json:"episode_run_time"`
	IMDbID           string  `json:"imdb_id"`
	ExternalIDs      struct {
		IMDbID string `json:"imdb_id"`
		TVDbID int64  `json:"tvdb_id"`
	} `json:"external_ids"`
}

// Parse reads a stored /movie/{id} or /tv/{id} response.
func (*Client) Parse(kind string, body []byte) (identify.Metadata, error) {
	if _, err := endpointFor(kind); err != nil {
		return identify.Metadata{}, err
	}
	var work details
	if err := json.Unmarshal(body, &work); err != nil {
		return identify.Metadata{}, fmt.Errorf("tmdb: read a stored response: %w", err)
	}
	if work.ID == 0 {
		return identify.Metadata{}, errors.New("tmdb: a stored response has no id")
	}
	summary := work.candidate()
	ids := map[string]string{"tmdb": summary.ID}
	if imdb := firstOf(work.IMDbID, work.ExternalIDs.IMDbID); imdb != "" {
		ids["imdb"] = imdb
	}
	if work.ExternalIDs.TVDbID > 0 {
		ids["tvdb"] = strconv.FormatInt(work.ExternalIDs.TVDbID, 10)
	}
	runtime := work.Runtime
	if kind == identify.KindSeries {
		runtime = 0 // a series has no runtime; its episodes each have theirs
	}
	return identify.Metadata{
		ID: summary.ID, Name: summary.Name, OriginalName: summary.OriginalName, OriginalLang: work.OriginalLanguage,
		Overview: work.Overview, Tagline: work.Tagline, Year: summary.Year,
		RuntimeMs: runtime * int64(time.Minute/time.Millisecond), IDs: ids,
	}, nil
}

// FetchSeason returns /tv/{id}/season/{n}: every episode of one season, one call (D209).
func (client *Client) FetchSeason(ctx context.Context, seriesID string, season int) ([]byte, error) {
	if _, err := strconv.ParseInt(seriesID, 10, 64); err != nil {
		return nil, fmt.Errorf("tmdb: %q is not a TMDB id: %w", seriesID, identify.ErrNoMatch)
	}
	return client.get(ctx, "/tv/"+seriesID+"/season/"+strconv.Itoa(season), url.Values{})
}

// ParseSeason reads a stored season listing.
func (*Client) ParseSeason(body []byte) ([]identify.Episode, error) {
	//nolint:tagliatelle // TMDB's field names
	var listing struct {
		Episodes []struct {
			ID            int64  `json:"id"`
			SeasonNumber  int    `json:"season_number"`
			EpisodeNumber int    `json:"episode_number"`
			Name          string `json:"name"`
			Overview      string `json:"overview"`
			AirDate       string `json:"air_date"`
			Runtime       int64  `json:"runtime"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("tmdb: read a stored season: %w", err)
	}
	episodes := make([]identify.Episode, 0, len(listing.Episodes))
	for _, episode := range listing.Episodes {
		episodes = append(episodes, identify.Episode{
			ID: strconv.FormatInt(episode.ID, 10), Season: episode.SeasonNumber, Number: episode.EpisodeNumber,
			Name: episode.Name, Overview: episode.Overview, Year: yearOf(episode.AirDate),
			RuntimeMs: episode.Runtime * int64(time.Minute/time.Millisecond),
		})
	}
	return episodes, nil
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

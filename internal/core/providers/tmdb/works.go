// SPDX-License-Identifier: AGPL-3.0-or-later

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
	"github.com/kstenerud/orpheum/internal/core/library"
)

// endpoint is where one kind of work lives at TMDB, and what its search is keyed on.
type endpoint struct {
	path           string // "movie" or "tv"
	yearFilter     string // the search's year parameter
	findResultsKey string // /find's result list
}

func endpointFor(kind string) (endpoint, error) {
	switch kind {
	case identify.KindMovie:
		return endpoint{path: "movie", yearFilter: "year", findResultsKey: "movie_results"}, nil
	case identify.KindSeries:
		return endpoint{path: "tv", yearFilter: "first_air_date_year", findResultsKey: "tv_results"}, nil
	}
	return endpoint{}, fmt.Errorf("tmdb: a %s is not a kind TMDB is asked about", kind)
}

// workSummary is one search or find result: a film's fields, or a series' under their own names.
//
//nolint:tagliatelle // TMDB's field names
type workSummary struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Name          string `json:"name"`
	OriginalTitle string `json:"original_title"`
	OriginalName  string `json:"original_name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	PosterPath    string `json:"poster_path"`
	VoteCount     int    `json:"vote_count"`
}

func (found workSummary) candidate() identify.Candidate {
	return identify.Candidate{
		ID: strconv.FormatInt(found.ID, 10), Name: firstNonEmpty(found.Title, found.Name),
		OriginalName:       firstNonEmpty(found.OriginalTitle, found.OriginalName),
		Year:               identify.YearOfDate(firstNonEmpty(found.ReleaseDate, found.FirstAirDate)),
		ProviderPosterPath: found.PosterPath,
		Votes:              found.VoteCount,
	}
}

// Search asks /search/movie or /search/tv for a name and year.
func (client *Client) Search(ctx context.Context, query identify.Query) ([]identify.Candidate, error) {
	kindEndpoint, err := endpointFor(query.Kind)
	if err != nil {
		return nil, err
	}
	parameters := url.Values{"query": {query.Name}, "include_adult": {"false"}}
	if query.Year > 0 {
		parameters.Set(kindEndpoint.yearFilter, strconv.Itoa(query.Year))
	}
	body, err := client.get(ctx, "/search/"+kindEndpoint.path, parameters)
	if err != nil {
		return nil, err
	}
	var response struct {
		Results []workSummary `json:"results"`
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
	kindEndpoint, err := endpointFor(kind)
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
	var response map[string][]workSummary
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("tmdb: read the find response: %w", err)
	}
	if len(response[kindEndpoint.findResultsKey]) == 0 {
		return "", fmt.Errorf("tmdb: %s %s: %w", source, id, identify.ErrNoMatch)
	}
	return strconv.FormatInt(response[kindEndpoint.findResultsKey][0].ID, 10), nil
}

// Fetch returns a film's details with its credits and external ids, or a series' own, the body
// that is stored.
func (client *Client) Fetch(ctx context.Context, kind, id string) ([]byte, error) {
	kindEndpoint, err := endpointFor(kind)
	if err != nil {
		return nil, err
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return nil, fmt.Errorf("tmdb: %q is not a TMDB id: %w", id, identify.ErrNoMatch)
	}
	appended := "credits,external_ids"
	if kind == identify.KindSeries {
		// A series' list of episode groups comes with it, at no request of its own (episode-orderings.md
		// §2.1, as Jellyfin asks for it).
		appended += ",episode_groups"
	}
	return client.get(ctx, "/"+kindEndpoint.path+"/"+id, url.Values{"append_to_response": {appended}})
}

// workDetails is the part of /movie/{id} and /tv/{id} that is read, in TMDB's names.
//
//nolint:tagliatelle // TMDB's field names
type workDetails struct {
	workSummary
	OriginalLanguage string  `json:"original_language"`
	Overview         string  `json:"overview"`
	Tagline          string  `json:"tagline"`
	RuntimeMinutes   int64   `json:"runtime"`
	EpisodeRunTime   []int64 `json:"episode_run_time"`
	BackdropPath     string  `json:"backdrop_path"`
	IMDbID           string  `json:"imdb_id"`
	ExternalIDs      struct {
		IMDbID string `json:"imdb_id"`
		TVDbID int64  `json:"tvdb_id"`
	} `json:"external_ids"`
	// Seasons are a series' seasons, the specials' 0 among them.
	Seasons []struct {
		SeasonNumber int `json:"season_number"`
	} `json:"seasons"`
}

// Parse reads a stored /movie/{id} or /tv/{id} response.
func (*Client) Parse(kind string, body []byte) (identify.Metadata, error) {
	if _, err := endpointFor(kind); err != nil {
		return identify.Metadata{}, err
	}
	var work workDetails
	if err := json.Unmarshal(body, &work); err != nil {
		return identify.Metadata{}, fmt.Errorf("tmdb: read a stored response: %w", err)
	}
	if work.ID == 0 {
		return identify.Metadata{}, errors.New("tmdb: a stored response has no id")
	}
	summary := work.candidate()
	ids := map[string]string{"tmdb": summary.ID}
	if imdb := firstNonEmpty(work.IMDbID, work.ExternalIDs.IMDbID); imdb != "" {
		ids["imdb"] = imdb
	}
	if work.ExternalIDs.TVDbID > 0 {
		ids["tvdb"] = strconv.FormatInt(work.ExternalIDs.TVDbID, 10)
	}
	runtimeMinutes := work.RuntimeMinutes
	if kind == identify.KindSeries {
		runtimeMinutes = 0 // a series has no runtime; its episodes each have theirs
	}
	var artwork []identify.ImageRef
	if work.PosterPath != "" {
		artwork = append(artwork, identify.ImageRef{Kind: library.ArtPoster, Ref: work.PosterPath})
	}
	if work.BackdropPath != "" {
		artwork = append(artwork, identify.ImageRef{Kind: library.ArtBackdrop, Ref: work.BackdropPath})
	}
	var seasons []int
	for _, season := range work.Seasons {
		seasons = append(seasons, season.SeasonNumber)
	}
	return identify.Metadata{
		ID: summary.ID, Name: summary.Name, OriginalName: summary.OriginalName, OriginalLang: work.OriginalLanguage,
		Overview: work.Overview, Tagline: work.Tagline, Year: summary.Year,
		ReleaseDate: firstNonEmpty(work.ReleaseDate, work.FirstAirDate),
		RuntimeMs:   runtimeMinutes * int64(time.Minute/time.Millisecond), IDs: ids, Artwork: artwork,
		SeasonNumbers: seasons,
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
			ID             int64  `json:"id"`
			SeasonNumber   int    `json:"season_number"`
			EpisodeNumber  int    `json:"episode_number"`
			Name           string `json:"name"`
			Overview       string `json:"overview"`
			AirDate        string `json:"air_date"`
			RuntimeMinutes int64  `json:"runtime"`
			StillPath      string `json:"still_path"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("tmdb: read a stored season: %w", err)
	}
	episodes := make([]identify.Episode, 0, len(listing.Episodes))
	for _, episode := range listing.Episodes {
		episodes = append(episodes, identify.Episode{
			ID: strconv.FormatInt(episode.ID, 10), Season: episode.SeasonNumber, Number: episode.EpisodeNumber,
			Name: episode.Name, Overview: episode.Overview, Year: identify.YearOfDate(episode.AirDate),
			RuntimeMs: episode.RuntimeMinutes * int64(time.Minute/time.Millisecond), StillRef: episode.StillPath,
		})
	}
	return episodes, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// groupKinds are TMDB's episode group types, by number, as the kinds an ordering says it is
// (episode-orderings.md §2.1: "Groups support 7 different types").
var groupKinds = map[int]string{ //nolint:gochecknoglobals // a fixed table of TMDB's
	1: "aired_alternate", 2: "absolute", 3: "dvd", 4: "digital", 5: "story_arc", 6: "production", 7: "tv",
}

// EpisodeGroupsIn is the episode_groups a stored /tv/{id} record carries, the same shape as
// /tv/{id}/episode_groups answers; false for a record fetched without it.
func (*Client) EpisodeGroupsIn(seriesBody []byte) ([]byte, bool) {
	//nolint:tagliatelle // TMDB's field names
	var record struct {
		EpisodeGroups json.RawMessage `json:"episode_groups"`
	}
	if err := json.Unmarshal(seriesBody, &record); err != nil || len(record.EpisodeGroups) == 0 ||
		string(record.EpisodeGroups) == "null" {
		return nil, false
	}
	return record.EpisodeGroups, true
}

// FetchEpisodeGroups returns /tv/{id}/episode_groups: the list of a series' episode groups, for a
// series whose record was fetched before it carried them.
func (client *Client) FetchEpisodeGroups(ctx context.Context, seriesID string) ([]byte, error) {
	if _, err := strconv.ParseInt(seriesID, 10, 64); err != nil {
		return nil, fmt.Errorf("tmdb: %q is not a TMDB id: %w", seriesID, identify.ErrNoMatch)
	}
	return client.get(ctx, "/tv/"+seriesID+"/episode_groups", url.Values{})
}

// ParseEpisodeGroups reads a stored list of a series' episode groups.
func (*Client) ParseEpisodeGroups(body []byte) ([]identify.EpisodeGroup, error) {
	//nolint:tagliatelle // TMDB's field names
	var list struct {
		Results []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Type         int    `json:"type"`
			EpisodeCount int    `json:"episode_count"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("tmdb: read a stored list of episode groups: %w", err)
	}
	groups := make([]identify.EpisodeGroup, 0, len(list.Results))
	for _, group := range list.Results {
		if group.ID == "" {
			continue
		}
		groups = append(groups, identify.EpisodeGroup{
			ID: group.ID, Name: group.Name, Kind: groupKinds[group.Type], EpisodeCount: group.EpisodeCount,
		})
	}
	return groups, nil
}

// FetchEpisodeGroup returns /tv/episode_group/{id}: one group's parts and their episodes.
func (client *Client) FetchEpisodeGroup(ctx context.Context, groupID string) ([]byte, error) {
	if groupID == "" || url.PathEscape(groupID) != groupID {
		return nil, fmt.Errorf("tmdb: %q is not an episode group id: %w", groupID, identify.ErrNoMatch)
	}
	return client.get(ctx, "/tv/episode_group/"+groupID, url.Values{})
}

// ParseEpisodeGroup reads a stored group's parts, each with its episodes by their TMDB ids.
func (*Client) ParseEpisodeGroup(body []byte) ([]identify.EpisodeGroupPart, error) {
	var group struct {
		Groups []struct {
			Name     string `json:"name"`
			Order    int    `json:"order"`
			Episodes []struct {
				ID    int64 `json:"id"`
				Order int   `json:"order"`
			} `json:"episodes"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("tmdb: read a stored episode group: %w", err)
	}
	parts := make([]identify.EpisodeGroupPart, 0, len(group.Groups))
	for _, part := range group.Groups {
		entries := make([]identify.EpisodeGroupEntry, 0, len(part.Episodes))
		for _, episode := range part.Episodes {
			entries = append(entries, identify.EpisodeGroupEntry{
				EpisodeID: strconv.FormatInt(episode.ID, 10), Order: episode.Order,
			})
		}
		parts = append(parts, identify.EpisodeGroupPart{Order: part.Order, Name: part.Name, Episodes: entries})
	}
	return parts, nil
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package identifytest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// fakeProviderBodies reads the fake's stored bodies as the real providers' answers are read (D458).
const fakeProviderBodies = strictjson.LastWins

// Provider is TMDB behind identify.Provider (standards/testing.md, Network). A test fills its
// maps before the code under test calls it, and reads Calls once no call is in flight; a test
// whose server calls it from another goroutine changes it only through SetFailure.
type Provider struct {
	// Films are the works Fetch returns, by id; a stored body is the Metadata as JSON.
	Films map[string]identify.Metadata
	// Searches are Search's results, by SearchKey.
	Searches map[string][]identify.Candidate
	// TMDBIDsByOtherID is Find's table: this provider's id by another source's id, whichever
	// source names it.
	TMDBIDsByOtherID map[string]string
	// Seasons are FetchSeason's listings, by "<series id>/<season>".
	Seasons map[string][]identify.Episode
	// EpisodeGroupLists are each series' list of episode groups, by its id: its record, Fetch's, carries
	// it, and FetchEpisodeGroups answers it alone, as TMDB's does.
	EpisodeGroupLists map[string][]identify.EpisodeGroup
	// RecordsWithoutGroups are the series whose record Fetch returns without their list, as one
	// fetched before it was asked for.
	RecordsWithoutGroups map[string]bool
	// EpisodeGroups are each group's parts, FetchEpisodeGroup's, by the group's id.
	EpisodeGroups map[string][]identify.EpisodeGroupPart
	// Calls are the calls made, in order: "search <SearchKey>", "find <source> <id>",
	// "fetch <id>", "season <series id>/<season>", "episode_groups <series id>" and
	// "episode_group <group id>".
	Calls []string
	// Importances are how much each call said it mattered, in the order of Calls
	// (providers.ImportanceOf).
	Importances []providers.Importance

	mutex   sync.Mutex
	failure error // guarded by mutex
}

var _ identify.Provider = (*Provider)(nil)

// NewProvider is a Provider that knows nothing yet.
func NewProvider() *Provider {
	return &Provider{
		Films: map[string]identify.Metadata{}, Searches: map[string][]identify.Candidate{},
		TMDBIDsByOtherID: map[string]string{}, Seasons: map[string][]identify.Episode{},
		EpisodeGroupLists: map[string][]identify.EpisodeGroup{}, RecordsWithoutGroups: map[string]bool{},
		EpisodeGroups: map[string][]identify.EpisodeGroupPart{},
	}
}

// SearchKey is the key of Searches for a search by name and year (0 for none).
func SearchKey(name string, year int) string { return fmt.Sprintf("%s|%d", name, year) }

// Add makes film one Fetch returns, and is the candidate a search would list for it.
func (fake *Provider) Add(film identify.Metadata) identify.Candidate {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Films[film.ID] = film
	return identify.Candidate{ID: film.ID, Name: film.Name, OriginalName: film.OriginalName, Year: film.Year}
}

// SetFailure makes every later Search, Find, Fetch and FetchSeason fail with failure, as a
// provider that refuses or limits us does; nil lets them answer again.
func (fake *Provider) SetFailure(failure error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.failure = failure
}

// note keeps a call and how much it mattered. The caller holds the mutex.
func (fake *Provider) note(ctx context.Context, call string) {
	fake.Calls = append(fake.Calls, call)
	fake.Importances = append(fake.Importances, providers.ImportanceOf(ctx))
}

// Name is "tmdb".
func (*Provider) Name() string { return "tmdb" }

// Search returns Searches' results for the query's name and year.
func (fake *Provider) Search(ctx context.Context, query identify.Query) ([]identify.Candidate, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	key := SearchKey(query.Name, query.Year)
	fake.note(ctx, "search "+key)
	if fake.failure != nil {
		return nil, fake.failure
	}
	return fake.Searches[key], nil
}

// Find looks id up in TMDBIDsByOtherID, whatever its source.
func (fake *Provider) Find(ctx context.Context, _, source, id string) (string, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.note(ctx, "find "+source+" "+id)
	if fake.failure != nil {
		return "", fake.failure
	}
	if found, ok := fake.TMDBIDsByOtherID[id]; ok {
		return found, nil
	}
	return "", identify.ErrNoMatch
}

// Fetch returns the film with id as JSON; a series' carries its list of episode groups, empty when
// EpisodeGroupLists has none, as TMDB's record carries what it appends, unless RecordsWithoutGroups
// names it.
func (fake *Provider) Fetch(ctx context.Context, kind, id string) ([]byte, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.note(ctx, "fetch "+id)
	if fake.failure != nil {
		return nil, fake.failure
	}
	film, found := fake.Films[id]
	if !found {
		return nil, identify.ErrNoMatch
	}
	groups := fake.EpisodeGroupLists[id]
	if kind != identify.KindSeries || fake.RecordsWithoutGroups[id] {
		return json.Marshal(film) //nolint:wrapcheck,musttag // a test fake stores Metadata as Go names it
	}
	// The record carries its list, as TMDB's appends it: beside the Metadata, which Parse reads.
	return json.Marshal(struct { //nolint:wrapcheck,musttag // as above
		identify.Metadata
		EpisodeGroups []identify.EpisodeGroup `json:"episodeGroups"`
	}{film, append([]identify.EpisodeGroup{}, groups...)})
}

// EpisodeGroupsIn is the list a record Fetch returned carries.
func (*Provider) EpisodeGroupsIn(seriesBody []byte) ([]byte, bool) {
	var record struct {
		EpisodeGroups json.RawMessage `json:"episodeGroups"`
	}
	if err := fakeProviderBodies.Unmarshal(seriesBody, &record); err != nil || len(record.EpisodeGroups) == 0 {
		return nil, false
	}
	return record.EpisodeGroups, true
}

// FetchEpisodeGroups returns a series' list from EpisodeGroupLists as JSON; none listed is an empty
// list.
func (fake *Provider) FetchEpisodeGroups(ctx context.Context, seriesID string) ([]byte, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.note(ctx, "episode_groups "+seriesID)
	if fake.failure != nil {
		return nil, fake.failure
	}
	if _, found := fake.Films[seriesID]; !found {
		return nil, identify.ErrNoMatch
	}
	return json.Marshal(fake.EpisodeGroupLists[seriesID]) //nolint:wrapcheck,musttag // as Fetch's
}

// ParseEpisodeGroups reads back what FetchEpisodeGroups returned.
func (*Provider) ParseEpisodeGroups(body []byte) ([]identify.EpisodeGroup, error) {
	var groups []identify.EpisodeGroup
	err := fakeProviderBodies.Unmarshal(body, &groups)
	return groups, err //nolint:wrapcheck // a test fake
}

// FetchEpisodeGroup returns a group's parts from EpisodeGroups as JSON.
func (fake *Provider) FetchEpisodeGroup(ctx context.Context, groupID string) ([]byte, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.note(ctx, "episode_group "+groupID)
	if fake.failure != nil {
		return nil, fake.failure
	}
	parts, found := fake.EpisodeGroups[groupID]
	if !found {
		return nil, identify.ErrNoMatch
	}
	return json.Marshal(parts) //nolint:wrapcheck,musttag // as Fetch's
}

// ParseEpisodeGroup reads back what FetchEpisodeGroup returned.
func (*Provider) ParseEpisodeGroup(body []byte) ([]identify.EpisodeGroupPart, error) {
	var parts []identify.EpisodeGroupPart
	err := fakeProviderBodies.Unmarshal(body, &parts)
	return parts, err //nolint:wrapcheck // a test fake
}

// Parse reads back what Fetch returned.
func (*Provider) Parse(_ string, body []byte) (identify.Metadata, error) {
	var film identify.Metadata
	err := fakeProviderBodies.Unmarshal(body, &film)
	return film, err //nolint:wrapcheck // a test fake
}

// FetchSeason returns the season's listing from Seasons as JSON.
func (fake *Provider) FetchSeason(ctx context.Context, seriesID string, season int) ([]byte, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	key := fmt.Sprintf("%s/%d", seriesID, season)
	fake.note(ctx, "season "+key)
	if fake.failure != nil {
		return nil, fake.failure
	}
	episodes, found := fake.Seasons[key]
	if !found {
		return nil, identify.ErrNoMatch
	}
	return json.Marshal(episodes) //nolint:wrapcheck,musttag // a test fake stores Episodes as Go names them
}

// ParseSeason reads back what FetchSeason returned.
func (*Provider) ParseSeason(body []byte) ([]identify.Episode, error) {
	var episodes []identify.Episode
	err := fakeProviderBodies.Unmarshal(body, &episodes)
	return episodes, err //nolint:wrapcheck // a test fake
}

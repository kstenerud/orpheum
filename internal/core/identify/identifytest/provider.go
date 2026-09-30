// SPDX-License-Identifier: GPL-3.0-or-later

package identifytest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/providers"
)

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
	// Calls are the calls made, in order: "search <SearchKey>", "find <source> <id>",
	// "fetch <id>" and "season <series id>/<season>".
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

// Fetch returns the film with id as JSON.
func (fake *Provider) Fetch(ctx context.Context, _, id string) ([]byte, error) {
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
	return json.Marshal(film) //nolint:wrapcheck,musttag // a test fake stores Metadata as Go names it
}

// Parse reads back what Fetch returned.
func (*Provider) Parse(_ string, body []byte) (identify.Metadata, error) {
	var film identify.Metadata
	err := json.Unmarshal(body, &film) //nolint:musttag // as the fake stored it
	return film, err                   //nolint:wrapcheck // a test fake
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
	err := json.Unmarshal(body, &episodes) //nolint:musttag // as the fake stored it
	return episodes, err                   //nolint:wrapcheck // a test fake
}

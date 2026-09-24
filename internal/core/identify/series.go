// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// seriesSpec is what a series' files say about it: its folders, and its tvshow.nfo.
type seriesSpec struct {
	name     string
	year     int
	overview string
	ids      map[string]string
	locked   []string
	hasNFO   bool
}

// fromNFO takes a tvshow.nfo as the series' own source (D183).
func (spec *seriesSpec) fromNFO(nfo NFO) {
	if nfo.Title != "" {
		spec.name = nfo.Title
	}
	if nfo.Year > 0 {
		spec.year = nfo.Year
	}
	spec.overview, spec.locked, spec.hasNFO = nfo.Plot, nfo.Locked, true
	if spec.ids == nil {
		spec.ids = map[string]string{}
	}
	maps.Copy(spec.ids, nfo.ProviderIDs)
}

// seriesMatched reports whether a series is matched at its provider, by the matcher or a person.
func seriesMatched(row sqlcgen.GetSeriesForMatchRow) bool {
	ident := derefText(row.Ident)
	return row.SourceID != nil && (ident == identIdentified || ident == identConfirmed)
}

// seasonPart is where a season's listing is stored in provider_response.
func seasonPart(season int) string { return "season/" + strconv.Itoa(season) }

// seriesMatch is a series' match state, read once per pass.
func (work *run) seriesMatch(ctx context.Context, seriesID int64) (sqlcgen.GetSeriesForMatchRow, error) {
	if row, found := work.seriesRows[seriesID]; found {
		return row, nil
	}
	row, err := work.queries.GetSeriesForMatch(ctx, seriesID)
	if err != nil {
		return row, fmt.Errorf("read series %d's match: %w", seriesID, err)
	}
	work.seriesRows[seriesID] = row
	return row, nil
}

// listing is a series' stored listing of one season, parsed once per pass, and whether it was
// fetched at all.
func (work *run) listing(ctx context.Context, seriesID int64, provider Provider, season int) ([]Episode, bool, error) {
	key := fmt.Sprintf("%d/%d", seriesID, season)
	if episodes, found := work.listings[key]; found {
		return episodes, episodes != nil, nil
	}
	episodes, err := storedListing(ctx, work.queries, provider, seriesID, season)
	if err != nil {
		return nil, false, err
	}
	work.listings[key] = episodes
	return episodes, episodes != nil, nil
}

// storedListing reads and parses a season listing; nil when it was never fetched.
func storedListing(
	ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64, season int,
) ([]Episode, error) {
	stored, err := store.One(queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		ItemID: seriesID, Provider: provider.Name(), Part: seasonPart(season),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read series %d's season %d: %w", seriesID, season, err)
	}
	body, err := decompress(stored)
	if err != nil {
		return nil, fmt.Errorf("series %d's season %d: %w", seriesID, season, err)
	}
	if len(body) == 0 {
		return []Episode{}, nil // the provider has no such season
	}
	episodes, err := provider.ParseSeason(body)
	if err != nil {
		return nil, fmt.Errorf("series %d's season %d: %w", seriesID, season, err)
	}
	if episodes == nil {
		episodes = []Episode{}
	}
	return episodes, nil
}

// seasonsToFetch are the seasons of a series whose listing is missing, or was fetched before an
// episode of it arrived that it does not list: a new episode is a first fetch (D170, D209 §3).
func seasonsToFetch(ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64) ([]int, error) {
	episodes, err := queries.ListSeriesEpisodes(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list series %d's episodes: %w", seriesID, err)
	}
	var seasons []int
	listings := map[int][]Episode{}
	for _, episode := range episodes {
		if episode.Source != nil && *episode.Source == sourceNFO {
			continue // its own NFO speaks for it
		}
		season, number := int(deref(episode.Grp)), int(deref(episode.Number))
		if slices.Contains(seasons, season) {
			continue
		}
		listing, read := listings[season]
		if !read {
			if listing, err = storedListing(ctx, queries, provider, seriesID, season); err != nil {
				return nil, err
			}
			listings[season] = listing
		}
		where := seasonOf{provider: provider, seriesID: seriesID, season: season}
		stale, err := staleFor(ctx, queries, where, listing, number, episode.CreatedAt)
		if err != nil {
			return nil, err
		}
		if stale {
			seasons = append(seasons, season)
		}
	}
	slices.Sort(seasons)
	return seasons, nil
}

// seasonOf names one season of a series at a provider.
type seasonOf struct {
	provider Provider
	seriesID int64
	season   int
}

// staleFor reports whether a season's listing must be fetched for an episode: it was never
// fetched, or it does not list the episode and was fetched before the episode arrived.
func staleFor(
	ctx context.Context, queries *sqlcgen.Queries, where seasonOf, listing []Episode, number int, arrived int64,
) (bool, error) {
	provider, seriesID, season := where.provider, where.seriesID, where.season
	if listing == nil {
		return true, nil
	}
	if slices.ContainsFunc(listing, func(episode Episode) bool { return episode.Number == number }) {
		return false, nil
	}
	fetchedAt, err := queries.GetProviderResponseFetchedAt(ctx, sqlcgen.GetProviderResponseFetchedAtParams{
		ItemID: seriesID, Provider: provider.Name(), Part: seasonPart(season),
	})
	if err != nil {
		return false, fmt.Errorf("read when series %d's season %d was fetched: %w", seriesID, season, err)
	}
	return arrived > fetchedAt, nil
}

// resolveSeries writes a series' own values — its tvshow.nfo, else its stored match, else its
// folders — with a person's overrides over them, and its match state where the library reads
// NFO files only. It reports what the fetch queue owes the series.
func (work *run) resolveSeries(ctx context.Context, seriesID int64, spec seriesSpec) error {
	current, err := work.seriesMatch(ctx, seriesID)
	if err != nil {
		return err
	}
	provider, known := work.providers[providerOf(work.library.MetadataSource)]
	values, source, err := work.seriesValues(ctx, seriesID, spec, current)
	if err != nil {
		return err
	}
	if recordErr := work.recordSeriesIDs(ctx, seriesID, spec, values); recordErr != nil {
		return recordErr
	}
	overrides, err := work.queries.ListOverrides(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("read series %d's overrides: %w", seriesID, err)
	}
	for _, override := range overrides {
		values[override.Field] = override.Value
	}
	ident := identUnidentified
	if work.library.MetadataSource == sourceNFO && spec.hasNFO {
		ident = identIdentified
	}
	year, _ := strconv.ParseInt(values[fieldYear], 10, 64) //nolint:errcheck // zero when unset
	err = work.queries.UpdateSeriesResolved(ctx, sqlcgen.UpdateSeriesResolvedParams{
		Year: optionalInt(year), Source: optional(source), Ident: &ident, ID: seriesID,
	})
	if err != nil {
		return fmt.Errorf("resolve series %d: %w", seriesID, err)
	}
	err = work.queries.UpsertOriginalText(ctx, sqlcgen.UpsertOriginalTextParams{
		ItemID: seriesID, Lang: langUnknown, Name: values[fieldName], Overview: optional(values[fieldOverview]),
	})
	if err != nil {
		return fmt.Errorf("name series %d: %w", seriesID, err)
	}
	if known && seriesMatched(current) {
		seasons, err := seasonsToFetch(ctx, work.queries, provider, seriesID)
		if err != nil {
			return err
		}
		if len(seasons) > 0 {
			work.report.AwaitingListings = append(work.report.AwaitingListings, seriesID)
		}
	}
	return nil
}

// seriesValues are a series' own values and their source: a person's chosen match over all;
// then its tvshow.nfo; then its stored match; then its folders.
func (work *run) seriesValues(
	ctx context.Context, seriesID int64, spec seriesSpec, current sqlcgen.GetSeriesForMatchRow,
) (map[string]string, string, error) {
	values := map[string]string{fieldName: spec.name, fieldYear: strconv.Itoa(spec.year), fieldOverview: spec.overview}
	source := ""
	switch {
	case work.library.MetadataSource == sourceNFO && !spec.hasNFO:
		source = sourceNone
	case spec.hasNFO:
		source = sourceNFO
	}
	provider, known := work.providers[providerOf(work.library.MetadataSource)]
	if !known || !seriesMatched(current) || (spec.hasNFO && derefText(current.Ident) != identConfirmed) {
		if source == "" && current.Source != nil && *current.Source == sourceNone {
			source = sourceNone // its provider found nothing
		}
		return values, source, nil
	}
	stored, err := work.queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		ItemID: seriesID, Provider: provider.Name(), Part: "",
	})
	if err != nil {
		return nil, "", fmt.Errorf("read series %d's %s response: %w", seriesID, provider.Name(), err)
	}
	body, err := decompress(stored)
	if err != nil {
		return nil, "", fmt.Errorf("series %d's %s response: %w", seriesID, provider.Name(), err)
	}
	metadata, err := provider.Parse(KindSeries, body)
	if err != nil {
		return nil, "", fmt.Errorf("series %d's %s response: %w", seriesID, provider.Name(), err)
	}
	values[fieldName], values[fieldYear], values[fieldOverview] = metadata.Name, strconv.Itoa(metadata.Year),
		metadata.Overview
	return values, provider.Name(), nil
}

// recordSeriesIDs keeps the ids a series' folders and tvshow.nfo name, and its tvshow.nfo's
// locked fields as overrides (D182).
func (work *run) recordSeriesIDs(ctx context.Context, seriesID int64, spec seriesSpec, values map[string]string) error {
	for provider, id := range spec.ids {
		err := work.queries.InsertItemProviderID(ctx, sqlcgen.InsertItemProviderIDParams{
			ItemID: seriesID, Provider: provider, ProviderID: id,
		})
		if err != nil {
			return fmt.Errorf("record series %d's %s %s: %w", seriesID, provider, id, err)
		}
	}
	for _, field := range spec.locked {
		value, overridable := values[field]
		if !overridable {
			continue
		}
		err := work.queries.InsertOverrideIfAbsent(ctx, sqlcgen.InsertOverrideIfAbsentParams{
			ItemID: seriesID, Field: field, Value: value, Origin: "nfo", SetAt: work.now,
		})
		if err != nil {
			return fmt.Errorf("record series %d's locked %s: %w", seriesID, field, err)
		}
	}
	return nil
}

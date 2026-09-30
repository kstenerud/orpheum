// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// seriesSpec is what a series' files say about it: its folders, and its tvshow.nfo.
type seriesSpec struct {
	name         string
	year         int
	overview     string
	ids          map[string]string
	lockedFields []string
	hasNFO       bool
}

// fromNFO takes a tvshow.nfo as the series' own source (D183).
func (spec *seriesSpec) fromNFO(nfo NFO) {
	if nfo.Title != "" {
		spec.name = nfo.Title
	}
	if nfo.Year > 0 {
		spec.year = nfo.Year
	}
	spec.overview, spec.lockedFields, spec.hasNFO = nfo.Plot, nfo.LockedFields, true
	if spec.ids == nil {
		spec.ids = map[string]string{}
	}
	maps.Copy(spec.ids, nfo.ProviderIDs)
}

// isSeriesMatched reports whether a series is matched at its provider, by the matcher or a person.
func isSeriesMatched(row sqlcgen.GetSeriesForMatchRow) bool {
	identification := ptr.ValueOrZero(row.IdentificationState)
	return row.FollowedSourceID != nil &&
		(identification == identificationIdentified || identification == identificationConfirmed)
}

// seasonPart is where a season's listing is stored in provider_response.
func seasonPart(season int) string { return "season/" + strconv.Itoa(season) }

// seasonOfPart is the season a stored part names: seasonPart read back.
func seasonOfPart(part string) (int, error) {
	season, err := strconv.Atoi(strings.TrimPrefix(part, "season/"))
	if err != nil {
		return 0, fmt.Errorf("stored part %q: %w", part, err)
	}
	return season, nil
}

// cachedSeriesMatch is a series' match state, read once per pass.
func (filer *libraryFiler) cachedSeriesMatch(
	ctx context.Context, seriesID int64) (sqlcgen.GetSeriesForMatchRow, error,
) {
	if row, found := filer.seriesMatchByID[seriesID]; found {
		return row, nil
	}
	row, err := filer.queries.GetSeriesForMatch(ctx, seriesID)
	if err != nil {
		return row, fmt.Errorf("read series %d's match: %w", seriesID, err)
	}
	filer.seriesMatchByID[seriesID] = row
	return row, nil
}

// seasonListing is a series' stored listing of one season, parsed once per pass, and whether it was
// fetched at all.
func (filer *libraryFiler) seasonListing(
	ctx context.Context, seriesID int64, provider Provider, season int) ([]Episode, bool, error,
) {
	key := fmt.Sprintf("%d/%d", seriesID, season)
	if episodes, found := filer.listings[key]; found {
		return episodes, episodes != nil, nil
	}
	episodes, err := storedListing(ctx, filer.queries, provider, seriesID, season)
	if err != nil {
		return nil, false, err
	}
	filer.listings[key] = episodes
	return episodes, episodes != nil, nil
}

// storedListing reads and parses a season listing; nil when it was never fetched.
func storedListing(
	ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64, season int,
) ([]Episode, error) {
	bodyGzip, err := store.OrNotFound(queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		ItemID: seriesID, Provider: provider.Name(), Part: seasonPart(season),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read series %d's season %d: %w", seriesID, season, err)
	}
	body, err := bounded.Gunzip(bodyGzip, maxStoredResponseBytes)
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
	episodes, err := queries.ListLiveSeriesEpisodeSources(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list series %d's episodes: %w", seriesID, err)
	}
	var seasons []int
	listings := map[int][]Episode{}
	for _, episode := range episodes {
		if episode.FollowedSource != nil && *episode.FollowedSource == sourceNFO {
			continue // its own NFO speaks for it
		}
		season, number := int(ptr.ValueOrZero(episode.GroupNumber)), int(ptr.ValueOrZero(episode.NumberInGroup))
		if slices.Contains(seasons, season) {
			continue
		}
		listing, isCached := listings[season]
		if !isCached {
			if listing, err = storedListing(ctx, queries, provider, seriesID, season); err != nil {
				return nil, err
			}
			listings[season] = listing
		}
		where := seasonAtProvider{provider: provider, seriesID: seriesID, season: season}
		stale, err := isListingStale(ctx, queries, where, listing, number, episode.AddedAt)
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

// seasonAtProvider names one season of a series at a provider.
type seasonAtProvider struct {
	provider Provider
	seriesID int64
	season   int
}

// isListingStale reports whether a season's listing must be fetched for an episode: it was never
// fetched, or it does not list the episode and was fetched before the episode arrived.
func isListingStale(
	ctx context.Context, queries *sqlcgen.Queries, where seasonAtProvider, listing []Episode, number int,
	arrivedAt int64,
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
	return arrivedAt > fetchedAt, nil
}

// resolveSeries writes a series' own values — its tvshow.nfo, else its stored match, else its
// folders — with a person's overrides over them, and its match state where the library reads
// NFO files only. It reports what the fetch queue owes the series.
func (filer *libraryFiler) resolveSeries(ctx context.Context, seriesID int64, spec seriesSpec) error {
	seriesMatchRow, err := filer.cachedSeriesMatch(ctx, seriesID)
	if err != nil {
		return err
	}
	provider, known := filer.providers[providerOf(filer.library.MetadataSource)]
	own, err := filer.seriesValues(ctx, seriesID, spec, seriesMatchRow)
	if err != nil {
		return err
	}
	values, source := own.values, own.source
	if recordErr := filer.recordSeriesIDs(ctx, seriesID, spec, values); recordErr != nil {
		return recordErr
	}
	overrides, err := filer.queries.ListOverrides(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("read series %d's overrides: %w", seriesID, err)
	}
	for _, override := range overrides {
		values[override.Field] = override.OverrideValue
	}
	identification := identificationUnidentified
	if filer.library.MetadataSource == sourceNFO && spec.hasNFO {
		identification = identificationIdentified
	}
	year, _ := strconv.ParseInt(values[fieldYear], 10, 64) //nolint:errcheck // zero when unset
	err = filer.queries.UpdateSeriesResolved(ctx, sqlcgen.UpdateSeriesResolvedParams{
		Year: ptr.NilIfZero(year), FollowedSource: ptr.NilIfZero(source), IdentificationState: &identification,
		ID: seriesID,
	})
	if err != nil {
		return fmt.Errorf("resolve series %d: %w", seriesID, err)
	}
	err = filer.writeText(ctx, seriesID, itemText{
		sortListing: KindSeries, name: values[fieldName], overview: values[fieldOverview],
	})
	if err != nil {
		return err
	}
	filer.reportImages(seriesID, source, own.artwork)
	if known && isSeriesMatched(seriesMatchRow) {
		seasons, err := seasonsToFetch(ctx, filer.queries, provider, seriesID)
		if err != nil {
			return err
		}
		if len(seasons) > 0 {
			filer.report.AwaitingListings = append(filer.report.AwaitingListings, seriesID)
		}
	}
	return nil
}

// seriesSourceValues is what a series is, as its source says.
type seriesSourceValues struct {
	values  map[string]string
	source  string
	artwork []ImageRef
}

// seriesValues are a series' own values and their source: a person's chosen match over all;
// then its tvshow.nfo; then its stored match; then its folders.
func (filer *libraryFiler) seriesValues(
	ctx context.Context, seriesID int64, spec seriesSpec, seriesMatchRow sqlcgen.GetSeriesForMatchRow,
) (seriesSourceValues, error) {
	values := map[string]string{fieldName: spec.name, fieldYear: strconv.Itoa(spec.year), fieldOverview: spec.overview}
	source := ""
	switch {
	case filer.library.MetadataSource == sourceNFO && !spec.hasNFO:
		source = sourceNone
	case spec.hasNFO:
		source = sourceNFO
	}
	provider, known := filer.providers[providerOf(filer.library.MetadataSource)]
	if !known || !isSeriesMatched(seriesMatchRow) ||
		(spec.hasNFO && ptr.ValueOrZero(seriesMatchRow.IdentificationState) != identificationConfirmed) {
		if source == "" && seriesMatchRow.FollowedSource != nil && *seriesMatchRow.FollowedSource == sourceNone {
			source = sourceNone // its provider found nothing
		}
		return seriesSourceValues{values: values, source: source}, nil
	}
	bodyGzip, err := filer.queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		ItemID: seriesID, Provider: provider.Name(), Part: "",
	})
	if err != nil {
		return seriesSourceValues{}, fmt.Errorf("read series %d's %s response: %w", seriesID, provider.Name(), err)
	}
	body, err := bounded.Gunzip(bodyGzip, maxStoredResponseBytes)
	if err != nil {
		return seriesSourceValues{}, fmt.Errorf("series %d's %s response: %w", seriesID, provider.Name(), err)
	}
	metadata, err := provider.Parse(KindSeries, body)
	if err != nil {
		return seriesSourceValues{}, fmt.Errorf("series %d's %s response: %w", seriesID, provider.Name(), err)
	}
	values[fieldName], values[fieldYear], values[fieldOverview] = metadata.Name, strconv.Itoa(metadata.Year),
		metadata.Overview
	return seriesSourceValues{values: values, source: provider.Name(), artwork: metadata.Artwork}, nil
}

// recordSeriesIDs keeps the ids a series' folders and tvshow.nfo name, and its tvshow.nfo's
// locked fields as overrides (D182).
func (filer *libraryFiler) recordSeriesIDs(
	ctx context.Context, seriesID int64, spec seriesSpec, values map[string]string,
) error {
	for provider, id := range spec.ids {
		err := filer.queries.InsertItemProviderID(ctx, sqlcgen.InsertItemProviderIDParams{
			ItemID: seriesID, Provider: provider, ProviderID: id,
		})
		if err != nil {
			return fmt.Errorf("record series %d's %s %s: %w", seriesID, provider, id, err)
		}
	}
	for _, field := range spec.lockedFields {
		value, overridable := values[field]
		if !overridable {
			continue
		}
		err := filer.queries.InsertOverrideIfAbsent(ctx, sqlcgen.InsertOverrideIfAbsentParams{
			ItemID: seriesID, Field: field, OverrideValue: value, Origin: "nfo", SetAt: filer.nowMs,
		})
		if err != nil {
			return fmt.Errorf("record series %d's locked %s: %w", seriesID, field, err)
		}
	}
	return nil
}

// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// airedOrdering is the ordering an episode's season and number place it in (D96, D159).
const airedOrdering = "aired"

// fileEpisode files an episode: its series, found by where the file already was, the tvshow.nfo
// above it or the name its folders give, and its title at (season, episode) in the series' aired
// order. Its values come from its own NFO; else, where the library reads a provider, from its
// series' season listing (D209); else from the path.
func (filer *libraryFiler) fileEpisode(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, parsed ParsedPath,
) error {
	showNFO, hasShowNFO := filer.showNFOFor(file)
	seriesID, err := filer.seriesOf(ctx, file, parsed, showNFO, hasShowNFO)
	if err != nil {
		return err
	}
	filer.seriesIDByFolder[seriesFolderOf(file.Path)] = seriesID
	season, number := parsed.Season, parsed.Episode
	spec := titleSpec{
		kind: library.TitleKindEpisode, name: fmt.Sprintf("Episode %d", number),
		runtimeMs: probeRuntimeMs(file.ProbeJSON),
		ids:       map[string]string{},
	}
	nfo, hasNFO := filer.nfoFor(file)
	switch {
	case hasNFO && nfo.Kind == "episodedetails":
		if nfo.Season > 0 || nfo.Episode > 0 {
			season, number = nfo.Season, nfo.Episode
		}
		spec.fromNFO(nfo)
		if nfo.Title == "" {
			spec.name = fmt.Sprintf("Episode %d", number)
		}
	case filer.library.MetadataSource != sourceNFO:
		if listErr := filer.fromListing(ctx, seriesID, season, number, &spec); listErr != nil {
			return listErr
		}
	case hasShowNFO:
		spec.source, spec.identification = sourceNFO, identificationIdentified
		spec.year = showNFO.Year
	}
	spec.key = episodeKey(seriesID, season, number)
	filer.unidentifiedUnlessNFO(&spec)
	titleID, err := filer.findOrAddTitle(ctx, spec)
	if err != nil {
		return err
	}
	if err := filer.positionEpisode(ctx, seriesID, titleID, season, number); err != nil {
		return err
	}
	return filer.place(ctx, file, titleID, spec)
}

// showNFOFor is the tvshow.nfo in the nearest folder above a file, if there is one.
func (filer *libraryFiler) showNFOFor(file sqlcgen.ListIdentifyFilesRow) (NFO, bool) {
	for folder := path.Dir(file.Path); folder != "/" && folder != "."; folder = path.Dir(folder) {
		if nfo, found := filer.showNFOs[folder]; found {
			return nfo, true
		}
	}
	return NFO{}, false
}

// seriesOf is an episode's series: the one it is already in, when the file has not moved, so a
// series renamed by its match keeps its episodes; else the one its tvshow.nfo or folders name.
// It gathers what the file says about the series for the pass to resolve.
func (filer *libraryFiler) seriesOf(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, parsed ParsedPath, showNFO NFO, hasShowNFO bool,
) (int64, error) {
	evidence := seriesSpec{name: parsed.Name, year: parsed.Year, ids: maps.Clone(parsed.ProviderIDs)}
	if hasShowNFO {
		evidence.fromNFO(showNFO)
	}
	seriesID, err := filer.currentSeries(ctx, file)
	if err == nil && seriesID == 0 {
		seriesID, err = filer.findOrAddSeries(ctx, evidence.name)
	}
	if err != nil {
		return 0, err
	}
	if _, seen := filer.seriesSpecByID[seriesID]; !seen {
		filer.seriesSpecByID[seriesID] = evidence
	}
	return seriesID, nil
}

// currentSeries is the series a file's episode is already placed in, if it has one and has not
// moved.
func (filer *libraryFiler) currentSeries(ctx context.Context, file sqlcgen.ListIdentifyFilesRow) (int64, error) {
	titleID, hasCurrentTitle, err := filer.currentTitle(ctx, file)
	if err != nil || !hasCurrentTitle {
		return 0, err
	}
	// Found through its place rather than its name.
	place, err := store.OrNotFound(filer.queries.GetEpisodePlace(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read title %d's series: %w", titleID, err)
	}
	return place.CollectionID, nil
}

// fromListing gives an episode its values from its series' stored season listing, or says why
// it cannot (D209).
func (filer *libraryFiler) fromListing(ctx context.Context, seriesID int64, season, number int, spec *titleSpec) error {
	series, err := filer.cachedSeriesMatch(ctx, seriesID)
	if err != nil {
		return err
	}
	provider, known := filer.providers[providerOf(filer.library.MetadataSource)]
	switch {
	case !known:
	case ptr.ValueOrZero(series.IdentificationState) == identificationAmbiguous:
		spec.reason = "its series matches more than one show: choose its match"
	case isSeriesMatched(series):
		episodes, fetched, listErr := filer.seasonListing(ctx, seriesID, provider, season)
		if listErr != nil {
			return listErr
		}
		for _, episode := range episodes {
			if episode.Season == season && episode.Number == number {
				spec.fromEpisode(provider.Name(), episode)
				return nil
			}
		}
		spec.reason = fmt.Sprintf("%s lists no season %d, episode %d", provider.Name(), season, number)
		if !fetched {
			spec.reason = fmt.Sprintf("season %d's listing is not fetched yet", season)
		}
	case series.FollowedSource != nil && *series.FollowedSource == sourceNone:
		spec.reason = provider.Name() + " found no single match for its series"
	default:
		spec.reason = "its series is not matched yet"
	}
	return nil
}

// fromEpisode takes an episode's values from its series' season listing.
func (spec *titleSpec) fromEpisode(provider string, episode Episode) {
	if episode.Name != "" {
		spec.name = episode.Name
	}
	spec.overview, spec.year = episode.Overview, episode.Year
	if episode.RuntimeMs > 0 {
		spec.runtimeMs = episode.RuntimeMs
	}
	spec.source, spec.sourceID, spec.identification, spec.reason = provider, episode.ID, identificationIdentified, ""
	if episode.StillRef != "" {
		spec.artwork = []ArtRef{{Kind: ArtStill, Ref: episode.StillRef}}
	}
}

// findOrAddSeries finds the series collection by name in this library, or makes it.
func (filer *libraryFiler) findOrAddSeries(ctx context.Context, name string) (int64, error) {
	if id, found := filer.seriesIDByName[strings.ToLower(name)]; found {
		return id, nil
	}
	libraryID := filer.library.ID
	id, err := filer.queries.InsertItem(ctx, sqlcgen.InsertItemParams{
		Kind: itemKindCollection, LibraryID: &libraryID, AddedAt: filer.nowMs,
	})
	if err != nil {
		return 0, fmt.Errorf("add series %q: %w", name, err)
	}
	collection := sqlcgen.InsertCollectionParams{ID: id, Kind: "series", CreatedBy: "scanner"}
	err = filer.queries.InsertCollection(ctx, collection)
	if err != nil {
		return 0, fmt.Errorf("add series %q: %w", name, err)
	}
	if err := filer.writeText(ctx, id, itemText{sortListing: KindSeries, name: name}); err != nil {
		return 0, err
	}
	filer.seriesIDByName[strings.ToLower(name)] = id
	return id, nil
}

// positionEpisode puts an episode in its series and in the series' aired order.
func (filer *libraryFiler) positionEpisode(ctx context.Context, seriesID, titleID int64, season, number int) error {
	sortOrder := float64(season)*10_000 + float64(number)
	err := filer.queries.UpsertCollectionMember(ctx, sqlcgen.UpsertCollectionMemberParams{
		CollectionID: seriesID, MemberTitle: &titleID, Position: &sortOrder,
	})
	if err != nil {
		return fmt.Errorf("add title %d to series %d: %w", titleID, seriesID, err)
	}
	aired := sqlcgen.GetOrderingParams{CollectionID: seriesID, Name: airedOrdering}
	ordering, err := filer.queries.GetOrdering(ctx, aired)
	if err != nil {
		ordering, err = filer.queries.InsertOrdering(ctx, sqlcgen.InsertOrderingParams{
			CollectionID: seriesID, Name: airedOrdering,
		})
		if err != nil {
			return fmt.Errorf("add series %d's aired order: %w", seriesID, err)
		}
	}
	groupNumber, numberInGroup := int64(season), int64(number)
	err = filer.queries.UpsertPosition(ctx, sqlcgen.UpsertPositionParams{
		OrderingID: ordering, TitleID: titleID, GroupNumber: &groupNumber, NumberInGroup: &numberInGroup,
		SortOrder: sortOrder,
	})
	if err != nil {
		return fmt.Errorf("place title %d at season %d, episode %d: %w", titleID, season, number, err)
	}
	return nil
}

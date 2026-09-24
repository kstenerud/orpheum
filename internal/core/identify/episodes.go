// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// airedOrdering is the ordering an episode's season and number place it in (D96, D159).
const airedOrdering = "aired"

// episode files an episode: its series, found by where the file already was, the tvshow.nfo
// above it or the name its folders give, and its title at (season, episode) in the series' aired
// order. Its values come from its own NFO; else, where the library reads a provider, from its
// series' season listing (D209); else from the path.
func (work *run) episode(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, parsed Parsed) error {
	show, hasShow := work.showNFOFor(file)
	seriesID, err := work.seriesOf(ctx, file, parsed, show, hasShow)
	if err != nil {
		return err
	}
	season, number := parsed.Season, parsed.Episode
	spec := titleSpec{
		kind: kindEpisode, name: fmt.Sprintf("Episode %d", number), runtimeMs: probeRuntime(file.Probe),
		ids: map[string]string{},
	}
	nfo, hasNFO := work.nfoFor(file)
	switch {
	case hasNFO && nfo.Kind == "episodedetails":
		if nfo.Season > 0 || nfo.Episode > 0 {
			season, number = nfo.Season, nfo.Episode
		}
		spec.fromNFO(nfo)
		if nfo.Title == "" {
			spec.name = fmt.Sprintf("Episode %d", number)
		}
	case work.library.MetadataSource != sourceNFO:
		if listErr := work.fromListing(ctx, seriesID, season, number, &spec); listErr != nil {
			return listErr
		}
	case hasShow:
		spec.source, spec.ident = sourceNFO, identIdentified
		spec.year = show.Year
	}
	spec.key = episodeKey(seriesID, season, number)
	work.unidentifiedUnlessNFO(&spec)
	titleID, err := work.titleFor(ctx, spec)
	if err != nil {
		return err
	}
	if err := work.position(ctx, seriesID, titleID, season, number); err != nil {
		return err
	}
	return work.place(ctx, file, titleID, spec)
}

// showNFOFor is the tvshow.nfo in the nearest folder above a file, if there is one.
func (work *run) showNFOFor(file sqlcgen.ListIdentifyFilesRow) (NFO, bool) {
	for folder := path.Dir(file.Path); folder != "/" && folder != "."; folder = path.Dir(folder) {
		if nfo, found := work.showNFOs[folder]; found {
			return nfo, true
		}
	}
	return NFO{}, false
}

// seriesOf is an episode's series: the one it is already in, when the file has not moved, so a
// series renamed by its match keeps its episodes; else the one its tvshow.nfo or folders name.
// It gathers what the file says about the series for the pass to resolve.
func (work *run) seriesOf(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, parsed Parsed, show NFO, hasShow bool,
) (int64, error) {
	evidence := seriesSpec{name: parsed.Name, year: parsed.Year, ids: maps.Clone(parsed.ProviderIDs)}
	if hasShow {
		evidence.fromNFO(show)
	}
	seriesID, err := work.currentSeries(ctx, file)
	if err == nil && seriesID == 0 {
		seriesID, err = work.seriesFor(ctx, evidence.name)
	}
	if err != nil {
		return 0, err
	}
	if _, seen := work.seriesSeen[seriesID]; !seen {
		work.seriesSeen[seriesID] = evidence
	}
	return seriesID, nil
}

// currentSeries is the series a file's episode is already placed in, if it has one and has not
// moved.
func (work *run) currentSeries(ctx context.Context, file sqlcgen.ListIdentifyFilesRow) (int64, error) {
	titleID, kept, err := work.currentTitle(ctx, file)
	if err != nil || !kept {
		return 0, err
	}
	seriesID, err := store.One(work.queries.GetSeriesOfTitle(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read title %d's series: %w", titleID, err)
	}
	return seriesID, nil
}

// fromListing gives an episode its values from its series' stored season listing, or says why
// it cannot (D209).
func (work *run) fromListing(ctx context.Context, seriesID int64, season, number int, spec *titleSpec) error {
	series, err := work.seriesMatch(ctx, seriesID)
	if err != nil {
		return err
	}
	provider, known := work.providers[providerOf(work.library.MetadataSource)]
	switch {
	case !known:
	case derefText(series.Ident) == identAmbiguous:
		spec.reason = "its series matches more than one show: choose its match"
	case seriesMatched(series):
		episodes, fetched, listErr := work.listing(ctx, seriesID, provider, season)
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
	case series.Source != nil && *series.Source == sourceNone:
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
	spec.source, spec.sourceID, spec.ident, spec.reason = provider, episode.ID, identIdentified, ""
}

// seriesFor finds the series collection by name in this library, or makes it.
func (work *run) seriesFor(ctx context.Context, name string) (int64, error) {
	if id, found := work.series[strings.ToLower(name)]; found {
		return id, nil
	}
	libraryID := work.library.ID
	id, err := work.queries.InsertItem(ctx, sqlcgen.InsertItemParams{
		Kind: itemCollection, LibraryID: &libraryID, CreatedAt: work.now,
	})
	if err != nil {
		return 0, fmt.Errorf("add series %q: %w", name, err)
	}
	collection := sqlcgen.InsertCollectionParams{ID: id, Kind: "series", CreatedBy: "scanner"}
	err = work.queries.InsertCollection(ctx, collection)
	if err != nil {
		return 0, fmt.Errorf("add series %q: %w", name, err)
	}
	named := sqlcgen.UpsertOriginalTextParams{ItemID: id, Lang: langUnknown, Name: name}
	err = work.queries.UpsertOriginalText(ctx, named)
	if err != nil {
		return 0, fmt.Errorf("name series %q: %w", name, err)
	}
	work.series[strings.ToLower(name)] = id
	return id, nil
}

// position puts an episode in its series and in the series' aired order.
func (work *run) position(ctx context.Context, seriesID, titleID int64, season, number int) error {
	sort := float64(season)*10_000 + float64(number)
	err := work.queries.UpsertCollectionMember(ctx, sqlcgen.UpsertCollectionMemberParams{
		CollectionID: seriesID, MemberTitle: &titleID, Position: &sort,
	})
	if err != nil {
		return fmt.Errorf("add title %d to series %d: %w", titleID, seriesID, err)
	}
	aired := sqlcgen.GetOrderingParams{CollectionID: seriesID, Name: airedOrdering}
	ordering, err := work.queries.GetOrdering(ctx, aired)
	if err != nil {
		ordering, err = work.queries.InsertOrdering(ctx, sqlcgen.InsertOrderingParams{
			CollectionID: seriesID, Name: airedOrdering,
		})
		if err != nil {
			return fmt.Errorf("add series %d's aired order: %w", seriesID, err)
		}
	}
	grp, num := int64(season), int64(number)
	err = work.queries.UpsertPosition(ctx, sqlcgen.UpsertPositionParams{
		OrderingID: ordering, TitleID: titleID, Grp: &grp, Number: &num, Sort: sort,
	})
	if err != nil {
		return fmt.Errorf("place title %d at season %d, episode %d: %w", titleID, season, number, err)
	}
	return nil
}

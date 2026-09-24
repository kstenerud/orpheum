// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// airedOrdering is the ordering an episode's season and number place it in (D96, D159).
const airedOrdering = "aired"

// episode files an episode: its series, found by the tvshow.nfo above it or by the name its
// folders give, and its title at (season, episode) in the series' aired order.
func (work *run) episode(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, parsed Parsed) error {
	seriesName, show, hasShow := parsed.Name, NFO{}, false
	for folder := path.Dir(file.Path); folder != "/" && folder != "."; folder = path.Dir(folder) {
		if nfo, found := work.showNFOs[folder]; found {
			show, hasShow = nfo, true
			if nfo.Title != "" {
				seriesName = nfo.Title
			}
			break
		}
	}
	seriesID, err := work.seriesFor(ctx, seriesName)
	if err != nil {
		return err
	}
	season, number := parsed.Season, parsed.Episode
	spec := titleSpec{
		kind: kindEpisode, name: fmt.Sprintf("Episode %d", number), runtimeMs: probeRuntime(file.Probe),
		ids: map[string]string{},
	}
	nfo, hasNFO := work.nfoFor(file)
	if hasNFO && nfo.Kind == "episodedetails" {
		if nfo.Season > 0 || nfo.Episode > 0 {
			season, number = nfo.Season, nfo.Episode
		}
		spec.fromNFO(nfo)
		if nfo.Title == "" {
			spec.name = fmt.Sprintf("Episode %d", number)
		}
	} else if hasShow {
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

// seriesFor finds the series collection by name in this library, or makes it.
func (work *run) seriesFor(ctx context.Context, name string) (int64, error) {
	if id, found := work.series[strings.ToLower(name)]; found {
		return id, nil
	}
	libraryID := work.library.ID
	id, err := work.queries.InsertItem(ctx, sqlcgen.InsertItemParams{
		Kind: "collection", LibraryID: &libraryID, CreatedAt: work.now,
	})
	if err != nil {
		return 0, fmt.Errorf("add series %q: %w", name, err)
	}
	collection := sqlcgen.InsertCollectionParams{ID: id, Kind: "series", CreatedBy: "scanner"}
	err = work.queries.InsertCollection(ctx, collection)
	if err != nil {
		return 0, fmt.Errorf("add series %q: %w", name, err)
	}
	err = work.queries.UpsertOriginalText(ctx, sqlcgen.UpsertOriginalTextParams{ItemID: id, Lang: "und", Name: name})
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

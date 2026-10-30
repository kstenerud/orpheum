// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Art is the images an item shows, by the source SHA-256 /art/ names them with; "" when it has none.
type Art struct {
	PosterSha256   string
	BackdropSha256 string
	StillSha256    string
}

// shownArtSourceSha256s is the image of a kind each item shows, by its source SHA-256 in hex, in
// ListShownArtwork's order: any image before a frame still, which stands in only where no other is
// (D362); a person's choice; then an image from the source the item's library prefers, one beside
// the media unless the library takes TMDB only (D215); then the newest stored.
func (browser *Browser) shownArtSourceSha256s(
	ctx context.Context, kind string, itemIDs []int64,
) (map[int64]string, error) {
	return shownArt(ctx, browser.database.Read(), kind, itemIDs)
}

// shownArt is shownArtSourceSha256s read by queries.
func shownArt(ctx context.Context, queries *sqlcgen.Queries, kind string, itemIDs []int64) (map[int64]string, error) {
	shown, err := shownImages(ctx, queries, kind, itemIDs)
	if err != nil {
		return nil, err
	}
	sourceSha256ByItemID := make(map[int64]string, len(shown))
	for itemID, image := range shown {
		sourceSha256ByItemID[itemID] = image.sourceSha256
	}
	return sourceSha256ByItemID, nil
}

// shownImage is the image of a kind an item shows: its source SHA-256 in hex, and whether it is a
// frame still of the item's video (D362), which any other picture outranks.
type shownImage struct {
	sourceSha256 string
	isFrame      bool
}

// shownImages is shownArt, each image saying whether it is a frame still.
func shownImages(ctx context.Context, queries *sqlcgen.Queries, kind string, itemIDs []int64) (
	map[int64]shownImage, error,
) {
	shownByItemID := map[int64]shownImage{}
	if len(itemIDs) == 0 {
		return shownByItemID, nil
	}
	itemIDsJSON, err := json.Marshal(itemIDs)
	if err != nil {
		return nil, fmt.Errorf("encode %d item ids: %w", len(itemIDs), err)
	}
	rows, err := queries.ListShownArtwork(ctx, sqlcgen.ListShownArtworkParams{
		ItemIDListJSON: string(itemIDsJSON), Kind: kind,
	})
	if err != nil {
		return nil, fmt.Errorf("read the %s of %d items: %w", kind, len(itemIDs), err)
	}
	for _, row := range rows {
		if _, alreadyHas := shownByItemID[row.ItemID]; !alreadyHas {
			shownByItemID[row.ItemID] = shownImage{
				sourceSha256: hex.EncodeToString(row.SourceSha256), isFrame: row.Source == ArtSourceFrame,
			}
		}
	}
	return shownByItemID, nil
}

// itemArt is one item's images.
func (browser *Browser) itemArt(ctx context.Context, itemID int64) (Art, error) {
	var art Art
	for _, kind := range []struct {
		name string
		into *string
	}{{ArtPoster, &art.PosterSha256}, {ArtBackdrop, &art.BackdropSha256}, {ArtStill, &art.StillSha256}} {
		sourceSha256ByItemID, err := browser.shownArtSourceSha256s(ctx, kind.name, []int64{itemID})
		if err != nil {
			return Art{}, err
		}
		*kind.into = sourceSha256ByItemID[itemID]
	}
	return art, nil
}

// withPosters sets each entry's poster, and the still a film or series without one shows in its
// place.
func (browser *Browser) withPosters(ctx context.Context, page Page) (Page, error) {
	ids := make([]int64, 0, len(page.Entries))
	for _, entry := range page.Entries {
		ids = append(ids, entry.ID)
	}
	posters, err := browser.shownArtSourceSha256s(ctx, ArtPoster, ids)
	if err != nil {
		return Page{}, err
	}
	for index := range page.Entries {
		page.Entries[index].PosterSha256 = posters[page.Entries[index].ID]
	}
	if page, err = browser.withNoPosterStills(ctx, page); err != nil {
		return Page{}, err
	}
	return browser.withLandscapes(ctx, page)
}

// withNoPosterStills gives each film and series that has no poster the 16:9 picture its poster's
// place shows above the mark of a missing poster, read anew with each page, so a poster that arrives
// replaces it at once (the owner, 2026-10-27; D362 §2): its backdrop; else, a film's still, which only
// a frame of its video gives it, and a series' first episode's still (ListFirstEpisodes).
func (browser *Browser) withNoPosterStills(ctx context.Context, page Page) (Page, error) {
	var posterless, films, series []int64
	for _, entry := range page.Entries {
		if entry.PosterSha256 != "" {
			continue
		}
		switch entry.Kind {
		case ListFilms:
			posterless, films = append(posterless, entry.ID), append(films, entry.ID)
		case ListSeries:
			posterless, series = append(posterless, entry.ID), append(series, entry.ID)
		}
	}
	if len(posterless) == 0 {
		return page, nil
	}
	queries := browser.database.Read()
	backdrops, err := shownArt(ctx, queries, ArtBackdrop, posterless)
	if err != nil {
		return Page{}, err
	}
	firstEpisodeBySeries, err := firstEpisodes(ctx, queries, series)
	if err != nil {
		return Page{}, err
	}
	stillOf := slices.Clone(films)
	for _, episodeID := range firstEpisodeBySeries {
		stillOf = append(stillOf, episodeID)
	}
	stills, err := shownArt(ctx, queries, ArtStill, stillOf)
	if err != nil {
		return Page{}, err
	}
	for index, entry := range page.Entries {
		if entry.PosterSha256 != "" {
			continue
		}
		switch entry.Kind {
		case ListFilms:
			page.Entries[index].NoPosterStillSha256 = cmp.Or(backdrops[entry.ID], stills[entry.ID])
		case ListSeries:
			firstEpisode := firstEpisodeBySeries[entry.ID]
			page.Entries[index].NoPosterStillSha256 = cmp.Or(backdrops[entry.ID], stills[firstEpisode])
		}
	}
	return page, nil
}

// firstEpisodes are each series' first episode with a live file, of its first regular season, else
// of its specials (ListFirstEpisodes), by series; a series with none has none.
func firstEpisodes(ctx context.Context, queries *sqlcgen.Queries, seriesIDs []int64) (map[int64]int64, error) {
	episodeBySeries := map[int64]int64{}
	if len(seriesIDs) == 0 {
		return episodeBySeries, nil
	}
	seriesIDsJSON, err := json.Marshal(seriesIDs)
	if err != nil {
		return nil, fmt.Errorf("encode %d series ids: %w", len(seriesIDs), err)
	}
	rows, err := queries.ListFirstEpisodes(ctx, string(seriesIDsJSON))
	if err != nil {
		return nil, fmt.Errorf("read the first episodes of %d series: %w", len(seriesIDs), err)
	}
	for _, row := range rows {
		if row.EpisodeID != 0 {
			episodeBySeries[row.SeriesID] = row.EpisodeID
		}
	}
	return episodeBySeries, nil
}

// withLandscapes gives each other video its 16:9 picture: its still, else its backdrop (D349 §3),
// and a frame still of its video only when it has neither (D362 §2).
func (browser *Browser) withLandscapes(ctx context.Context, page Page) (Page, error) {
	var ids []int64
	for _, entry := range page.Entries {
		if entry.Kind == ListOther {
			ids = append(ids, entry.ID)
		}
	}
	if len(ids) == 0 {
		return page, nil
	}
	queries := browser.database.Read()
	stills, err := shownImages(ctx, queries, ArtStill, ids)
	if err != nil {
		return Page{}, err
	}
	backdrops, err := shownArt(ctx, queries, ArtBackdrop, ids)
	if err != nil {
		return Page{}, err
	}
	for index := range page.Entries {
		id := page.Entries[index].ID
		still := stills[id]
		if still.isFrame && backdrops[id] != "" {
			still = shownImage{}
		}
		page.Entries[index].LandscapeSha256 = cmp.Or(still.sourceSha256, backdrops[id])
	}
	return page, nil
}

// SeasonStillMembers are the members whose stills a series' own page shows, as Series sums its
// seasons up: each group's first member with a still, none for a group with none. A look at the page
// raises their artwork, as a title's page raises its own (the owner, Fire TV, 2026-10-25: a series'
// season stills showed blank while their sizes waited their turn).
func SeasonStillMembers(ctx context.Context, queries *sqlcgen.Queries, members []Member) ([]Member, error) {
	ids := make([]int64, 0, len(members))
	for _, each := range members {
		ids = append(ids, each.TitleID)
	}
	stills, err := shownArt(ctx, queries, ArtStill, ids)
	if err != nil {
		return nil, err
	}
	var shown []Member
	isShown := map[int64]bool{}
	for _, each := range members {
		if !isShown[each.Group] && stills[each.TitleID] != "" {
			isShown[each.Group] = true
			shown = append(shown, each)
		}
	}
	return shown, nil
}

// withSeasonPosters gives each of a series' aired seasons its own poster, where one is stored (D379):
// per season, the first of ListSeasonPosters' order.
func (browser *Browser) withSeasonPosters(ctx context.Context, seriesID int64, seasons []SeasonSummary) error {
	rows, err := browser.database.Read().ListSeasonPosters(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("read the season posters of series %d: %w", seriesID, err)
	}
	posterByNumber := make(map[int64]string, len(rows))
	for _, row := range rows {
		if _, alreadyHas := posterByNumber[row.SeasonNumber]; !alreadyHas {
			posterByNumber[row.SeasonNumber] = hex.EncodeToString(row.SourceSha256)
		}
	}
	for index := range seasons {
		seasons[index].PosterSha256 = posterByNumber[seasons[index].Number]
	}
	return nil
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Art is the images an item shows, by the source SHA-256 /art/ names them with; "" when it has none.
type Art struct {
	PosterSha256   string
	BackdropSha256 string
	StillSha256    string
}

// shownArtSourceSha256s is the image of a kind each item shows, by its source SHA-256 in hex, in
// ListShownArtwork's order: a person's choice; then an image from the source the item's library
// prefers, one beside the media unless the library takes TMDB only (D215); then the newest stored.
func (browser *Browser) shownArtSourceSha256s(
	ctx context.Context, kind string, itemIDs []int64,
) (map[int64]string, error) {
	return shownArt(ctx, browser.database.Read(), kind, itemIDs)
}

// shownArt is shownArtSourceSha256s read by queries.
func shownArt(ctx context.Context, queries *sqlcgen.Queries, kind string, itemIDs []int64) (map[int64]string, error) {
	sourceSha256ByItemID := map[int64]string{}
	if len(itemIDs) == 0 {
		return sourceSha256ByItemID, nil
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
		if _, alreadyHas := sourceSha256ByItemID[row.ItemID]; !alreadyHas {
			sourceSha256ByItemID[row.ItemID] = hex.EncodeToString(row.SourceSha256)
		}
	}
	return sourceSha256ByItemID, nil
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

// withPosters sets each entry's poster.
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
	return browser.withLandscapes(ctx, page)
}

// withLandscapes gives each other video its 16:9 picture, its still or else its backdrop (D349 §3).
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
	stills, err := browser.shownArtSourceSha256s(ctx, ArtStill, ids)
	if err != nil {
		return Page{}, err
	}
	backdrops, err := browser.shownArtSourceSha256s(ctx, ArtBackdrop, ids)
	if err != nil {
		return Page{}, err
	}
	for index := range page.Entries {
		id := page.Entries[index].ID
		page.Entries[index].LandscapeSha256 = cmp.Or(stills[id], backdrops[id])
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

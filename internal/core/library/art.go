// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
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

// shownArtSourceSha256s is the image of a kind each item shows, by its source SHA-256 in hex: a
// person's choice, else the newest stored.
func (browser *Browser) shownArtSourceSha256s(
	ctx context.Context, kind string, itemIDs []int64,
) (map[int64]string, error) {
	sourceSha256ByItemID := map[int64]string{}
	if len(itemIDs) == 0 {
		return sourceSha256ByItemID, nil
	}
	itemIDsJSON, err := json.Marshal(itemIDs)
	if err != nil {
		return nil, fmt.Errorf("encode %d item ids: %w", len(itemIDs), err)
	}
	rows, err := browser.database.Read().ListShownArtwork(ctx, sqlcgen.ListShownArtworkParams{
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
	return page, nil
}

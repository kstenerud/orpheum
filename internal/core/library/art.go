// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The kinds of image an item shows, as the artwork table names them.
const (
	artPoster   = "poster"
	artBackdrop = "backdrop"
	artStill    = "still"
)

// Art is the images an item shows, by the hash /art/ names them with; "" when it has none.
type Art struct {
	PosterHash   string
	BackdropHash string
	StillHash    string
}

// shownArtHashes is the image of a kind each item shows, by hash: a person's choice, else the newest
// stored.
func (browser *Browser) shownArtHashes(ctx context.Context, kind string, itemIDs []int64) (map[int64]string, error) {
	hashes := map[int64]string{}
	if len(itemIDs) == 0 {
		return hashes, nil
	}
	itemIDsJSON, err := json.Marshal(itemIDs)
	if err != nil {
		return nil, fmt.Errorf("encode %d item ids: %w", len(itemIDs), err)
	}
	rows, err := browser.database.Read().ListShownArtwork(ctx, sqlcgen.ListShownArtworkParams{
		ItemIds: string(itemIDsJSON), Kind: kind,
	})
	if err != nil {
		return nil, fmt.Errorf("read the %s of %d items: %w", kind, len(itemIDs), err)
	}
	for _, row := range rows {
		if _, alreadyHas := hashes[row.ItemID]; !alreadyHas {
			hashes[row.ItemID] = hex.EncodeToString(row.SourceSha256)
		}
	}
	return hashes, nil
}

// itemArt is one item's images.
func (browser *Browser) itemArt(ctx context.Context, itemID int64) (Art, error) {
	var art Art
	for _, kind := range []struct {
		name string
		into *string
	}{{artPoster, &art.PosterHash}, {artBackdrop, &art.BackdropHash}, {artStill, &art.StillHash}} {
		hashes, err := browser.shownArtHashes(ctx, kind.name, []int64{itemID})
		if err != nil {
			return Art{}, err
		}
		*kind.into = hashes[itemID]
	}
	return art, nil
}

// withPosters sets each entry's poster.
func (browser *Browser) withPosters(ctx context.Context, page Page) (Page, error) {
	ids := make([]int64, 0, len(page.Entries))
	for _, entry := range page.Entries {
		ids = append(ids, entry.ID)
	}
	posters, err := browser.shownArtHashes(ctx, artPoster, ids)
	if err != nil {
		return Page{}, err
	}
	for index := range page.Entries {
		page.Entries[index].PosterHash = posters[page.Entries[index].ID]
	}
	return page, nil
}

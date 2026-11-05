// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"encoding/hex"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// insertItemWithArt makes an item and stores one image of a kind beside it, returning the item's
// id and the image's source SHA-256 in hex.
func insertItemWithArt(t *testing.T, world *shelf, artKind string, sourceSha256 byte) (int64, string) {
	t.Helper()
	var itemID int64
	sha := make([]byte, 32)
	sha[0] = sourceSha256
	err := world.database.Write(world.t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		itemID, err = queries.InsertItem(world.t.Context(), sqlcgen.InsertItemParams{
			Kind: "movie", LibraryID: new(int64(1)), AddedAt: 1, IdentificationState: new("identified"),
		})
		if err != nil {
			return err
		}
		_, err = queries.InsertArtwork(world.t.Context(), sqlcgen.InsertArtworkParams{
			ItemID: itemID, Kind: artKind, Source: "local", SourceSha256: sha,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return itemID, hex.EncodeToString(sha)
}

// An other video's 16:9 picture is its still when it has one, else its backdrop; with neither, it
// has none (D349 §3).
func TestWithLandscapes_AnOtherVideosPictureIsItsStillElseItsBackdrop(t *testing.T) {
	t.Parallel()
	world := newShelf(t)

	withStill, stillSha := insertItemWithArt(t, world, ArtStill, 0x11)
	err := world.database.Write(world.t.Context(), func(queries *sqlcgen.Queries) error {
		_, err := queries.InsertArtwork(world.t.Context(), sqlcgen.InsertArtworkParams{
			ItemID: withStill, Kind: ArtBackdrop, Source: "local", SourceSha256: []byte{0x22},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	withBackdropOnly, backdropSha := insertItemWithArt(t, world, ArtBackdrop, 0x33)
	withNeither, _ := insertItemWithArt(t, world, ArtPoster, 0x44) // a poster is not a landscape

	page := Page{Entries: []Entry{
		{ID: withStill, Kind: ListOther},
		{ID: withBackdropOnly, Kind: ListOther},
		{ID: withNeither, Kind: ListOther},
	}}
	got, err := world.browser.withLandscapes(world.t.Context(), page)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{withStill: stillSha, withBackdropOnly: backdropSha, withNeither: ""}
	for _, entry := range got.Entries {
		if entry.LandscapeSha256 != want[entry.ID] {
			t.Errorf("item %d's landscape: %q; want %q", entry.ID, entry.LandscapeSha256, want[entry.ID])
		}
	}
}

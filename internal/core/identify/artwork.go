// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// storedArt is one artwork row.
type storedArt = sqlcgen.ListLiveArtworkRow

// OwedArtwork is an item's images its source names that are not stored yet: the artwork queue's
// work (D212).
type OwedArtwork struct {
	ItemID int64
	Source string
	Refs   []ArtRef
}

// oweArtwork notes the images an item's source names that its artwork does not hold: new ones,
// and ones the source changed. One the node refused is held, with no hash, and not asked for again.
func (filer *libraryFiler) oweArtwork(ctx context.Context, itemID int64, source string, refs []ArtRef) error {
	if source != SourceLocal {
		refs = filer.withoutLocalKinds(itemID, refs)
	}
	if len(refs) == 0 || source == "" || source == sourceNFO || source == sourceNone {
		return nil
	}
	stored, err := filer.queries.ListLiveArtwork(ctx, itemID)
	if err != nil {
		return fmt.Errorf("list item %d's artwork: %w", itemID, err)
	}
	var owed []ArtRef
	for _, ref := range refs {
		held := slices.ContainsFunc(stored, func(row storedArt) bool {
			return row.Kind == ref.Kind && row.Source == source && ptr.ValueOrZero(row.SourceRef) == ref.Ref
		})
		if !held {
			owed = append(owed, ref)
		}
	}
	if len(owed) > 0 {
		owedByItem := OwedArtwork{ItemID: itemID, Source: source, Refs: owed}
		filer.report.OwedArtwork = append(filer.report.OwedArtwork, owedByItem)
	}
	return nil
}

// This package writes the artwork table and nothing else does (schema.md, "Who writes each
// table"): it decides which images an item has, and the artwork service, which fetches and stores
// them, records each outcome through RecordImage.

// ItemImage is one image of an item's, from one source, and the source SHA-256 its sizes are
// stored under; a nil hash is an image that could not be stored.
type ItemImage struct {
	ItemID       int64
	Source       string
	Ref          ArtRef
	SourceSha256 []byte
}

// RecordImage records an item's image: stored, or, with no hash, refused, so it is not fetched
// again (D213 §1). A stored image retires the source's earlier one of its kind, never a person's
// choice, so an older image stays shown only until a newer one is stored.
func RecordImage(ctx context.Context, queries *sqlcgen.Queries, image ItemImage, now int64) error {
	id, err := queries.InsertArtwork(ctx, sqlcgen.InsertArtworkParams{
		ItemID: image.ItemID, Kind: image.Ref.Kind, Source: image.Source, SourceRef: &image.Ref.Ref,
		SourceSha256: image.SourceSha256,
	})
	if err != nil {
		return fmt.Errorf("record item %d's %s: %w", image.ItemID, image.Ref.Kind, err)
	}
	if image.SourceSha256 == nil {
		return nil
	}
	err = queries.RetireReplacedArtwork(ctx, sqlcgen.RetireReplacedArtworkParams{
		RemovedAt: &now, ItemID: image.ItemID, Kind: image.Ref.Kind, Source: image.Source, KeepID: id,
	})
	if err != nil {
		return fmt.Errorf("retire item %d's earlier %s: %w", image.ItemID, image.Ref.Kind, err)
	}
	return nil
}

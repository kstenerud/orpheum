// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// This file is the artwork table's one writer (schema.md, "Who writes each table"): which images
// an item holds, from which source, and when one goes. identify finds the images an item's
// source names and never imports this package; the server's wiring hands what it found to Take.

// SourceLocal is an image beside the media, read and re-encoded by the node (D215).
const SourceLocal = "local"

// Ref is one image a source names: its kind (library.ArtPoster, ArtBackdrop, ArtLogo, ArtStill)
// and the source's reference to it — a provider's path, or localRef's name for a file beside the
// media. A make_artwork_sizes job's arguments carry it as it is.
type Ref struct {
	Kind      string `json:"kind"`
	SourceRef string `json:"sourceRef"`
}

// ItemImages are the images one item's source names (D212).
type ItemImages struct {
	ItemID int64
	Source string
	Refs   []Ref
}

// LocalImage is an image a pass found beside the media for an item (D215): its kind, and its file
// with the size and modification time by which a changed file is owed again.
type LocalImage struct {
	ItemID       int64
	Kind         string
	FileID       int64
	SizeBytes    int64
	ModifiedAtMs int64
}

// LocalImages are every image a pass found beside one library's media.
type LocalImages struct {
	LibraryID int64
	Images    []LocalImage
}

// Found is what a pass or a match found of items' images, as identify reported it.
type Found struct {
	// Rematched are items matched to another work at a source they followed, with the new work's
	// images: their images from that source the new work does not name were the earlier work's,
	// and go (D223 §9).
	Rematched []ItemImages
	// BesideTheMedia, from a pass, is what it found beside its library's media: an image an item
	// showed from beside the media that was not found again goes (D215 §1). Nil from a match.
	BesideTheMedia *LocalImages
	// Named are the images each item's provider names; with the images beside the media, what
	// an item does not hold is fetched (D212).
	Named []ItemImages
}

// Take holds what a pass or a match found: what is no longer named goes, and a job is queued for
// each item with images it does not hold. It logs a failure; the next pass finds the same again.
func (service *Service) Take(ctx context.Context, found Found) {
	named := found.Named
	if found.BesideTheMedia != nil {
		named = append(slices.Clone(named), localItemImages(found.BesideTheMedia.Images)...)
	}
	err := service.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return service.retireUnnamed(ctx, queries, found)
	})
	var owed []ItemImages
	if err == nil {
		owed, err = service.owed(ctx, named)
	}
	if err != nil {
		service.parts.Logger.ErrorContext(ctx, "could not take the images a pass found", "err", err)
		return
	}
	service.queueOwed(ctx, owed)
}

// localItemImages groups the images found beside the media by item, in item order, each named
// by localRef.
func localItemImages(images []LocalImage) []ItemImages {
	var byItem []ItemImages
	for _, image := range images {
		index := slices.IndexFunc(byItem, func(item ItemImages) bool { return item.ItemID == image.ItemID })
		if index < 0 {
			byItem = append(byItem, ItemImages{ItemID: image.ItemID, Source: SourceLocal})
			index = len(byItem) - 1
		}
		byItem[index].Refs = append(byItem[index].Refs, Ref{Kind: image.Kind, SourceRef: localRef(image)})
	}
	slices.SortFunc(byItem, func(a, b ItemImages) int { return cmp.Compare(a.ItemID, b.ItemID) })
	return byItem
}

// retireUnnamed retires a rematched item's images of the earlier work, and, after a pass, the
// images beside the media it no longer found beside their item: a deleted poster.jpg, or a video
// renamed away from its image. Without it, a removed image kept showing, ranked first (D215 §1).
// A person's choice stays.
func (service *Service) retireUnnamed(ctx context.Context, queries *sqlcgen.Queries, found Found) error {
	now := service.parts.Clock.Now().UnixMilli()
	for _, rematched := range found.Rematched {
		if err := retireAllBut(ctx, queries, rematched, now); err != nil {
			return err
		}
	}
	if found.BesideTheMedia == nil {
		return nil
	}
	libraryID := found.BesideTheMedia.LibraryID
	items, err := queries.ListItemsWithLiveUnchosenLocalArtwork(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list library %d's local artwork: %w", libraryID, err)
	}
	local := localItemImages(found.BesideTheMedia.Images)
	for _, itemID := range items {
		kept := ItemImages{ItemID: itemID, Source: SourceLocal}
		if index := slices.IndexFunc(local, func(item ItemImages) bool { return item.ItemID == itemID }); index >= 0 {
			kept = local[index]
		}
		if err := retireAllBut(ctx, queries, kept, now); err != nil {
			return err
		}
	}
	return nil
}

// retireAllBut retires an item's images from a source that are not among the refs kept; a
// person's choice stays.
func retireAllBut(ctx context.Context, queries *sqlcgen.Queries, kept ItemImages, now int64) error {
	refs := make([]string, 0, len(kept.Refs))
	for _, ref := range kept.Refs {
		refs = append(refs, ref.SourceRef)
	}
	encoded, err := json.Marshal(refs)
	if err != nil {
		return fmt.Errorf("encode item %d's %s artwork: %w", kept.ItemID, kept.Source, err)
	}
	err = queries.RetireArtworkNotFound(ctx, sqlcgen.RetireArtworkNotFoundParams{
		RemovedAt: &now, ItemID: kept.ItemID, Source: kept.Source, FoundRefs: string(encoded),
	})
	if err != nil {
		return fmt.Errorf("retire item %d's %s artwork no longer named: %w", kept.ItemID, kept.Source, err)
	}
	return nil
}

// owed is, of the images each item's source names, those its artwork does not hold: new ones,
// and ones the source changed. One the node refused is held, with no hash, and not asked for
// again.
func (service *Service) owed(ctx context.Context, named []ItemImages) ([]ItemImages, error) {
	var owed []ItemImages
	for _, item := range named {
		rows, err := service.parts.Database.Read().ListLiveArtwork(ctx, item.ItemID)
		if err != nil {
			return nil, fmt.Errorf("list item %d's artwork: %w", item.ItemID, err)
		}
		var missing []Ref
		for _, ref := range item.Refs {
			if !holds(rows, item.Source, ref) {
				missing = append(missing, ref)
			}
		}
		if len(missing) > 0 {
			owed = append(owed, ItemImages{ItemID: item.ItemID, Source: item.Source, Refs: missing})
		}
	}
	return owed, nil
}

// holds reports whether an item's live artwork has an image, stored or refused.
func holds(rows []sqlcgen.ListLiveArtworkRow, source string, ref Ref) bool {
	return slices.ContainsFunc(rows, func(row sqlcgen.ListLiveArtworkRow) bool {
		return row.Kind == ref.Kind && row.Source == source && row.SourceRef != nil && *row.SourceRef == ref.SourceRef
	})
}

// record records an item's image: stored, or, with no hash, refused, so it is not fetched again
// (D213 §1). A stored image retires the source's earlier one of its kind, never a person's
// choice, so an older image stays shown only until a newer one is stored. An item removed since
// it was queued records nothing.
func (service *Service) record(ctx context.Context, want wantedImage, sourceSha256 []byte) error {
	now := service.parts.Clock.Now().UnixMilli()
	err := service.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, err := store.OrNotFound(queries.GetItemKind(ctx, want.itemID))
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read item %d: %w", want.itemID, err)
		}
		id, err := queries.InsertArtwork(ctx, sqlcgen.InsertArtworkParams{
			ItemID: want.itemID, Kind: want.ref.Kind, Source: want.source, SourceRef: &want.ref.SourceRef,
			SourceSha256: sourceSha256,
		})
		if err != nil || sourceSha256 == nil {
			return err
		}
		err = queries.RetireReplacedArtwork(ctx, sqlcgen.RetireReplacedArtworkParams{
			RemovedAt: &now, ItemID: want.itemID, Kind: want.ref.Kind, Source: want.source, KeepID: id,
		})
		if err != nil {
			return fmt.Errorf("retire its earlier %s: %w", want.ref.Kind, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("record item %d's %s: %w", want.itemID, want.ref.Kind, err)
	}
	return nil
}

// RetryImages makes items' missing and refused images owed again, for their next pass to find
// (D213 §1, §2): a refresh that looks again, which identify asks for through the server's wiring.
func (service *Service) RetryImages(ctx context.Context, itemIDs ...int64) error {
	now := service.parts.Clock.Now().UnixMilli()
	err := service.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, itemID := range itemIDs {
			err := queries.RetryItemArtwork(ctx, sqlcgen.RetryItemArtworkParams{RemovedAt: &now, ItemID: itemID})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ask again for the images of %v: %w", itemIDs, err)
	}
	return nil
}

// localRef is how an image beside the media is named in the artwork table and a
// make_artwork_sizes job: its file, and its size and modification time, so a changed file is owed
// again.
func localRef(image LocalImage) string {
	return fmt.Sprintf("file:%d:%d:%d", image.FileID, image.SizeBytes, image.ModifiedAtMs)
}

// localRefFileID is the id of the file a localRef names.
func localRefFileID(ref string) (int64, bool) {
	var fileID, sizeBytes, mtimeMs int64
	if _, err := fmt.Sscanf(ref, "file:%d:%d:%d", &fileID, &sizeBytes, &mtimeMs); err != nil {
		return 0, false
	}
	return fileID, true
}

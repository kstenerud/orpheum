// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// itemText is what an item is called and about, as the pass resolved it.
type itemText struct {
	sortListing string // the list it sorts in: a title's kind, or "series"
	name        string
	overview    string
	tagline     string
	sortTitle   string // the NFO's own sort name, which wins over the name's key (D402)
}

// writeText writes an item's text, with its name folded by the one normaliser for search (D48)
// and its sort key in the library's locale (D47, D402): the only way the pass names anything. Its text
// is already clean: paths, NFO files, providers and people are cleaned where they enter
// (principles §22).
func (filer *libraryFiler) writeText(ctx context.Context, itemID int64, text itemText) error {
	original := sqlcgen.UpsertOriginalTextParams{
		ItemID: itemID, Language: langtag.Undetermined, Name: text.name, Overview: ptr.NilIfZero(text.overview),
		Tagline: ptr.NilIfZero(text.tagline), SearchAlternates: ptr.NilIfZero(textindex.Fold(text.name)),
		SortTitle: ptr.NilIfZero(text.sortTitle),
	}
	sortKey := sqlcgen.UpsertSortKeyParams{
		ItemID: itemID, Locale: filer.sorter.Locale(), Listing: text.sortListing,
		CollationKey: sortKeyOf(filer.sorter, text.name, text.sortTitle),
	}
	if filer.isTextHeld(itemID, original, sortKey) {
		return nil
	}
	if err := filer.queries.UpsertOriginalText(ctx, original); err != nil {
		return fmt.Errorf("name item %d: %w", itemID, err)
	}
	if err := filer.queries.UpsertSortKey(ctx, sortKey); err != nil {
		return fmt.Errorf("sort item %d: %w", itemID, err)
	}
	filer.noteText(itemID, original, sortKey)
	return nil
}

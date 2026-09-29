// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// itemText is what an item is called and about, as the pass resolved it.
type itemText struct {
	sortListing string // the list it sorts in: a title's kind, or "series"
	name        string
	overview    string
	tagline     string
}

// writeText writes an item's text, with its name folded by the one normaliser for search (D48)
// and its sort key in the library's locale (D47): the only way the pass names anything. Its text
// is already clean: paths, NFO files, providers and people are cleaned where they enter
// (principles §22).
func (filer *libraryFiler) writeText(ctx context.Context, itemID int64, text itemText) error {
	err := filer.queries.UpsertOriginalText(ctx, sqlcgen.UpsertOriginalTextParams{
		ItemID: itemID, Language: langUnknown, Name: text.name, Overview: nilIfEmpty(text.overview),
		Tagline: nilIfEmpty(text.tagline), SearchAlternates: nilIfEmpty(textindex.Fold(text.name)),
	})
	if err != nil {
		return fmt.Errorf("name item %d: %w", itemID, err)
	}
	locale := filer.sorter.Locale()
	err = filer.queries.DeleteItemSortKeys(ctx, sqlcgen.DeleteItemSortKeysParams{ItemID: itemID, Locale: locale})
	if err != nil {
		return fmt.Errorf("clear item %d's sort key: %w", itemID, err)
	}
	err = filer.queries.InsertSortKey(ctx, sqlcgen.InsertSortKeyParams{
		ItemID: itemID, Locale: locale, Listing: text.sortListing, CollationKey: filer.sorter.Key(text.name),
	})
	if err != nil {
		return fmt.Errorf("sort item %d: %w", itemID, err)
	}
	return nil
}

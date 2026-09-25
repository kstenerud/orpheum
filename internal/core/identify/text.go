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
	listing  string // the list it sorts in: a title's kind, or "series"
	name     string
	overview string
	tagline  string
}

// writeText writes an item's text, with its name folded by the one normaliser for search (D48)
// and its sort key in the library's locale (D47): the only way the pass names anything. Its text
// is already clean: paths, NFO files, providers and people are cleaned where they enter
// (principles §22).
func (work *run) writeText(ctx context.Context, itemID int64, text itemText) error {
	err := work.queries.UpsertOriginalText(ctx, sqlcgen.UpsertOriginalTextParams{
		ItemID: itemID, Lang: langUnknown, Name: text.name, Overview: optional(text.overview),
		Tagline: optional(text.tagline), SearchAlt: optional(textindex.Fold(text.name)),
	})
	if err != nil {
		return fmt.Errorf("name item %d: %w", itemID, err)
	}
	locale := work.sorter.Locale()
	err = work.queries.DeleteItemSortKeys(ctx, sqlcgen.DeleteItemSortKeysParams{ItemID: itemID, Locale: locale})
	if err != nil {
		return fmt.Errorf("clear item %d's sort key: %w", itemID, err)
	}
	err = work.queries.InsertSortKey(ctx, sqlcgen.InsertSortKeyParams{
		ItemID: itemID, Locale: locale, Listing: text.listing, Key: work.sorter.Key(text.name),
	})
	if err != nil {
		return fmt.Errorf("sort item %d: %w", itemID, err)
	}
	return nil
}

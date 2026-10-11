// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"bytes"
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// RefreshSortAndSearchKeys makes every live name's sort key in the sort_locale setting's locale,
// and its fold for search, what this build derives from the name, now. Both are functions of a
// stored name and the locale alone, so neither a change of locale nor a build that collates or
// folds otherwise waits for a scan (D47, D48; agent-failures A4). It compares each stored key and
// fold with one derived afresh, rewrites only those that differ, and drops other locales' keys.
// It returns how many items it rewrote.
func (pass *Pass) RefreshSortAndSearchKeys(ctx context.Context) (int, error) {
	sorter, err := pass.sorter(ctx)
	if err != nil {
		return 0, err
	}
	locale := sorter.Locale()
	rewrittenCount := 0
	err = pass.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if dropErr := queries.DeleteSortKeysOfOtherLocales(ctx, locale); dropErr != nil {
			return fmt.Errorf("drop other locales' sort keys: %w", dropErr)
		}
		rows, listErr := queries.ListNamesWithSortAndSearchKeys(ctx, locale)
		if listErr != nil {
			return fmt.Errorf("list the names to key: %w", listErr)
		}
		for _, item := range storedNameKeys(rows) {
			rewrote, keyErr := rekeyName(ctx, queries, sorter, item)
			if keyErr != nil {
				return keyErr
			}
			if rewrote {
				rewrittenCount++
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("refresh the %s sort keys and the search folds: %w", locale, err)
	}
	return rewrittenCount, nil
}

// storedNameKey is one named item as stored: its name, its fold for search, the list it sorts in
// ("" for none), and its sort keys in the locale.
type storedNameKey struct {
	itemID           int64
	name             string
	searchAlternates string
	listing          string
	sortKeys         []storedSortKey
}

// storedSortKey is one sort_key row of an item in the locale.
type storedSortKey struct {
	listing      string
	collationKey []byte
}

// storedNameKeys groups the rows, one per stored sort key, into one per item, in the rows' order.
func storedNameKeys(rows []sqlcgen.ListNamesWithSortAndSearchKeysRow) []storedNameKey {
	var items []storedNameKey
	for _, row := range rows {
		if len(items) == 0 || items[len(items)-1].itemID != row.ItemID {
			items = append(items, storedNameKey{
				itemID: row.ItemID, name: row.Name, searchAlternates: ptr.ValueOrZero(row.SearchAlternates),
				listing: sortListingOf(row.TitleKind, row.CollectionKind),
			})
		}
		if row.KeyedListing != nil {
			last := &items[len(items)-1]
			stored := storedSortKey{listing: *row.KeyedListing, collationKey: row.CollationKey}
			last.sortKeys = append(last.sortKeys, stored)
		}
	}
	return items
}

// sortListingOf is the list an item sorts in: a title's kind, or "series"; any other collection
// sorts in none.
func sortListingOf(titleKind, collectionKind *string) string {
	if titleKind != nil {
		return *titleKind
	}
	if ptr.ValueOrZero(collectionKind) == KindSeries {
		return KindSeries
	}
	return ""
}

// rekeyName rewrites an item's fold and sort key where they are not what its name derives now,
// and says whether it rewrote either.
func rekeyName(
	ctx context.Context, queries *sqlcgen.Queries, sorter *textindex.Sorter, item storedNameKey,
) (bool, error) {
	rewrote := false
	if folded := textindex.Fold(item.name); folded != item.searchAlternates {
		err := queries.SetSearchAlternates(ctx, sqlcgen.SetSearchAlternatesParams{
			ItemID: item.itemID, SearchAlternates: ptr.NilIfZero(folded),
		})
		if err != nil {
			return false, fmt.Errorf("fold item %d's name again: %w", item.itemID, err)
		}
		rewrote = true
	}
	if item.listing == "" {
		return rewrote, nil
	}
	key := sorter.Key(item.name)
	if len(item.sortKeys) == 1 && item.sortKeys[0].listing == item.listing &&
		bytes.Equal(item.sortKeys[0].collationKey, key) {
		return rewrote, nil
	}
	stale := sqlcgen.DeleteItemSortKeysParams{ItemID: item.itemID, Locale: sorter.Locale()}
	if err := queries.DeleteItemSortKeys(ctx, stale); err != nil {
		return false, fmt.Errorf("clear item %d's sort key: %w", item.itemID, err)
	}
	err := queries.InsertSortKey(ctx, sqlcgen.InsertSortKeyParams{
		ItemID: item.itemID, Locale: sorter.Locale(), Listing: item.listing, CollationKey: key,
	})
	if err != nil {
		return false, fmt.Errorf("sort item %d: %w", item.itemID, err)
	}
	return true, nil
}

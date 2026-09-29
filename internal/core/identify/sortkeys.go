// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// RefreshSortKeys makes every named title and series sortable in the sort_locale setting's
// locale, now: a sort key is a function of a stored name and the locale alone, so a change of
// locale needs no scan (D47; agent-failures A4). It does nothing when no key is missing, and
// otherwise rebuilds the locale's keys and drops other locales'. It returns how many it wrote.
func (pass *Pass) RefreshSortKeys(ctx context.Context) (int, error) {
	sorter, err := pass.sorter(ctx)
	if err != nil {
		return 0, err
	}
	locale := sorter.Locale()
	missing, err := pass.database.Read().CountItemsMissingSortKey(ctx, locale)
	if err != nil {
		return 0, fmt.Errorf("count the items with no %s sort key: %w", locale, err)
	}
	if missing == 0 {
		return 0, nil
	}
	written := 0
	err = pass.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if dropErr := queries.DeleteSortKeysOfOtherLocales(ctx, locale); dropErr != nil {
			return fmt.Errorf("drop other locales' sort keys: %w", dropErr)
		}
		items, listErr := queries.ListItemsToSort(ctx)
		if listErr != nil {
			return fmt.Errorf("list the items to sort: %w", listErr)
		}
		for _, item := range items {
			stale := sqlcgen.DeleteItemSortKeysParams{ItemID: item.ItemID, Locale: locale}
			if clearErr := queries.DeleteItemSortKeys(ctx, stale); clearErr != nil {
				return fmt.Errorf("clear item %d's sort key: %w", item.ItemID, clearErr)
			}
			insertErr := queries.InsertSortKey(ctx, sqlcgen.InsertSortKeyParams{
				ItemID: item.ItemID, Locale: locale, Listing: item.Listing, CollationKey: sorter.Key(item.Name),
			})
			if insertErr != nil {
				return fmt.Errorf("sort item %d: %w", item.ItemID, insertErr)
			}
		}
		written = len(items)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("rebuild the %s sort keys: %w", locale, err)
	}
	return written, nil
}

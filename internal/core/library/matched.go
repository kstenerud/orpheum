// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// matchedNamesAtMost is how many matching names a searched list draws its titles from: the best
// first where the index ranks them, by name where it does not (D48). A list is searched to narrow
// it, and a query that matches more names than this narrows nothing a person would page through.
const matchedNamesAtMost = 10000

// matchedItems is the items a list holds: every one, the zero value, or, for a list searched, those
// its query matched.
type matchedItems struct {
	// idsJSON is their ids, a JSON array, as a list's query takes them (item_ids_json); nil for
	// every item.
	idsJSON *string
}

// withMatched is query with the items its Query matches, of its reach, found by the same planning
// and indexes as Search (one listing API, search a criterion of it: the owner, 2026-10-30), and
// whether none can match: a query that plans to nothing, as Search finds nothing for it, or one that
// matches no title.
func (browser *Browser) withMatched(ctx context.Context, query ListQuery) (ListQuery, bool, error) {
	isFiltered := query.GenreID != 0 || query.PersonID != 0 || query.Decade != 0 || query.IsUnwatched
	if query.Query == "" && !isFiltered {
		return query, false, nil
	}
	var ids []int64
	if query.Query != "" {
		plan := textindex.PlanQuery(query.Query)
		if plan.IndexQuery == "" {
			return query, true, nil
		}
		isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
		matched, err := browser.database.MatchListed(ctx, store.SearchQuery{
			Route: store.SearchRoute(plan.Route), IndexQuery: plan.IndexQuery, Limit: matchedNamesAtMost,
			IsEveryLibrary: isEveryLibrary, LibraryIDsJSON: libraryIDsJSON,
		})
		if err != nil {
			return query, false, fmt.Errorf("match a list's query: %w", err)
		}
		ids = matched
	}
	if isFiltered {
		kept, err := browser.database.Read().ListFilteredItemIDs(ctx, sqlcgen.ListFilteredItemIDsParams{
			GenreID: ptr.NilIfZero(query.GenreID), PersonID: ptr.NilIfZero(query.PersonID),
			Decade:      ptr.NilIfZero(query.Decade),
			IsUnwatched: boolInt(query.IsUnwatched), UserID: query.UserID,
		})
		if err != nil {
			return query, false, fmt.Errorf("filter a list: %w", err)
		}
		if query.Query == "" {
			ids = kept
		} else {
			ids = slices.DeleteFunc(ids, func(id int64) bool {
				_, isKept := slices.BinarySearch(kept, id)
				return !isKept
			})
		}
	}
	if len(ids) == 0 {
		return query, true, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return query, false, fmt.Errorf("encode a list's matches: %w", err)
	}
	query.matched = matchedItems{idsJSON: new(string(encoded))}
	return query, false, nil
}

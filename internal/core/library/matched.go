// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store"
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
	if query.Query == "" {
		return query, false, nil
	}
	plan := textindex.PlanQuery(query.Query)
	if plan.IndexQuery == "" {
		return query, true, nil
	}
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	ids, err := browser.database.MatchListed(ctx, store.SearchQuery{
		Route: store.SearchRoute(plan.Route), IndexQuery: plan.IndexQuery, Limit: matchedNamesAtMost,
		IsEveryLibrary: isEveryLibrary, LibraryIDsJSON: libraryIDsJSON,
	})
	if err != nil {
		return query, false, fmt.Errorf("match a list's query: %w", err)
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

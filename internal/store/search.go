// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// SearchRoute is which index a search asks (D48); internal/core/textindex decides it.
type SearchRoute string

// The routes, as textindex names them.
const (
	SearchRouteSubstring SearchRoute = "substring"
	SearchRouteWords     SearchRoute = "words"
	SearchRouteTrigram   SearchRoute = "trigram"
)

// SearchHit is one item a search found.
type SearchHit struct {
	ItemID    int64
	ItemKind  string  // title or collection
	TitleKind *string // movie, episode or extra; nil for a collection
	Name      string
	Year      *int64
}

// The search statements: written by hand because sqlc cannot parse FTS5's MATCH, and static, so
// nothing a person types becomes SQL (gosec G201, G202). The column lists are the same in each.
const (
	searchColumns = `item_text.item_id, item.kind, title.kind, item_text.name,
  coalesce(title.year, collection.year)`
	searchJoins = `JOIN item ON item.id = item_text.item_id
LEFT JOIN title ON title.id = item.id
LEFT JOIN collection ON collection.id = item.id`
	// The listed check keeps only what browse would list, and of the libraries the caller reaches (D243
	// section 4): a title a library of those lists by a live file at a place it covers, or a series one
	// of whose episodes it does (version_library, D363 sections 6, 8). Every library, ?2 = 1, or those
	// in the JSON array of ids ?3. A film whose files are all gone, or that no library covers, is kept,
	// with its history, but not shown, in search as in browse (browse.sql). The statements number
	// their arguments: the index's query ?1, the reach ?2 and ?3, the limit ?4.
	searchReached = `(CAST(?2 AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(?3 AS TEXT))))`
	searchListedCondition = `item.removed_at IS NULL AND (
  (title.id IS NOT NULL AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND ` + searchReached + `))
  OR (collection.id IS NOT NULL AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND ` + searchReached + `)))`

	searchWordsSQL = `SELECT ` + searchColumns + `
FROM item_fts
JOIN item_text ON item_text.rowid = item_fts.rowid
` + searchJoins + `
WHERE item_fts MATCH ?1 AND ` + searchListedCondition + `
ORDER BY bm25(item_fts), item_text.item_id
LIMIT ?4`

	searchTrigramSQL = `SELECT ` + searchColumns + `
FROM item_fts_tri
JOIN item_text ON item_text.rowid = item_fts_tri.rowid
` + searchJoins + `
WHERE item_fts_tri MATCH ?1 AND ` + searchListedCondition + `
ORDER BY item_text.name, item_text.item_id
LIMIT ?4`

	searchSubstringSQL = `SELECT ` + searchColumns + `
FROM item_text
` + searchJoins + `
WHERE instr(item_text.search_alternates, ?1) > 0 AND ` + searchListedCondition + `
ORDER BY item_text.name, item_text.item_id
LIMIT ?4`
)

// SearchQuery is one search: the index's query, a folded substring, an FTS5 query or a trigram
// phrase as Route needs, at most Limit hits, of the libraries the caller reaches.
type SearchQuery struct {
	Route      SearchRoute
	IndexQuery string
	Limit      int64
	// IsEveryLibrary is 1 for a caller who reaches every library; otherwise LibraryIDsJSON, a JSON
	// array of ids, lists those it reaches (auth.Reach.QueryArgs).
	IsEveryLibrary int64
	LibraryIDsJSON string
}

// Search asks one index for the items matching query (D48).
func (database *Database) Search(ctx context.Context, query SearchQuery) (hits []SearchHit, err error) {
	statement := map[SearchRoute]string{
		SearchRouteSubstring: searchSubstringSQL,
		SearchRouteWords:     searchWordsSQL,
		SearchRouteTrigram:   searchTrigramSQL,
	}[query.Route]
	if statement == "" {
		return nil, fmt.Errorf("search: no route %q", query.Route)
	}
	if tally := querytally.Of(ctx); tally != nil {
		route := string(query.Route)
		tally.Count("-- name: Search" + strings.ToUpper(route[:1]) + route[1:] + " ") // SearchSubstring…
	}
	rows, err := database.readers.QueryContext(ctx, statement, query.IndexQuery, query.IsEveryLibrary,
		query.LibraryIDsJSON, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", query.Route, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("search %s: %w", query.Route, closeErr)
		}
	}()
	hits = []SearchHit{}
	for rows.Next() {
		var hit SearchHit
		var titleKind sql.NullString
		var year sql.NullInt64
		if err := rows.Scan(&hit.ItemID, &hit.ItemKind, &titleKind, &hit.Name, &year); err != nil {
			return nil, fmt.Errorf("search %s: %w", query.Route, err)
		}
		if titleKind.Valid {
			hit.TitleKind = &titleKind.String
		}
		if year.Valid {
			hit.Year = &year.Int64
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search %s: %w", query.Route, err)
	}
	return hits, nil
}

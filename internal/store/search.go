// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"
)

// SearchRoute is which index a search asks (D48); internal/core/textindex decides it.
type SearchRoute string

// The routes, as textindex names them.
const (
	SearchShort   SearchRoute = "short"
	SearchWords   SearchRoute = "words"
	SearchTrigram SearchRoute = "trigram"
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

	searchWordsSQL = `SELECT ` + searchColumns + `
FROM item_fts
JOIN item_text ON item_text.rowid = item_fts.rowid
` + searchJoins + `
WHERE item_fts MATCH ? AND item.removed_at IS NULL
ORDER BY bm25(item_fts), item_text.item_id
LIMIT ?`

	searchTrigramSQL = `SELECT ` + searchColumns + `
FROM item_fts_tri
JOIN item_text ON item_text.rowid = item_fts_tri.rowid
` + searchJoins + `
WHERE item_fts_tri MATCH ? AND item.removed_at IS NULL
ORDER BY item_text.name, item_text.item_id
LIMIT ?`

	searchShortSQL = `SELECT ` + searchColumns + `
FROM item_text
` + searchJoins + `
WHERE instr(item_text.search_alternates, ?) > 0 AND item.removed_at IS NULL
ORDER BY item_text.name, item_text.item_id
LIMIT ?`
)

// Search asks one index for the items matching a folded, routed query (D48), at most limit.
func (database *Database) Search(ctx context.Context, route SearchRoute, match string, limit int64) (
	hits []SearchHit, err error,
) {
	statement := map[SearchRoute]string{
		SearchShort: searchShortSQL, SearchWords: searchWordsSQL, SearchTrigram: searchTrigramSQL,
	}[route]
	if statement == "" {
		return nil, fmt.Errorf("search: no route %q", route)
	}
	rows, err := database.readers.QueryContext(ctx, statement, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", route, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("search %s: %w", route, closeErr)
		}
	}()
	hits = []SearchHit{}
	for rows.Next() {
		var hit SearchHit
		var titleKind sql.NullString
		var year sql.NullInt64
		if err := rows.Scan(&hit.ItemID, &hit.ItemKind, &titleKind, &hit.Name, &year); err != nil {
			return nil, fmt.Errorf("search %s: %w", route, err)
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
		return nil, fmt.Errorf("search %s: %w", route, err)
	}
	return hits, nil
}

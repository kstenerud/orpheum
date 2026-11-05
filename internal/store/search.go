// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
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
	ItemKind  string  // the item's kind: movie, series, other and the rest (D426 §2)
	TitleKind *string // movie, episode or extra; nil for a collection
	Name      string
	Year      *int64
	// Place is where a grouped search's hit stands in its order (FindTitles), from which the next
	// page goes on; Search leaves it zero.
	Place SearchPlace
}

// The search statements: written by hand because sqlc cannot parse FTS5's MATCH, and static, so
// nothing a person types becomes SQL (gosec G201, G202). The column lists are the same in each.
const (
	searchColumns = `item_text.item_id, item.kind, work.kind, item_text.name,
  item.year`
	searchJoins = `JOIN item ON item.id = item_text.item_id
LEFT JOIN work ON work.id = item.id
LEFT JOIN sequence ON sequence.id = item.id`
	// The listed check keeps only what browse would list, and of the libraries the caller reaches (D243
	// section 4): a title a library of those lists by a live file at a place it covers, or a series one
	// of whose episodes it does (version_library, D363 sections 6, 8). Every library, ?2 = 1, or those
	// in the JSON array of ids ?3. A film whose files are all gone, or that no library covers, is kept,
	// with its history, but not shown, in search as in browse (browse.sql). The statements number
	// their arguments: the index's query ?1, the reach ?2 and ?3, the limit ?4.
	searchReached = `(CAST(?2 AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(?3 AS TEXT))))`
	searchListedCondition = `item.removed_at IS NULL AND (
  (work.id IS NOT NULL AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = work.id AND version.removed_at IS NULL AND ` + searchReached + `))
  OR (sequence.id IS NOT NULL AND EXISTS (
    SELECT 1 FROM series_member
    CROSS JOIN version ON version.work_id = series_member.member_work
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE series_member.sequence_id = sequence.id AND version.removed_at IS NULL
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

	// The matching statements are a list's search (one listing API, search a criterion of it: the
	// owner, 2026-10-30): the same match and the same listed check as search's, but only the ids, and
	// only of what a list lists, films, other videos and series; never an episode or an extra. They
	// number their arguments as search's do.
	matchListable = `(work.kind IN ('movie', 'other') OR sequence.kind = 'series')`

	matchWordsSQL = `SELECT item_text.item_id
FROM item_fts
JOIN item_text ON item_text.rowid = item_fts.rowid
` + searchJoins + `
WHERE item_fts MATCH ?1 AND ` + matchListable + ` AND ` + searchListedCondition + `
ORDER BY bm25(item_fts), item_text.item_id
LIMIT ?4`

	matchTrigramSQL = `SELECT item_text.item_id
FROM item_fts_tri
JOIN item_text ON item_text.rowid = item_fts_tri.rowid
` + searchJoins + `
WHERE item_fts_tri MATCH ?1 AND ` + matchListable + ` AND ` + searchListedCondition + `
ORDER BY item_text.name, item_text.item_id
LIMIT ?4`

	matchSubstringSQL = `SELECT item_text.item_id
FROM item_text
` + searchJoins + `
WHERE instr(item_text.search_alternates, ?1) > 0 AND ` + matchListable + ` AND ` + searchListedCondition + `
ORDER BY item_text.name, item_text.item_id
LIMIT ?4`

	// The grouped search's statements (search-by-more-2026-11-04.md): its titles are what a list
	// lists, as the matching statements' are, with their names; its people are those credited on one
	// of them (step 0067). Each is a page in the order the index ranks it, each item once, at its best
	// name's place: its rank, the words index's (FTS5's rank, its bm25) or its name, then its id. A
	// page starts after the place (?5, ?6), as every list's cursor does (D416 section 6).
	findTitlesListed  = ` AND ` + matchListable + ` AND ` + searchListedCondition
	findTitlesColumns = `item_text.item_id, item.kind, work.kind, item_text.name, item.year`
	findPage          = `
GROUP BY item_text.item_id
HAVING (hit_rank, item_text.item_id) > (?5, CAST(?6 AS INTEGER))
ORDER BY hit_rank, item_text.item_id
LIMIT ?4`

	findTitlesWordsSQL = `SELECT ` + findTitlesColumns + `, min(item_fts.rank) AS hit_rank
FROM item_fts
JOIN item_text ON item_text.rowid = item_fts.rowid
` + searchJoins + `
WHERE item_fts MATCH ?1` + findTitlesListed + findPage

	findTitlesTrigramSQL = `SELECT ` + findTitlesColumns + `, min(item_text.name) AS hit_rank
FROM item_fts_tri
JOIN item_text ON item_text.rowid = item_fts_tri.rowid
` + searchJoins + `
WHERE item_fts_tri MATCH ?1` + findTitlesListed + findPage

	findTitlesSubstringSQL = `SELECT ` + findTitlesColumns + `, min(item_text.name) AS hit_rank
FROM item_text
` + searchJoins + `
WHERE instr(item_text.search_alternates, ?1) > 0` + findTitlesListed + findPage

	// A person is found by their item's text, as a title is (D48), when a film, other video or series
	// of the libraries the caller reaches credits them: the credited item is named item inside the
	// check, as the listed condition names it.
	findPeopleColumns = `item_text.item_id, item_text.name, person.picture_ref`
	findPeopleJoins   = `
JOIN person ON person.id = item_text.item_id
JOIN item AS person_item ON person_item.id = person.id`
	findPeopleCredited = ` AND person_item.removed_at IS NULL AND EXISTS (
  SELECT 1 FROM credit
  CROSS JOIN item ON item.id = credit.item_id
  LEFT JOIN work ON work.id = item.id
  LEFT JOIN sequence ON sequence.id = item.id
  WHERE credit.person_id = person.id AND ` + matchListable + ` AND ` + searchListedCondition + `)`

	findPeopleWordsSQL = `SELECT ` + findPeopleColumns + `, min(item_fts.rank) AS hit_rank
FROM item_fts
JOIN item_text ON item_text.rowid = item_fts.rowid` + findPeopleJoins + `
WHERE item_fts MATCH ?1` + findPeopleCredited + findPage

	findPeopleTrigramSQL = `SELECT ` + findPeopleColumns + `, min(item_text.name) AS hit_rank
FROM item_fts_tri
JOIN item_text ON item_text.rowid = item_fts_tri.rowid` + findPeopleJoins + `
WHERE item_fts_tri MATCH ?1` + findPeopleCredited + findPage

	findPeopleSubstringSQL = `SELECT ` + findPeopleColumns + `, min(item_text.name) AS hit_rank
FROM item_text` + findPeopleJoins + `
WHERE instr(item_text.search_alternates, ?1) > 0` + findPeopleCredited + findPage
)

// SearchQuery is one search: the index's query, a folded substring, an FTS5 query or a trigram
// phrase as Route needs, at most Limit hits, of the libraries the caller reaches.
type SearchQuery struct {
	Route      SearchRoute
	IndexQuery string
	Limit      int64
	// After is where a grouped search's page starts (FindTitles, FindPeople): after this place, nil
	// for the first page. The other statements take none.
	After *SearchPlace
	// IsEveryLibrary is 1 for a caller who reaches every library; otherwise LibraryIDsJSON, a JSON
	// array of ids, lists those it reaches (auth.Reach.QueryArgs).
	IsEveryLibrary int64
	LibraryIDsJSON string
}

// SearchPlace is a grouped search's hit's place in its order (FindTitles, FindPeople): its rank, as
// its route ranks, then its id.
type SearchPlace struct {
	// Score is the words route's rank: the words index's (FTS5's rank, its bm25), the lower the
	// better, read back exactly as the index gave it, since the next page's query compares it again.
	Score float64
	// Name is the trigram and substring routes' rank: the hit's name.
	Name string
	ID   int64
}

// Search asks one index for the items matching query (D48).
func (database *Database) Search(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	hits := []SearchHit{}
	err := database.askIndex(ctx, "Search", map[SearchRoute]string{
		SearchRouteSubstring: searchSubstringSQL,
		SearchRouteWords:     searchWordsSQL,
		SearchRouteTrigram:   searchTrigramSQL,
	}, query, func(rows *sql.Rows) error {
		hit, err := scanHit(rows)
		hits = append(hits, hit)
		return err
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// scanHit reads one row of searchColumns.
func scanHit(rows *sql.Rows) (SearchHit, error) {
	var hit SearchHit
	var titleKind sql.NullString
	var year sql.NullInt64
	if err := rows.Scan(&hit.ItemID, &hit.ItemKind, &titleKind, &hit.Name, &year); err != nil {
		return SearchHit{}, fmt.Errorf("read a hit: %w", err)
	}
	if titleKind.Valid {
		hit.TitleKind = &titleKind.String
	}
	if year.Valid {
		hit.Year = &year.Int64
	}
	return hit, nil
}

// FindTitles is a page of the films, other videos and series whose names match query, of the
// libraries it reaches, the best first where the index ranks them (D48): a grouped search's titles,
// each once, at the place of its best name that matches, after query.After.
func (database *Database) FindTitles(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	hits := []SearchHit{}
	err := database.askIndex(ctx, "FindTitles", map[SearchRoute]string{
		SearchRouteSubstring: findTitlesSubstringSQL,
		SearchRouteWords:     findTitlesWordsSQL,
		SearchRouteTrigram:   findTitlesTrigramSQL,
	}, query, func(rows *sql.Rows) error {
		var hit SearchHit
		var titleKind sql.NullString
		var year sql.NullInt64
		var rank any
		if err := rows.Scan(&hit.ItemID, &hit.ItemKind, &titleKind, &hit.Name, &year, &rank); err != nil {
			return fmt.Errorf("read a hit: %w", err)
		}
		if titleKind.Valid {
			hit.TitleKind = &titleKind.String
		}
		if year.Valid {
			hit.Year = &year.Int64
		}
		place, err := placeOf(query.Route, rank, hit.ItemID)
		hit.Place = place
		hits = append(hits, hit)
		return err
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// PersonHit is one person a search found.
type PersonHit struct {
	PersonID   int64
	Name       string
	PictureRef *string
	// Place is where the person stands in the search's order, from which the next page goes on.
	Place SearchPlace
}

// FindPeople is a page of the people whose names match query and whom a film, other video or
// series of the libraries it reaches credits, the best first where the index ranks them (D48), each
// once, after query.After.
func (database *Database) FindPeople(ctx context.Context, query SearchQuery) ([]PersonHit, error) {
	hits := []PersonHit{}
	err := database.askIndex(ctx, "FindPeople", map[SearchRoute]string{
		SearchRouteSubstring: findPeopleSubstringSQL,
		SearchRouteWords:     findPeopleWordsSQL,
		SearchRouteTrigram:   findPeopleTrigramSQL,
	}, query, func(rows *sql.Rows) error {
		var hit PersonHit
		var pictureRef sql.NullString
		var rank any
		if err := rows.Scan(&hit.PersonID, &hit.Name, &pictureRef, &rank); err != nil {
			return fmt.Errorf("read a person: %w", err)
		}
		if pictureRef.Valid {
			hit.PictureRef = &pictureRef.String
		}
		place, err := placeOf(query.Route, rank, hit.PersonID)
		hit.Place = place
		hits = append(hits, hit)
		return err
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// placeOf is a grouped search's hit's place: the rank its route's statement read, and its id.
func placeOf(route SearchRoute, rank any, id int64) (SearchPlace, error) {
	switch value := rank.(type) {
	case float64:
		if route == SearchRouteWords {
			return SearchPlace{Score: value, ID: id}, nil
		}
	case string:
		if route != SearchRouteWords {
			return SearchPlace{Name: value, ID: id}, nil
		}
	case []byte:
		if route != SearchRouteWords {
			return SearchPlace{Name: string(value), ID: id}, nil
		}
	}
	return SearchPlace{}, fmt.Errorf("a %s hit's rank %T is not its route's", route, rank)
}

// afterArguments are a grouped search's ?5 and ?6: the place its page starts after, in its route's
// rank, or before every hit for the first page.
func afterArguments(query SearchQuery) (any, int64) {
	switch {
	case query.After == nil && query.Route == SearchRouteWords:
		return math.Inf(-1), 0
	case query.After == nil:
		return "", 0
	case query.Route == SearchRouteWords:
		return query.After.Score, query.After.ID
	default:
		return query.After.Name, query.After.ID
	}
}

// MatchListed is the ids of the films, other videos and series whose names match query, of the
// libraries it reaches, each once: from at most query.Limit names, the best first where the index
// ranks them (D48). A list searched holds only these (browse.sql).
func (database *Database) MatchListed(ctx context.Context, query SearchQuery) ([]int64, error) {
	ids := []int64{}
	seen := map[int64]bool{}
	err := database.askIndex(ctx, "Match", map[SearchRoute]string{
		SearchRouteSubstring: matchSubstringSQL,
		SearchRouteWords:     matchWordsSQL,
		SearchRouteTrigram:   matchTrigramSQL,
	}, query, func(rows *sql.Rows) error {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("read a match: %w", err)
		}
		if !seen[id] { // an item matches once for each of its names that does
			seen[id] = true
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// askIndex runs the statement of statements for query's route, counted as name and the route
// (SearchSubstring…), each row read by readRow.
func (database *Database) askIndex(
	ctx context.Context, name string, statements map[SearchRoute]string, query SearchQuery,
	readRow func(*sql.Rows) error,
) (err error) {
	statement := statements[query.Route]
	if statement == "" {
		return fmt.Errorf("%s: no route %q", name, query.Route)
	}
	route := string(query.Route)
	if tally := querytally.Of(ctx); tally != nil {
		tally.Count("-- name: " + name + strings.ToUpper(route[:1]) + route[1:] + " ")
	}
	afterRank, afterID := afterArguments(query)
	rows, err := database.readers.QueryContext(ctx, statement, query.IndexQuery, query.IsEveryLibrary,
		query.LibraryIDsJSON, query.Limit, afterRank, afterID)
	if err != nil {
		return fmt.Errorf("%s %s: %w", name, route, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("%s %s: %w", name, route, closeErr)
		}
	}()
	for rows.Next() {
		if err := readRow(rows); err != nil {
			return fmt.Errorf("%s %s: %w", name, route, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%s %s: %w", name, route, err)
	}
	return nil
}

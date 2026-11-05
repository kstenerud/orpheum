// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The roles a credit is kept in (step 0067), as the identification pass writes them.
const (
	RoleCast     = "cast"
	RoleDirector = "director"
	RoleWriter   = "writer"
)

// Genre is a genre as a title's page or the genre chips show it.
type Genre struct {
	ID   int64
	Name string
	// TitleCount is how many films, series or other videos the caller reaches are in it, where a
	// listing of genres counts them; zero on a title's page.
	TitleCount int64
}

// CreditedPerson is a person as a title's page credits them.
type CreditedPerson struct {
	PersonID int64
	Name     string
	// Character is a cast member's part; "" for crew and where the source names none.
	Character string
	// PictureRef is their picture as their source names it, a TMDB profile path or an NFO's thumb;
	// not fetched (step 0067).
	PictureRef string
}

// TitlePeople is a title's genres, in its source's order, and its people in billing order within
// each role.
type TitlePeople struct {
	Genres    []Genre
	Cast      []CreditedPerson
	Directors []CreditedPerson
	Writers   []CreditedPerson
}

// peopleOf is an item's genres and credits: a film's, an other video's or a series' own.
func (browser *Browser) peopleOf(ctx context.Context, itemID int64) (TitlePeople, error) {
	queries := browser.database.Read()
	people := TitlePeople{
		Genres: []Genre{}, Cast: []CreditedPerson{}, Directors: []CreditedPerson{}, Writers: []CreditedPerson{},
	}
	genres, err := queries.ListItemGenres(ctx, itemID)
	if err != nil {
		return TitlePeople{}, fmt.Errorf("list item %d's genres: %w", itemID, err)
	}
	for _, genre := range genres {
		people.Genres = append(people.Genres, Genre{ID: genre.GenreID, Name: genre.Name})
	}
	credits, err := queries.ListItemCredits(ctx, itemID)
	if err != nil {
		return TitlePeople{}, fmt.Errorf("list item %d's credits: %w", itemID, err)
	}
	for _, credit := range credits {
		person := CreditedPerson{
			PersonID: credit.PersonID, Name: credit.Name, Character: ptr.ValueOrZero(credit.Character),
			PictureRef: ptr.ValueOrZero(credit.PictureRef),
		}
		switch credit.Job {
		case RoleCast:
			people.Cast = append(people.Cast, person)
		case RoleDirector:
			people.Directors = append(people.Directors, person)
		case RoleWriter:
			people.Writers = append(people.Writers, person)
		}
	}
	return people, nil
}

// PersonCredit is one of a person's roles on an item.
type PersonCredit struct {
	Role      string
	Character string
}

// PersonDetail is a person's page: their name and their picture. The items crediting them are a
// list's, GET /items?person= (D425 §7).
type PersonDetail struct {
	ID         int64
	Name       string
	PictureRef string
}

// Person is a person's page as reach sees it; a person no title it reaches credits is ErrNoItem, as
// one that does not exist (D243 §4).
func (browser *Browser) Person(ctx context.Context, reach auth.Reach, personID int64) (PersonDetail, error) {
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	person, err := store.OrNotFound(browser.database.Read().GetPersonReached(ctx, sqlcgen.GetPersonReachedParams{
		PersonID: personID, IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return PersonDetail{}, fmt.Errorf("person %d: %w", personID, ErrNoItem)
	}
	if err != nil {
		return PersonDetail{}, fmt.Errorf("read person %d: %w", personID, err)
	}
	return PersonDetail{ID: person.ID, Name: person.Name, PictureRef: ptr.ValueOrZero(person.PictureRef)}, nil
}

// CreditsOn are a person's roles on each of the items named, by item, in billing order within each
// role: what a list of the items crediting them says of each (D425 §7). An item that does not
// credit them is left out.
func (browser *Browser) CreditsOn(
	ctx context.Context, personID int64, itemIDs []int64,
) (map[int64][]PersonCredit, error) {
	idsJSON, err := json.Marshal(itemIDs)
	if err != nil {
		return nil, fmt.Errorf("encode the items' ids: %w", err)
	}
	rows, err := browser.database.Read().ListPersonCreditsOn(ctx, sqlcgen.ListPersonCreditsOnParams{
		PersonID: personID, ItemIdsJSON: string(idsJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("list person %d's credits: %w", personID, err)
	}
	credits := make(map[int64][]PersonCredit, len(itemIDs))
	for _, row := range rows {
		credits[row.ItemID] = append(credits[row.ItemID],
			PersonCredit{Role: row.Job, Character: ptr.ValueOrZero(row.Character)})
	}
	return credits, nil
}

// Genres are the genres with a film, series or other video of one of kinds (ListFilms, ListSeries,
// ListOther; ListKinds for every one) that reach reaches, each with how many, by name.
func (browser *Browser) Genres(ctx context.Context, reach auth.Reach, kinds []string) ([]Genre, error) {
	return browser.GenresAfter(ctx, reach, kinds, "", 0, -1)
}

// GenresAfter is a page of Genres: at most limit (-1 for every one) after the genre named afterName
// with afterID ("" and 0 for the first page).
func (browser *Browser) GenresAfter(
	ctx context.Context, reach auth.Reach, kinds []string, afterName string, afterID, limit int64,
) ([]Genre, error) {
	kindsJSON, err := json.Marshal(distinctKinds(kinds))
	if err != nil {
		return nil, fmt.Errorf("list the genres: %w", err)
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	rows, err := browser.database.Read().ListGenresWithCounts(ctx, sqlcgen.ListGenresWithCountsParams{
		AfterName: afterName, AfterID: afterID,
		KindsJSON: string(kindsJSON), IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON, MaxRows: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list the genres: %w", err)
	}
	genres := make([]Genre, 0, len(rows))
	for _, row := range rows {
		genres = append(genres, Genre{ID: row.ID, Name: row.Name, TitleCount: row.WorkCount})
	}
	return genres, nil
}

// The groups of a grouped search, in the order it shows them.
const (
	GroupTitles = "titles"
	GroupPeople = "people"
	GroupGenres = "genres"
)

// GroupedQuery is one grouped search: the text typed, of the libraries reach reaches, a page of
// Limit hits in each group, or of one group alone after its Cursor when Group names one.
type GroupedQuery struct {
	Reach auth.Reach
	Query string
	Group string // "" for every group
	// Cursor is Group's NextCursor from the page before, "" for the first: the browse lists' cursor
	// (EncodeCursor), its key the last hit's rank as the group ranks, its id the hit's (D416 §6).
	Cursor string
	Limit  int64
}

// GroupedHits is a grouped search's page: each group's hits, the best first, and the cursor of
// each one's next page, "" on its last.
type GroupedHits struct {
	Titles           []store.SearchHit
	People           []store.PersonHit
	Genres           []Genre
	NextTitlesCursor string
	NextPeopleCursor string
	NextGenresCursor string
}

// SearchGrouped finds titles, people and genres by what was typed, from its first letter
// (search-by-more-2026-11-04.md §4): titles and people through the index that can answer it (D48),
// the substring route for a query under three characters; genres by their folded names. No group is
// capped but by its page. A cursor this server did not give, or one with no group, is ErrBadCursor.
func (browser *Browser) SearchGrouped(ctx context.Context, query GroupedQuery) (GroupedHits, error) {
	hits := GroupedHits{Titles: []store.SearchHit{}, People: []store.PersonHit{}, Genres: []Genre{}}
	if query.Cursor != "" && query.Group == "" {
		return GroupedHits{}, ErrBadCursor
	}
	plan := textindex.PlanQuery(query.Query)
	if plan.IndexQuery == "" {
		return hits, nil
	}
	route := store.SearchRoute(plan.Route)
	indexQuery, err := indexQueryOf(query, route, plan.IndexQuery)
	if err != nil {
		return GroupedHits{}, err
	}
	if query.Group == "" || query.Group == GroupTitles {
		if hits.Titles, err = browser.database.FindTitles(ctx, indexQuery); err != nil {
			return GroupedHits{}, fmt.Errorf("search titles: %w", err)
		}
		hits.Titles, hits.NextTitlesCursor = pageOf(hits.Titles, query.Limit, func(hit store.SearchHit) string {
			return searchCursor(route, hit.Place)
		})
	}
	if query.Group == "" || query.Group == GroupPeople {
		if hits.People, err = browser.database.FindPeople(ctx, indexQuery); err != nil {
			return GroupedHits{}, fmt.Errorf("search people: %w", err)
		}
		hits.People, hits.NextPeopleCursor = pageOf(hits.People, query.Limit, func(hit store.PersonHit) string {
			return searchCursor(route, hit.Place)
		})
	}
	if query.Group == "" || query.Group == GroupGenres {
		if hits.Genres, hits.NextGenresCursor, err = browser.genresMatching(ctx, query); err != nil {
			return GroupedHits{}, err
		}
	}
	return hits, nil
}

// indexQueryOf is the index's query for a grouped search's titles and people: one more than the
// page, so that whether there are more needs no count, after the place its cursor holds when it
// pages one of them.
func indexQueryOf(query GroupedQuery, route store.SearchRoute, indexQuery string) (store.SearchQuery, error) {
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	asked := store.SearchQuery{
		Route: route, IndexQuery: indexQuery, Limit: query.Limit + 1,
		IsEveryLibrary: isEveryLibrary, LibraryIDsJSON: libraryIDsJSON,
	}
	if query.Cursor != "" && (query.Group == GroupTitles || query.Group == GroupPeople) {
		after, err := searchPlaceAfter(route, query.Cursor)
		if err != nil {
			return store.SearchQuery{}, err
		}
		asked.After = &after
	}
	return asked, nil
}

// genresMatching is a page of the genres whose names hold what query typed, after its cursor, in
// matchGenres' order, and the next page's cursor.
func (browser *Browser) genresMatching(ctx context.Context, query GroupedQuery) ([]Genre, string, error) {
	var after *genreHit
	if query.Cursor != "" {
		key, id, err := DecodeCursor(query.Cursor)
		if err != nil {
			return nil, "", err
		}
		if after, err = genreHitOfKey(key, id); err != nil {
			return nil, "", err
		}
	}
	genres, err := browser.Genres(ctx, query.Reach, ListKinds())
	if err != nil {
		return nil, "", err
	}
	matched := matchGenres(genres, textindex.Fold(query.Query))
	if after != nil {
		start, _ := slices.BinarySearchFunc(matched, *after, compareGenreHits)
		for start < len(matched) && compareGenreHits(matched[start], *after) <= 0 {
			start++
		}
		matched = matched[start:]
	}
	page, next := pageOf(matched, query.Limit, func(hit genreHit) string {
		return EncodeCursor(genreHitKey(hit), hit.ID)
	})
	pageGenres := make([]Genre, 0, len(page))
	for _, hit := range page {
		pageGenres = append(pageGenres, hit.Genre)
	}
	return pageGenres, next, nil
}

// searchPlaceAfter is where a page of a group of titles or people starts: after the place cursor
// holds, and ErrBadCursor for a cursor this server did not give. Its key is the rank as route
// ranks: the words index's score (float64Key), or the hit's name.
func searchPlaceAfter(route store.SearchRoute, cursor string) (store.SearchPlace, error) {
	key, id, err := DecodeCursor(cursor)
	if err != nil {
		return store.SearchPlace{}, err
	}
	if route != store.SearchRouteWords {
		return store.SearchPlace{Name: string(key), ID: id}, nil
	}
	score, isScore := keyFloat64(key)
	if !isScore {
		return store.SearchPlace{}, ErrBadCursor
	}
	return store.SearchPlace{Score: score, ID: id}, nil
}

// searchCursor is the cursor of the page after a hit at place: searchPlaceAfter reads it back.
func searchCursor(route store.SearchRoute, place store.SearchPlace) string {
	if route == store.SearchRouteWords {
		return EncodeCursor(float64Key(place.Score), place.ID)
	}
	return EncodeCursor([]byte(place.Name), place.ID)
}

// pageOf is the first limit of hits, and the cursor made from the last of them when there were
// more, "" when there were not.
func pageOf[Hit any](hits []Hit, limit int64, cursorOf func(Hit) string) ([]Hit, string) {
	if int64(len(hits)) > limit {
		return hits[:limit], cursorOf(hits[limit-1])
	}
	return hits, ""
}

// genreHit is a genre a search matched, with how well: matchGenres' rank, the lower the better.
type genreHit struct {
	Genre
	rank int64
}

// compareGenreHits is matchGenres' order: by rank, then the most titles first, then by name and id.
func compareGenreHits(first, second genreHit) int {
	return cmp.Or(cmp.Compare(first.rank, second.rank), cmp.Compare(second.TitleCount, first.TitleCount),
		cmp.Compare(first.Name, second.Name), cmp.Compare(first.ID, second.ID))
}

// genreHitKey is a genre hit's place as a cursor's key: its rank and its title count (Int64Key
// each), then its name.
func genreHitKey(hit genreHit) []byte {
	return append(append(Int64Key(hit.rank), Int64Key(hit.TitleCount)...), hit.Name...)
}

// genreHitOfKey reads genreHitKey's place back, with the id of the cursor that held it; ErrBadCursor
// for a key that is not one.
func genreHitOfKey(key []byte, id int64) (*genreHit, error) {
	const numbers = 16
	if len(key) < numbers {
		return nil, ErrBadCursor
	}
	rank, _ := KeyInt64(key[:numbers/2])
	titleCount, _ := KeyInt64(key[numbers/2 : numbers])
	return &genreHit{Genre: Genre{ID: id, Name: string(key[numbers:]), TitleCount: titleCount}, rank: rank}, nil
}

// matchGenres are the genres whose folded names hold folded, the most relevant first: those that
// begin with it, then those with a word that does, then the rest; within each, the most titles
// first, then by name (compareGenreHits).
func matchGenres(genres []Genre, folded string) []genreHit {
	folded = strings.TrimSpace(folded)
	if folded == "" {
		return []genreHit{}
	}
	rank := func(genre Genre) int64 {
		name := textindex.Fold(genre.Name)
		switch {
		case strings.HasPrefix(name, folded):
			return 0
		case strings.Contains(" "+name, " "+folded):
			return 1
		case strings.Contains(name, folded):
			return 2
		}
		return -1
	}
	matched := []genreHit{}
	for _, genre := range genres {
		if genreRank := rank(genre); genreRank >= 0 {
			matched = append(matched, genreHit{Genre: genre, rank: genreRank})
		}
	}
	slices.SortFunc(matched, compareGenreHits)
	return matched
}

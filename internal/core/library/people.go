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

// Genres are the genres with a film, series or other video of kind (ListFilms, ListSeries, ListOther,
// or ListAny for every kind) that reach reaches, each with how many, by name.
func (browser *Browser) Genres(ctx context.Context, reach auth.Reach, kind string) ([]Genre, error) {
	if kind == ListAny {
		kind = ""
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	rows, err := browser.database.Read().ListGenresWithCounts(ctx, sqlcgen.ListGenresWithCountsParams{
		Kind: kind, IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
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
// Limit hits in each group from Offset, or of one group alone when Group names one.
type GroupedQuery struct {
	Reach  auth.Reach
	Query  string
	Group  string // "" for every group
	Offset int64
	Limit  int64
}

// GroupedHits is a grouped search's page: each group's hits, the best first, and whether each has
// more after them.
type GroupedHits struct {
	Titles        []store.SearchHit
	People        []store.PersonHit
	Genres        []Genre
	HasMoreTitles bool
	HasMorePeople bool
	HasMoreGenres bool
}

// SearchGrouped finds titles, people and genres by what was typed, from its first letter
// (search-by-more-2026-11-04.md §4): titles and people through the index that can answer it (D48),
// the substring route for a query under three characters; genres by their folded names. No group is
// capped but by its page.
func (browser *Browser) SearchGrouped(ctx context.Context, query GroupedQuery) (GroupedHits, error) {
	hits := GroupedHits{Titles: []store.SearchHit{}, People: []store.PersonHit{}, Genres: []Genre{}}
	plan := textindex.PlanQuery(query.Query)
	if plan.IndexQuery == "" {
		return hits, nil
	}
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	// One more than the page, so that whether there are more needs no count.
	indexQuery := store.SearchQuery{
		Route: store.SearchRoute(plan.Route), IndexQuery: plan.IndexQuery, Limit: query.Limit + 1,
		Offset: query.Offset, IsEveryLibrary: isEveryLibrary, LibraryIDsJSON: libraryIDsJSON,
	}
	var err error
	if query.Group == "" || query.Group == GroupTitles {
		if hits.Titles, err = browser.database.FindTitles(ctx, indexQuery); err != nil {
			return GroupedHits{}, fmt.Errorf("search titles: %w", err)
		}
		hits.Titles, hits.HasMoreTitles = pageOf(hits.Titles, query.Limit)
	}
	if query.Group == "" || query.Group == GroupPeople {
		if hits.People, err = browser.database.FindPeople(ctx, indexQuery); err != nil {
			return GroupedHits{}, fmt.Errorf("search people: %w", err)
		}
		hits.People, hits.HasMorePeople = pageOf(hits.People, query.Limit)
	}
	if query.Group == "" || query.Group == GroupGenres {
		genres, genresErr := browser.Genres(ctx, query.Reach, ListAny)
		if genresErr != nil {
			return GroupedHits{}, genresErr
		}
		matched := matchGenres(genres, textindex.Fold(query.Query))
		matched = matched[min(int64(len(matched)), query.Offset):]
		hits.Genres, hits.HasMoreGenres = pageOf(matched, query.Limit)
	}
	return hits, nil
}

// pageOf is the first limit of hits, and whether there were more.
func pageOf[Hit any](hits []Hit, limit int64) ([]Hit, bool) {
	if int64(len(hits)) > limit {
		return hits[:limit], true
	}
	return hits, false
}

// matchGenres are the genres whose folded names hold folded, the most relevant first: those that
// begin with it, then those with a word that does, then the rest; within each, the most titles
// first, then by name.
func matchGenres(genres []Genre, folded string) []Genre {
	folded = strings.TrimSpace(folded)
	if folded == "" {
		return []Genre{}
	}
	rank := func(genre Genre) int {
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
	matched := []Genre{}
	for _, genre := range genres {
		if rank(genre) >= 0 {
			matched = append(matched, genre)
		}
	}
	slices.SortStableFunc(matched, func(first, second Genre) int {
		return cmp.Or(cmp.Compare(rank(first), rank(second)), cmp.Compare(second.TitleCount, first.TitleCount),
			cmp.Compare(first.Name, second.Name))
	})
	return matched
}

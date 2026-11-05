// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// creditedShelf is a shelf of films with genres, years and people, as the identification pass
// keeps them (step 0067): Metropolis (1995, Crime and Drama; Ansel Vardo as Mira Holloway-Pent, directed by
// Oren Talisk), The Boarder (1999, Drama; Ansel Vardo, Oren Talisk), Scarlet Wren (1983, Crime; Al
// Vardo) and Up (2009, Animation), and a film with no live file crediting Ansel Vardo.
type creditedShelf struct {
	*shelf
	films   map[string]int64
	people  map[string]int64
	genres  map[string]int64
	userID  int64
	gonePal int64 // a person only the film with no live file credits
}

func newCreditedShelf(t *testing.T) *creditedShelf {
	t.Helper()
	world := &creditedShelf{
		shelf: newShelf(t), films: map[string]int64{}, people: map[string]int64{}, genres: map[string]int64{},
	}
	world.scan([]nodeproto.ScanRecord{
		observedRecord("h.mkv", "h"), observedRecord("i.mkv", "i"), observedRecord("s.mkv", "s"),
		observedRecord("u.mkv", "u"),
	}, true)
	for name, path := range map[string]string{
		"Metropolis": "h.mkv", "The Boarder": "i.mkv", "Scarlet Wren": "s.mkv", "Up": "u.mkv", "Gone": "gone.mkv",
	} {
		world.films[name] = world.fileAsFilm(name, path)
	}
	world.write(world.dateFilms)
	world.write(world.addGenres)
	world.write(world.addPeople)
	world.write(world.creditPeople)
	world.write(func(queries *sqlcgen.Queries) error {
		var err error
		world.userID, err = queries.InsertUser(t.Context(), sqlcgen.InsertUserParams{
			Name: "viewer", RoleID: 1, CreatedAt: 1,
		})
		if err != nil {
			return err
		}
		return queries.MarkWorksWatched(t.Context(), sqlcgen.MarkWorksWatchedParams{
			UserID: world.userID, MarkedAt: 2, WorkIdsJSON: fmt.Sprintf("[%d]", world.films["Metropolis"]),
		})
	})
	world.gonePal = world.people["Ghost Actor"]
	return world
}

// dateFilms gives each film its year.
func (world *creditedShelf) dateFilms(queries *sqlcgen.Queries) error {
	for name, year := range map[string]int64{"Metropolis": 1995, "The Boarder": 1999, "Scarlet Wren": 1983, "Up": 2009} {
		if err := queries.UpdateWorkResolved(world.t.Context(), sqlcgen.UpdateWorkResolvedParams{
			Kind: ListFilms, Year: new(year), IdentificationState: new("identified"), ID: world.films[name],
		}); err != nil {
			return err
		}
	}
	return nil
}

// addGenres adds the genres and files each film in its own.
func (world *creditedShelf) addGenres(queries *sqlcgen.Queries) error {
	ctx := world.t.Context()
	for _, name := range []string{"Crime", "Drama", "Animation"} {
		err := queries.InsertGenreIfAbsent(ctx, sqlcgen.InsertGenreIfAbsentParams{
			Name: name, NameKey: textindex.Fold(name),
		})
		if err != nil {
			return err
		}
		if world.genres[name], err = queries.GetGenreIDByKey(ctx, textindex.Fold(name)); err != nil {
			return err
		}
	}
	for film, genres := range map[string][]string{
		"Metropolis": {"Crime", "Drama"}, "The Boarder": {"Drama"}, "Scarlet Wren": {"Crime"}, "Up": {"Animation"},
		"Gone": {"Crime"},
	} {
		for ordinal, genre := range genres {
			if err := queries.InsertItemGenre(ctx, sqlcgen.InsertItemGenreParams{
				ItemID: world.films[film], GenreID: world.genres[genre], Ordinal: int64(ordinal),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// addPeople adds the people, each an item with a name and a picture.
func (world *creditedShelf) addPeople(queries *sqlcgen.Queries) error {
	ctx := world.t.Context()
	for _, name := range []string{"Ansel Vardo", "Oren Talisk", "Ghost Actor"} {
		id, err := queries.InsertItem(ctx, sqlcgen.InsertItemParams{Kind: "person", AddedAt: 1})
		if err != nil {
			return err
		}
		err = queries.InsertPerson(ctx, sqlcgen.InsertPersonParams{
			ID: id, NameKey: textindex.Fold(name), PictureRef: new("/" + name + ".jpg"),
		})
		if err != nil {
			return err
		}
		err = queries.UpsertOriginalText(ctx, sqlcgen.UpsertOriginalTextParams{
			ItemID: id, Language: "und", Name: name, SearchAlternates: new(textindex.Fold(name)),
		})
		if err != nil {
			return err
		}
		world.people[name] = id
	}
	return nil
}

// creditPeople credits the people on the films.
func (world *creditedShelf) creditPeople(queries *sqlcgen.Queries) error {
	for _, credit := range []struct {
		film, role, person, character string
		ordinal                       int64
	}{
		{"Metropolis", RoleCast, "Ansel Vardo", "Mira Holloway-Pent", 0},
		{"Metropolis", RoleDirector, "Oren Talisk", "", 0},
		{"Metropolis", RoleWriter, "Oren Talisk", "", 0},
		{"The Boarder", RoleCast, "Ansel Vardo", "Edda Quarrin", 0},
		{"The Boarder", RoleDirector, "Oren Talisk", "", 0},
		{"Scarlet Wren", RoleCast, "Ansel Vardo", "Silas Morrow-Fenn", 0},
		{"Gone", RoleCast, "Ansel Vardo", "", 0},
		{"Gone", RoleCast, "Ghost Actor", "", 1},
	} {
		if err := queries.InsertCredit(world.t.Context(), sqlcgen.InsertCreditParams{
			ItemID: world.films[credit.film], Job: credit.role, Ordinal: credit.ordinal,
			PersonID: world.people[credit.person], Character: ptr.NilIfZero(credit.character),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (world *creditedShelf) write(change func(queries *sqlcgen.Queries) error) {
	world.t.Helper()
	if err := world.database.Write(world.t.Context(), change); err != nil {
		world.t.Fatal(err)
	}
}

// The list's filters, genre, person, decade and unwatched, each keep only the titles they name, and
// combine with each other and with a query (search-by-more-2026-11-04.md §4); a film with no live
// file is listed by none.
func TestList_FiltersByGenrePersonDecadeAndUnwatchedCombine(t *testing.T) {
	t.Parallel()
	world := newCreditedShelf(t)
	list := func(query ListQuery) []string {
		t.Helper()
		query.Reach, query.Kind, query.Sort, query.Limit, query.UserID = auth.EveryLibrary(), ListFilms, SortName,
			10, world.userID
		page, err := world.browser.List(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		return entryNames(page)
	}
	for name, test := range map[string]struct {
		query ListQuery
		want  []string
	}{
		"genre":               {ListQuery{GenreID: world.genres["Crime"]}, []string{"Metropolis", "Scarlet Wren"}},
		"person":              {ListQuery{PersonID: world.people["Oren Talisk"]}, []string{"Metropolis", "The Boarder"}},
		"decade":              {ListQuery{Decade: 1990}, []string{"Metropolis", "The Boarder"}},
		"unwatched":           {ListQuery{IsUnwatched: true}, []string{"The Boarder", "Scarlet Wren", "Up"}},
		"person and decade":   {ListQuery{PersonID: world.people["Ansel Vardo"], Decade: 1980}, []string{"Scarlet Wren"}},
		"genre and unwatched": {ListQuery{GenreID: world.genres["Drama"], IsUnwatched: true}, []string{"The Boarder"}},
		"query and genre":     {ListQuery{Query: "the", GenreID: world.genres["Drama"]}, []string{"The Boarder"}},
		"nothing kept":        {ListQuery{GenreID: world.genres["Animation"], Decade: 1980}, nil},
	} {
		if got := list(test.query); !slices.Equal(got, test.want) {
			t.Errorf("%s: %v; want %v", name, got, test.want)
		}
	}
}

// A film's page carries its genres in its source's order and its people in billing order within
// each role, each with their person's id, a person an item of their own (D186).
func TestTitle_CarriesItsGenresAndCredits(t *testing.T) {
	t.Parallel()
	world := newCreditedShelf(t)
	detail, err := world.browser.Title(t.Context(), auth.EveryLibrary(), world.userID, world.films["Metropolis"])
	if err != nil {
		t.Fatal(err)
	}
	people := detail.People
	if got := []string{people.Genres[0].Name, people.Genres[1].Name}; len(people.Genres) != 2 ||
		!slices.Equal(got, []string{"Crime", "Drama"}) {
		t.Errorf("genres %+v; want Crime, Drama", people.Genres)
	}
	wantCast := []CreditedPerson{{
		PersonID: world.people["Ansel Vardo"], Name: "Ansel Vardo", Character: "Mira Holloway-Pent",
		PictureRef: "/Ansel Vardo.jpg",
	}}
	wantMann := []CreditedPerson{{
		PersonID: world.people["Oren Talisk"], Name: "Oren Talisk", PictureRef: "/Oren Talisk.jpg",
	}}
	if !slices.Equal(people.Cast, wantCast) || !slices.Equal(people.Directors, wantMann) ||
		!slices.Equal(people.Writers, wantMann) {
		t.Errorf("credits %+v; want Ansel Vardo as Mira Holloway-Pent, directed and written by Oren Talisk", people)
	}
}

// A person's page is the person, their name and picture; the items crediting them are a list's,
// which says their roles on each (D425 §7). A person no reached title credits is no person to them
// (D243 §4).
func TestPerson_ListsTheReachedTitlesCreditingThem(t *testing.T) {
	t.Parallel()
	world := newCreditedShelf(t)
	vardo, err := world.browser.Person(t.Context(), auth.EveryLibrary(), world.people["Ansel Vardo"])
	if err != nil {
		t.Fatal(err)
	}
	if vardo.Name != "Ansel Vardo" || vardo.PictureRef != "/Ansel Vardo.jpg" {
		t.Errorf("Ansel Vardo's page %q %q; want his name and picture", vardo.Name, vardo.PictureRef)
	}
	ids := []int64{world.films["The Boarder"], world.films["Metropolis"], world.films["Scarlet Wren"]}
	credits, err := world.browser.CreditsOn(t.Context(), world.people["Ansel Vardo"], ids)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, id := range ids {
		roles := credits[id]
		if len(roles) == 0 {
			t.Fatalf("no credit on %d; want his part", id)
		}
		titles = append(titles, fmt.Sprintf("%d %s %s", id, roles[0].Role, roles[0].Character))
	}
	want := []string{
		fmt.Sprintf("%d cast Edda Quarrin", ids[0]), fmt.Sprintf("%d cast Mira Holloway-Pent", ids[1]),
		fmt.Sprintf("%d cast Silas Morrow-Fenn", ids[2]),
	}
	if !slices.Equal(titles, want) {
		t.Errorf("Ansel Vardo's credits %v; want %v", titles, want)
	}
	if len(credits) != len(ids) {
		t.Errorf("credits on %d items; want the %d named alone", len(credits), len(ids))
	}
	for name, reach := range map[string]auth.Reach{
		"a person only a film with no live file credits": auth.EveryLibrary(),
		"a person in a library not reached":              auth.LibrariesOnly(99),
	} {
		personID := world.gonePal
		if name == "a person in a library not reached" {
			personID = world.people["Ansel Vardo"]
		}
		if _, err := world.browser.Person(t.Context(), reach, personID); !errors.Is(err, ErrNoItem) {
			t.Errorf("%s: %v; want ErrNoItem", name, err)
		}
	}
}

// The genres listed are those a reached title is in, with how many; a genre only a film with no
// live file is in counts it not.
func TestGenres_CountTheReachedTitles(t *testing.T) {
	t.Parallel()
	world := newCreditedShelf(t)
	genres, err := world.browser.Genres(t.Context(), auth.EveryLibrary(), ListFilms)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, genre := range genres {
		got = append(got, fmt.Sprintf("%s %d", genre.Name, genre.TitleCount))
	}
	if want := []string{"Animation 1", "Crime 2", "Drama 2"}; !slices.Equal(got, want) {
		t.Errorf("genres %v; want %v", got, want)
	}
	if none, err := world.browser.Genres(t.Context(), auth.LibrariesOnly(99), ListAny); err != nil || len(none) != 0 {
		t.Errorf("genres of a library not reached %v, %v; want none", none, err)
	}
}

// searchGrouped is query's hits, of every library.
func (world *creditedShelf) searchGrouped(query GroupedQuery) GroupedHits {
	world.t.Helper()
	query.Reach = auth.EveryLibrary()
	hits, err := world.browser.SearchGrouped(world.t.Context(), query)
	if err != nil {
		world.t.Fatal(err)
	}
	return hits
}

// groupedNames are the names of each group's hits.
func groupedNames(hits GroupedHits) (titles, people, genres []string) {
	for _, hit := range hits.Titles {
		titles = append(titles, hit.Name)
	}
	for _, hit := range hits.People {
		people = append(people, hit.Name)
	}
	for _, hit := range hits.Genres {
		genres = append(genres, hit.Name)
	}
	return titles, people, genres
}

// A grouped search answers from the first letter with titles, people and genres, each the best
// first (search-by-more-2026-11-04.md §4); a person only a film with no live file credits is not
// found.
func TestSearchGrouped_FindsTitlesPeopleAndGenresFromTheFirstLetter(t *testing.T) {
	t.Parallel()
	world := newCreditedShelf(t)
	titles, people, genres := groupedNames(world.searchGrouped(GroupedQuery{Query: "a", Limit: 10}))
	if !slices.Equal(titles, []string{"Metropolis", "Scarlet Wren"}) ||
		!slices.Equal(people, []string{"Ansel Vardo", "Oren Talisk"}) ||
		!slices.Equal(genres, []string{"Animation", "Drama"}) {
		t.Errorf("a: titles %v, people %v, genres %v; want Metropolis, Scarlet Wren; Ansel Vardo, Oren Talisk; "+
			"Animation (it begins so), then Drama", titles, people, genres)
	}
	if _, ghosts, _ := groupedNames(world.searchGrouped(GroupedQuery{Query: "ghost", Limit: 10})); len(ghosts) != 0 {
		t.Errorf("ghost found %v; want no one, whom only a film with no live file credits", ghosts)
	}
	titles, people, _ = groupedNames(world.searchGrouped(GroupedQuery{Query: "vardo", Limit: 10}))
	if len(titles) != 0 || !slices.Equal(people, []string{"Ansel Vardo"}) {
		t.Errorf("vardo: titles %v, people %v; want Ansel Vardo by the words index, no title", titles, people)
	}
}

// A grouped search's group is paged with no cap but its page: one group alone, from an offset,
// saying whether more follow (search-by-more-2026-11-04.md §4).
func TestSearchGrouped_PagesOneGroup(t *testing.T) {
	t.Parallel()
	world := newCreditedShelf(t)
	first := world.searchGrouped(GroupedQuery{Query: "a", Group: GroupPeople, Limit: 1})
	rest := world.searchGrouped(GroupedQuery{Query: "a", Group: GroupPeople, Offset: 1, Limit: 1})
	if _, people, _ := groupedNames(first); !slices.Equal(people, []string{"Ansel Vardo"}) || !first.HasMorePeople ||
		len(first.Titles) != 0 {
		t.Errorf("the first page of people %v (more %v, titles %v); want Ansel Vardo alone, more after", people,
			first.HasMorePeople, first.Titles)
	}
	if _, people, _ := groupedNames(rest); !slices.Equal(people, []string{"Oren Talisk"}) || rest.HasMorePeople {
		t.Errorf("the next page of people %v (more %v); want Oren Talisk, the last", people, rest.HasMorePeople)
	}
}

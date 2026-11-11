// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// searchedShelf is a shelf of films whose names a query matches, some, and does not, others.
func searchedShelf(t *testing.T) *shelf {
	t.Helper()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("h1.mkv", "h1"), observedRecord("h2.mkv", "h2"), observedRecord("h3.mkv", "h3"),
		observedRecord("u.mkv", "u"), observedRecord("s1.mkv", "s1"), observedRecord("s2.mkv", "s2"),
		observedRecord("o.mkv", "o"),
	}, true)
	world.fileAsFilm("Metropolis", "h1.mkv")
	world.fileAsFilm("Metropolises", "h2.mkv")
	world.fileAsFilm("Metropolis Wave", "h3.mkv")
	world.fileAsFilm("Up", "u.mkv")
	world.fileAsFilm("十字路", "s1.mkv")
	world.fileAsFilm("千と千尋の物語", "s2.mkv")
	world.fileAsFilm("Ordet", "o.mkv")
	world.fileAsFilm("Metropolis Gone", "gone.mkv") // no live file: listed by no list, searched or not
	return world
}

// A list with a query holds only the titles whose names match it, as search matches them (D48), in
// the list's own order, a page at a time, its cursor keeping the query's criterion (one listing API
// with search a criterion of it: the owner, 2026-10-30).
func TestList_AQueryListsOnlyWhatMatchesInTheListsOwnOrderAndPages(t *testing.T) {
	t.Parallel()
	world := searchedShelf(t)
	list := func(query ListQuery) Page {
		t.Helper()
		query.Reach, query.Kinds, query.Query = auth.EveryLibrary(), []string{ListFilms}, "heat"
		page, err := world.browser.List(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	first := list(ListQuery{Sort: SortName, Limit: 2})
	if !slices.Equal(entryNames(first), []string{"Metropolis", "Metropolis Wave"}) || first.NextCursor == "" {
		t.Fatalf("the first page of heat by name = %v (cursor %q); want Metropolis, Metropolis Wave and a next page",
			entryNames(first), first.NextCursor)
	}
	rest := list(ListQuery{Sort: SortName, Limit: 2, Cursor: first.NextCursor})
	if !slices.Equal(entryNames(rest), []string{"Metropolises"}) || rest.NextCursor != "" {
		t.Errorf("the next page = %v (cursor %q); want Metropolises alone, and the last: Up and Ordet are not "+
			"Metropolis's", entryNames(rest), rest.NextCursor)
	}
	if page := list(ListQuery{Sort: SortName, Order: OrderDescending, Limit: 10}); !slices.Equal(entryNames(page),
		[]string{"Metropolises", "Metropolis Wave", "Metropolis"}) {
		t.Errorf("heat by name, Z to A = %v; want the three, the other way", entryNames(page))
	}
	if page := list(ListQuery{Sort: SortAdded, Limit: 10}); len(page.Entries) != 3 {
		t.Errorf("heat newest first = %v; want the three that match", entryNames(page))
	}
	if page := list(ListQuery{Sort: SortReleased, Limit: 10}); len(page.Entries) != 3 {
		t.Errorf("heat by release = %v; want the three that match, undated as they are", entryNames(page))
	}
	if page := list(ListQuery{Sort: SortName, From: "Heath", Limit: 10}); !slices.Equal(entryNames(page),
		[]string{"Metropolises"}) {
		t.Errorf("heat from Heath = %v; want Metropolises, from where From starts", entryNames(page))
	}
}

// A query in any script matches through the index that can answer it (D48): the trigram index, or
// the folded substring for one too short for it; a query that plans to nothing, or matches nothing,
// lists nothing, and its letters are none.
func TestList_AQueryInAnyScriptAndAQueryThatMatchesNothing(t *testing.T) {
	t.Parallel()
	world := searchedShelf(t)
	for query, want := range map[string][]string{
		"十字": {"千と千尋の物語", "十字路"}, // the trigram index, in the locale's order
		"神隠":   {"十字路"},            // too short for it: the folded substring
		"up":   {"Up"},
		"   ":  nil, // plans to nothing
		"...":  nil,
		"zzz":  nil, // matches nothing
	} {
		page, err := world.browser.List(t.Context(), ListQuery{
			Reach: auth.EveryLibrary(), Kinds: ListKinds(), Sort: SortName, Limit: 10, Query: query,
		})
		if err != nil || !slices.Equal(entryNames(page), want) || page.Entries == nil {
			t.Errorf("every kind's titles matching %q = %v, %v; want %v", query, entryNames(page), err, want)
		}
	}
	for query, want := range map[string][]string{
		"heat": {"H"}, "ordet": {"O"}, "u": {"U"}, "zzz": nil, "   ": nil,
	} {
		letters, err := world.browser.Letters(t.Context(), ListQuery{
			Reach: auth.EveryLibrary(), Kinds: []string{ListFilms}, Sort: SortName, Query: query,
		})
		if err != nil || !slices.Equal(letters, want) || letters == nil {
			t.Errorf("the letters of the films matching %q = %v, %v; want %v", query, letters, err, want)
		}
	}
}

// A list's first page holds the whole list's total, of every sort, its query and filters counted,
// so a client sizes the list before its other pages come (the owner, 2026-11-11); its other pages,
// and a page from a letter, hold none.
func TestList_TheFirstPageHoldsTheWholeListsTotal(t *testing.T) {
	t.Parallel()
	world := searchedShelf(t)
	list := func(query ListQuery) Page {
		t.Helper()
		query.Reach, query.Kinds = auth.EveryLibrary(), []string{ListFilms}
		page, err := world.browser.List(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	first := list(ListQuery{Sort: SortName, Limit: 2})
	if first.Total != 7 {
		t.Errorf("the first page's total = %d; want the 7 films with a live file, not Metropolis Gone", first.Total)
	}
	if rest := list(ListQuery{Sort: SortName, Limit: 2, Cursor: first.NextCursor}); rest.Total != 0 {
		t.Errorf("the next page's total = %d; want none, on the first page alone", rest.Total)
	}
	if page := list(ListQuery{Sort: SortName, From: "H", Limit: 2}); page.Total != 0 {
		t.Errorf("a page from H's total = %d; want none: it is not the list's start", page.Total)
	}
	for _, sort := range []string{SortAdded, SortReleased} {
		if page := list(ListQuery{Sort: sort, Query: "heat", Limit: 2}); page.Total != 3 {
			t.Errorf("heat by %s: total = %d; want the 3 that match", sort, page.Total)
		}
	}
	if page := list(ListQuery{Sort: SortName, Query: "zzzz", Limit: 2}); page.Total != 0 {
		t.Errorf("a query matching nothing: total = %d; want 0", page.Total)
	}
	page, err := world.browser.List(t.Context(), ListQuery{
		Reach: auth.EveryLibrary(), Kinds: []string{ListFilms, ListSeries, ListFilms}, Sort: SortName, Limit: 2,
	})
	if err != nil || page.Total != 7 {
		t.Errorf("films and series, films named twice: total = %d, %v; want 7, each kind counted once", page.Total, err)
	}
}

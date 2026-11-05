// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// filmsByName is the films in the order of the list by name.
func filmsByName(t *testing.T, lib *testLibrary) []string {
	t.Helper()
	browser := library.NewBrowser(lib.database, lib.settings)
	listed, err := browser.List(t.Context(), library.ListQuery{
		Reach: auth.EveryLibrary(), Kind: library.ListFilms, Sort: library.SortName, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, entry := range listed.Entries {
		order = append(order, entry.Name)
	}
	return order
}

// D402: a film sorts without its leading article, its name shown whole; an NFO's sort title wins.
func TestPass_AFilmSortsWithoutItsLeadingArticleAndAnNFOSortTitleWins(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	lib.scan(
		videoRecord("The Gold Rush (1999).mkv"), videoRecord("A Woman of Paris (2001).mkv"),
		videoRecord("Metropolis (1995).mkv"), videoRecord("Destiny (1998).mkv"), videoRecord("Wing (1979).mkv"),
		videoRecord("Intolerance (1979).mkv"), videoRecord("The (2020).mkv"),
		videoRecord("The Lodger (1973).mkv"),
		nfoRecord("The Lodger (1973).nfo", "<movie><title>The Lodger</title><year>1973</year>"+
			"<sorttitle>Zzz Last</sorttitle></movie>"),
	)
	got := filmsByName(t, lib)
	want := []string{"Wing", "Intolerance", "A Woman of Paris", "Metropolis", "The Gold Rush", "The", "Destiny", "The Lodger"}
	if !slices.Equal(got, want) {
		t.Errorf("films by name = %v; want %v (The Gold Rush among M, The Lodger last)", got, want)
	}
}

// D402: every stored key is made again from its name when a build keys otherwise, so a library
// scanned before the change is filed by it at the next start with no manual step; an NFO's sort
// title is kept through it.
func TestRefreshSortAndSearchKeys_AKeyMadeWithTheArticleIsMadeAgainWithout(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	lib.scan(
		videoRecord("The Gold Rush (1999).mkv"), videoRecord("Metropolis (1995).mkv"), videoRecord("Destiny (1998).mkv"),
		videoRecord("Mission (2000).mkv"), videoRecord("The Lodger (1973).mkv"),
		nfoRecord("The Lodger (1973).nfo", "<movie><title>The Lodger</title><year>1973</year>"+
			"<sorttitle>Zzz Last</sorttitle></movie>"),
	)
	// What the build before D402 stored: The Gold Rush's key of its whole name, which is Metropolis's last
	// place, and The Lodger's key of its name, not its sort title.
	storetest.ExecRaw(t, lib.database.Path(),
		"UPDATE sort_key SET collation_key = X'FFFFFFFE' "+
			"WHERE item_id = (SELECT item_id FROM item_text WHERE name = 'The Gold Rush')",
		"UPDATE sort_key SET collation_key = X'01' "+
			"WHERE item_id = (SELECT item_id FROM item_text WHERE name = 'The Lodger')")
	if got := filmsByName(t, lib); !slices.Equal(got, []string{"The Lodger", "Metropolis", "Mission", "Destiny", "The Gold Rush"}) {
		t.Fatalf("films by name under the older keys = %v; the setup did not take", got)
	}
	if rewritten, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil || rewritten != 2 {
		t.Fatalf("RefreshSortAndSearchKeys = %d, %v; want The Gold Rush and The Lodger keyed again", rewritten, err)
	}
	if got := filmsByName(t, lib); !slices.Equal(got, []string{"Metropolis", "The Gold Rush", "Mission", "Destiny", "The Lodger"}) {
		t.Errorf("films by name after the refresh = %v", got)
	}
	if rewritten, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil || rewritten != 0 {
		t.Errorf("a second refresh rewrote %d (%v); every key was current", rewritten, err)
	}
}

// D402: with a German sort locale, Der letzte Mann sorts under U.
func TestPass_InGermanADeSortsWithoutItsArticle(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	lib.scan(videoRecord("Der letzte Mann (2004).mkv"), videoRecord("Ulme (2000).mkv"), videoRecord("Destiny (1998).mkv"),
		videoRecord("Der Golem (1981).mkv"))
	settingstest.Set(t, lib.settings, settings.SortLocale, `"de"`)
	if _, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := filmsByName(t, lib); !slices.Equal(got, []string{"Der Golem", "Ulme", "Der letzte Mann", "Destiny"}) {
		t.Errorf("in German = %v; want Der letzte Mann under U and Der Golem under B", got)
	}
}

// seriesByName is the series in the order of the list by name.
func seriesByName(t *testing.T, lib *testLibrary) []string {
	t.Helper()
	browser := library.NewBrowser(lib.database, lib.settings)
	listed, err := browser.List(t.Context(), library.ListQuery{
		Reach: auth.EveryLibrary(), Kind: library.ListSeries, Sort: library.SortName, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, entry := range listed.Entries {
		order = append(order, entry.Name)
	}
	return order
}

// D402: a series' tvshow.nfo sort title wins over its name, as a film's NFO's does.
func TestPass_ASeriesTvshowNFOSortTitleWins(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "nfo")
	lib.scan(
		nfoRecord("Alpha/tvshow.nfo", `<tvshow><title>Alpha</title><sorttitle>Zzz Last</sorttitle></tvshow>`),
		videoRecord("Alpha/Season 01/Alpha - S01E01.mkv"),
		nfoRecord("Beta/tvshow.nfo", `<tvshow><title>Beta</title></tvshow>`),
		videoRecord("Beta/Season 01/Beta - S01E01.mkv"),
	)
	if got := seriesByName(t, lib); !slices.Equal(got, []string{"Beta", "Alpha"}) {
		t.Errorf("series by name = %v; want Alpha last, by its sort title", got)
	}
}

// D402: a sort title the NFO no longer gives is forgotten even when the key it made is the key the
// name makes too, so a later rekey under another locale does not bring the old sort title back.
func TestPass_ASortTitleDroppedFromTheNFOIsForgotten(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	withSortTitle := nfoRecord("The Gold Rush (1999).nfo",
		"<movie><title>The Gold Rush</title><year>1999</year><sorttitle>Gold Rush</sorttitle></movie>")
	lib.scan(videoRecord("The Gold Rush (1999).mkv"), videoRecord("Tango (2000).mkv"), videoRecord("Mabel (1981).mkv"),
		withSortTitle)
	without := nfoRecord("The Gold Rush (1999).nfo", "<movie><title>The Gold Rush</title><year>1999</year></movie>")
	without.Sidecar.ModifiedAt = 2
	lib.scan(videoRecord("The Gold Rush (1999).mkv"), videoRecord("Tango (2000).mkv"), videoRecord("Mabel (1981).mkv"),
		without)
	// In German "The" is no article, so the name alone keys The Gold Rush under T, after Tango.
	settingstest.Set(t, lib.settings, settings.SortLocale, `"de"`)
	if _, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := filmsByName(t, lib); !slices.Equal(got, []string{"Mabel", "Tango", "The Gold Rush"}) {
		t.Errorf("films by name in German = %v; want The Gold Rush under T, its old sort title forgotten", got)
	}
}

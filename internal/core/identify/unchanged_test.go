// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A pass over a library nothing changed in writes nothing: no title, text, sort key, index or
// filing written again as it was (D303, DL9; the access audit's A2).
func TestPass_AgainOverAnUnchangedLibraryWritesNothing(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	lib.scan(
		nfoRecord("Metropolis (1927)/Metropolis (1927).nfo",
			`<movie><title>Metropolis</title><year>1927</year></movie>`),
		videoRecord("Metropolis (1927)/Metropolis (1927).mkv"),
		videoRecord("Metropolis (1927)/Metropolis (1927)-trailer.mkv"),
		videoRecord("Arkon (1979)/Arkon (1979).mkv"),
		nfoRecord("Test Show/tvshow.nfo", `<tvshow><title>Test Show</title><year>2024</year></tvshow>`),
		videoRecord("Test Show/Season 01/Test Show - S01E01.mkv"),
		videoRecord("Test Show/Season 01/Test Show - S01E02.mkv"),
	)
	before, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lib.pass.IdentifyLibrary(t.Context(), lib.libraryID); err != nil {
		t.Fatal(err)
	}
	after, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("a pass over an unchanged library changed %d rows; want none", after-before)
	}
}

// A pass over a matched library nothing changed in writes nothing either: what it resolves from
// the stored response is what it stored last time (D303, DL9).
func TestPass_AgainOverAnUnchangedMatchedLibraryWritesNothing(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Théo", 1994)] = []identify.Candidate{lib.provider.Add(theo)}
	const rel = "Théo (1994)/Théo (1994).mkv"
	lib.scan(videoRecord(rel))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s, want matched", outcome)
	}
	before, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lib.pass.IdentifyLibrary(t.Context(), lib.libraryID); err != nil {
		t.Fatal(err)
	}
	after, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("a pass over an unchanged matched library changed %d rows; want none", after-before)
	}
}

// A pass over a series library with a special no number places writes nothing either: the special
// keeps its title rather than gaining a new one each pass, which left the old ones in its series,
// not on disk (the owner's NAS, 2026-10-16: 80,246 of them under one series).
func TestPass_AgainOverAnUnchangedLibraryWithAnUnnumberedSpecialWritesNothing(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "nfo")
	lib.scan(
		nfoRecord("Test Show/tvshow.nfo", `<tvshow><title>Test Show</title><year>2024</year></tvshow>`),
		videoRecord("Test Show/Season 01/Test Show - S01E01.mkv"),
		videoRecord("Test Show/Featurettes/The Making Of.mkv"),
	)
	before, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lib.pass.IdentifyLibrary(t.Context(), lib.libraryID); err != nil {
		t.Fatal(err)
	}
	after, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("a pass over an unchanged library with a special changed %d rows; want none", after-before)
	}
}

// A pass over a library holding an episode its name placed, as a guess, writes nothing either, an
// hour on: the episode keeps its mark and the time of the first guess, rather than being marked
// again each pass (D303, DL9; staging's rescan of its shows, 2026-11-01, which waited forever for
// its library to stop changing).
func TestPass_AgainOverAnUnchangedLibraryWithAnEpisodePlacedByItsNameWritesNothing(t *testing.T) {
	t.Parallel()
	lib := placedByNameLibrary(t)
	lib.clock.Advance(time.Hour)
	before, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lib.pass.IdentifyLibrary(t.Context(), lib.libraryID); err != nil {
		t.Fatal(err)
	}
	after, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("a pass over an unchanged library with an episode placed by its name changed %d rows; want none",
			after-before)
	}
	if placed := lib.titlesByPath()[placedByNameUnplaced]; !lib.isGuessed(placed.WorkID) {
		t.Error("the episode placed by its name is no longer marked a guess")
	}
}

// A title filed as an extra once a film appears to own it takes the kind "extra", even where nothing
// else about it changes, as when a person confirmed it as a film: kept a film, its listing was
// rewritten by the pass and the sort-key refresh in turn, every pass (D162; found by
// TestPass_OverRandomLibrariesAgainWritesNothingAndMakesNoTitleEachPass).
func TestPass_AConfirmedFilmFiledAsAnExtraTakesTheKindExtra(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	const making = "Metropolis (1927)/Featurettes/Making Metropolis.mkv"
	lib.scan(videoRecord(making))
	title := lib.titlesByPath()[making].WorkID
	if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.ConfirmWorkMatch(t.Context(), title)
	}); err != nil {
		t.Fatal(err)
	}
	lib.scan(videoRecord("Metropolis (1927)/Metropolis (1927).mkv"), videoRecord(making))
	kind := storetest.QueryRaw(t, lib.database.Path(), fmt.Sprintf("SELECT kind FROM work WHERE id = %d", title))
	if !slices.Equal(kind, []string{"extra"}) {
		t.Errorf("the title filed as an extra is of kind %v; want extra", kind)
	}
}

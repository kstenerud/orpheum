// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
)

// A pass over a library nothing changed in writes nothing: no title, text, sort key, index or
// filing written again as it was (D303, DL9; the access audit's A2).
func TestPass_AgainOverAnUnchangedLibraryWritesNothing(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "mixed", "nfo")
	lib.scan(
		nfoRecord("Metropolis (1995)/Metropolis (1995).nfo", `<movie><title>Metropolis</title><year>1995</year></movie>`),
		videoRecord("Metropolis (1995)/Metropolis (1995).mkv"),
		videoRecord("Metropolis (1995)/Metropolis (1995)-trailer.mkv"),
		videoRecord("Wing (1979)/Wing (1979).mkv"),
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
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Variety", 1994)] = []identify.Candidate{lib.provider.Add(leon)}
	const rel = "Variety (1994)/Variety (1994).mkv"
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
	lib := newTestLibrary(t, "shows", "nfo")
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

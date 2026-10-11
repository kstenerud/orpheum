// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// ambiguousTheGeneral is a library holding one film its search could not settle.
func ambiguousTheGeneral(t *testing.T) (*testLibrary, int64) {
	t.Helper()
	lib := newTestLibrary(t, "movies", "tmdb")
	theGeneral72 := lib.provider.Add(identify.Metadata{ID: "1", Name: "The General", Year: 1972})
	theGeneral02 := lib.provider.Add(identify.Metadata{ID: "2", Name: "The General", Year: 2002, Overview: "Soderbergh's."})
	lib.provider.Searches[identifytest.SearchKey("The General", 2003)] = []identify.Candidate{theGeneral72, theGeneral02}
	lib.scan(videoRecord("The General (2003).mkv"))
	if outcome := lib.matchFileTitle("The General (2003).mkv"); outcome != identify.OutcomeAmbiguous {
		t.Fatalf("outcome %s, want ambiguous", outcome)
	}
	return lib, lib.titlesByPath()["The General (2003).mkv"].TitleID
}

func TestChooser_ListsTheAmbiguousTitlesAndTheirCandidates(t *testing.T) {
	t.Parallel()
	lib, titleID := ambiguousTheGeneral(t)
	titles, err := lib.chooser.Items(t.Context(), auth.EveryLibrary(), identify.IdentificationAmbiguous)
	if err != nil || len(titles) != 1 || titles[0].ID != titleID || titles[0].Name != "The General" {
		t.Fatalf("Items(ambiguous) = %+v, %v", titles, err)
	}
	candidates, provider, err := lib.chooser.Candidates(t.Context(), titleID)
	if err != nil || provider != "tmdb" || len(candidates) != 2 || candidates[1].Year != 2002 {
		t.Errorf("Candidates = %+v from %q, %v", candidates, provider, err)
	}
}

func TestChooser_SearchesAgainWhenTheCacheLostTheCandidates(t *testing.T) {
	t.Parallel()
	lib, titleID := ambiguousTheGeneral(t)
	err := lib.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.DeleteMatchCandidates(t.Context(), titleID)
	})
	if err != nil {
		t.Fatal(err)
	}
	searches := len(lib.provider.Calls)
	candidates, _, err := lib.chooser.Candidates(t.Context(), titleID)
	if err != nil || len(candidates) != 2 || len(lib.provider.Calls) != searches+1 {
		t.Fatalf("Candidates after the cache lost them = %+v, %v, calls %v", candidates, err, lib.provider.Calls)
	}
	if kept, err := lib.cache.Read().ListMatchCandidates(t.Context(), titleID); err != nil || len(kept) != 2 {
		t.Errorf("the second search's results were not kept: %d, %v", len(kept), err)
	}
}

func TestChooser_AChosenMatchIsPermanent(t *testing.T) {
	t.Parallel()
	lib, titleID := ambiguousTheGeneral(t)
	if err := lib.chooser.Choose(t.Context(), titleID, "tmdb", "2"); err != nil {
		t.Fatal(err)
	}
	for _, when := range []string{"after choosing", "after another pass"} {
		file := lib.titlesByPath()["The General (2003).mkv"]
		if file.IdentificationState != identify.IdentificationConfirmed || file.Year == nil || *file.Year != 2002 ||
			file.Filing != identify.FilingListed {
			t.Errorf("%s: %+v; want the chosen film, confirmed", when, file)
		}
		lib.scan(videoRecord("The General (2003).mkv"))
	}
	if kept, err := lib.cache.Read().ListMatchCandidates(t.Context(), titleID); err != nil || len(kept) != 0 {
		t.Errorf("%d candidates kept after the choice (%v)", len(kept), err)
	}
}

func TestChooser_AChoiceWinsOverTheNFOAndReplacesAnEarlierOne(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo_then_tmdb")
	lib.provider.Add(heat)
	lib.provider.Add(leon)
	lib.scan(videoRecord("Metropolis (1995).mkv"),
		nfoRecord("Metropolis (1995).nfo", "<movie><title>Metropolis</title><year>1995</year><tmdbid>9019</tmdbid></movie>"))
	titleID := lib.titlesByPath()["Metropolis (1995).mkv"].TitleID
	for _, id := range []string{heat.ID, leon.ID} {
		if err := lib.chooser.Choose(t.Context(), titleID, "tmdb", id); err != nil {
			t.Fatal(err)
		}
	}
	lib.scan(videoRecord("Metropolis (1995).mkv"),
		nfoRecord("Metropolis (1995).nfo", "<movie><title>Metropolis</title><year>1995</year><tmdbid>9019</tmdbid></movie>"))
	file := lib.titlesByPath()["Metropolis (1995).mkv"]
	if titleName(file) != leon.Name || file.IdentificationState != identify.IdentificationConfirmed {
		t.Errorf("file %+v is %q; want the person's choice over the NFO", file, titleName(file))
	}
	ids, err := lib.database.Read().ListItemProviderIDs(t.Context(), titleID)
	if err != nil {
		t.Fatal(err)
	}
	var tmdb []string
	for _, row := range ids {
		if row.Provider == "tmdb" {
			tmdb = append(tmdb, row.ProviderID)
		}
	}
	if !slices.Equal(tmdb, []string{leon.ID}) {
		t.Errorf("TMDB ids %v, want only the last choice", tmdb)
	}
}

func TestChooser_AnOverrideHoldsUntilItIsRemoved(t *testing.T) {
	t.Parallel()
	lib, titleID := ambiguousTheGeneral(t)
	if err := lib.chooser.Choose(t.Context(), titleID, "tmdb", "2"); err != nil {
		t.Fatal(err)
	}
	person := storetest.InsertUser(t, lib.database, "alice", "admin")
	err := lib.chooser.SetOverride(t.Context(), titleID, identify.FieldName, "The General (Soderbergh)", person)
	if err != nil {
		t.Fatal(err)
	}
	lib.scan(videoRecord("The General (2003).mkv"))
	if got := titleName(lib.titlesByPath()["The General (2003).mkv"]); got != "The General (Soderbergh)" {
		t.Errorf("with the override, after another pass: %q", got)
	}
	if err := lib.chooser.ClearOverride(t.Context(), titleID, identify.FieldName); err != nil {
		t.Fatal(err)
	}
	if got := titleName(lib.titlesByPath()["The General (2003).mkv"]); got != "The General" {
		t.Errorf("with the override removed: %q, want the source's name", got)
	}
}

func TestChooser_RefusesWhatItCannotDo(t *testing.T) {
	t.Parallel()
	lib, titleID := ambiguousTheGeneral(t)
	ctx := t.Context()
	for _, check := range []struct {
		what string
		err  error
		want error
	}{
		{"no title", lib.chooser.Choose(ctx, 999, "tmdb", "2"), identify.ErrNoItem},
		{"no provider", lib.chooser.Choose(ctx, titleID, "omdb", "2"), identify.ErrUnknownProvider},
		{"no field", lib.chooser.SetOverride(ctx, titleID, "rating", "5", 1), identify.ErrUnknownField},
		{
			"a year that is no year", lib.chooser.SetOverride(ctx, titleID, identify.FieldYear, "soon", 1),
			identify.ErrBadValue,
		},
		{"an empty name", lib.chooser.SetOverride(ctx, titleID, identify.FieldName, "", 1), identify.ErrBadValue},
		{"no override", lib.chooser.ClearOverride(ctx, titleID, identify.FieldTagline), identify.ErrNoOverride},
	} {
		if !errors.Is(check.err, check.want) {
			t.Errorf("%s: %v, want %v", check.what, check.err, check.want)
		}
	}
}

func TestChooser_CorrectingAnAmbiguousTitlesNameSearchesItsCandidatesAgain(t *testing.T) {
	t.Parallel()
	lib, titleID := ambiguousTheGeneral(t)
	theGeneral72 := identify.Metadata{ID: "1", Name: "The General", Year: 1972}
	lib.provider.Searches[identifytest.SearchKey("The General", 1972)] = []identify.Candidate{lib.provider.Add(theGeneral72)}
	person := storetest.InsertUser(t, lib.database, "alice", "admin")
	if err := lib.chooser.SetOverride(t.Context(), titleID, identify.FieldYear, "1972", person); err != nil {
		t.Fatal(err)
	}
	candidates, _, err := lib.chooser.Candidates(t.Context(), titleID)
	if err != nil || len(candidates) != 1 || candidates[0].Year != 1972 {
		t.Errorf("Candidates after the year is corrected = %+v, %v; want the new search's (D223 §7)", candidates, err)
	}
}

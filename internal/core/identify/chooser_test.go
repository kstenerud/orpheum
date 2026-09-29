// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// ambiguousTheGeneral is a library holding one film its search could not settle.
func ambiguousTheGeneral(t *testing.T) (*world, int64) {
	t.Helper()
	lib := newWorld(t, "movies", "tmdb")
	theGeneral72 := lib.provider.add(Metadata{ID: "1", Name: "The General", Year: 1972})
	theGeneral02 := lib.provider.add(Metadata{ID: "2", Name: "The General", Year: 2002, Overview: "Soderbergh's."})
	lib.provider.searches[searchKey("The General", 2003)] = []Candidate{theGeneral72, theGeneral02}
	lib.scan(video("The General (2003).mkv"))
	if outcome := lib.match("The General (2003).mkv"); outcome != OutcomeAmbiguous {
		t.Fatalf("outcome %s, want ambiguous", outcome)
	}
	return lib, lib.files()["The General (2003).mkv"].TitleID
}

func TestChooser_ListsTheAmbiguousTitlesAndTheirCandidates(t *testing.T) {
	t.Parallel()
	lib, titleID := ambiguousTheGeneral(t)
	titles, err := lib.chooser.Items(t.Context(), identAmbiguous)
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
	err := lib.matcher.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.DeleteMatchCandidates(t.Context(), titleID)
	})
	if err != nil {
		t.Fatal(err)
	}
	searches := len(lib.provider.calls)
	candidates, _, err := lib.chooser.Candidates(t.Context(), titleID)
	if err != nil || len(candidates) != 2 || len(lib.provider.calls) != searches+1 {
		t.Fatalf("Candidates after the cache lost them = %+v, %v, calls %v", candidates, err, lib.provider.calls)
	}
	if kept, err := lib.matcher.cache.Read().ListMatchCandidates(t.Context(), titleID); err != nil || len(kept) != 2 {
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
		file := lib.files()["The General (2003).mkv"]
		if file.IdentificationState != identConfirmed || file.Year == nil || *file.Year != 2002 ||
			file.Filing != filingListed {
			t.Errorf("%s: %+v; want the chosen film, confirmed", when, file)
		}
		lib.scan(video("The General (2003).mkv"))
	}
	if kept, err := lib.matcher.cache.Read().ListMatchCandidates(t.Context(), titleID); err != nil || len(kept) != 0 {
		t.Errorf("%d candidates kept after the choice (%v)", len(kept), err)
	}
}

func TestChooser_AChoiceWinsOverTheNFOAndReplacesAnEarlierOne(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo_then_tmdb")
	lib.provider.add(heat)
	lib.provider.add(leon)
	lib.scan(video("Metropolis (1995).mkv"),
		nfo("Metropolis (1995).nfo", "<movie><title>Metropolis</title><year>1995</year><tmdbid>9019</tmdbid></movie>"))
	titleID := lib.files()["Metropolis (1995).mkv"].TitleID
	for _, id := range []string{heat.ID, leon.ID} {
		if err := lib.chooser.Choose(t.Context(), titleID, "tmdb", id); err != nil {
			t.Fatal(err)
		}
	}
	lib.scan(video("Metropolis (1995).mkv"),
		nfo("Metropolis (1995).nfo", "<movie><title>Metropolis</title><year>1995</year><tmdbid>9019</tmdbid></movie>"))
	if file := lib.files()["Metropolis (1995).mkv"]; name(file) != leon.Name || file.IdentificationState != identConfirmed {
		t.Errorf("file %+v is %q; want the person's choice over the NFO", file, name(file))
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
	if err := lib.chooser.SetOverride(t.Context(), titleID, fieldName, "The General (Soderbergh)", person); err != nil {
		t.Fatal(err)
	}
	lib.scan(video("The General (2003).mkv"))
	if got := name(lib.files()["The General (2003).mkv"]); got != "The General (Soderbergh)" {
		t.Errorf("with the override, after another pass: %q", got)
	}
	if err := lib.chooser.ClearOverride(t.Context(), titleID, fieldName); err != nil {
		t.Fatal(err)
	}
	if got := name(lib.files()["The General (2003).mkv"]); got != "The General" {
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
		{"no title", lib.chooser.Choose(ctx, 999, "tmdb", "2"), ErrNoItem},
		{"no provider", lib.chooser.Choose(ctx, titleID, "omdb", "2"), ErrUnknownProvider},
		{"no field", lib.chooser.SetOverride(ctx, titleID, "rating", "5", 1), ErrUnknownField},
		{"a year that is no year", lib.chooser.SetOverride(ctx, titleID, fieldYear, "soon", 1), ErrBadValue},
		{"an empty name", lib.chooser.SetOverride(ctx, titleID, fieldName, "", 1), ErrBadValue},
		{"no override", lib.chooser.ClearOverride(ctx, titleID, fieldTagline), ErrNoOverride},
	} {
		if !errors.Is(check.err, check.want) {
			t.Errorf("%s: %v, want %v", check.what, check.err, check.want)
		}
	}
}

func TestChooser_CorrectingAnAmbiguousTitlesNameSearchesItsCandidatesAgain(t *testing.T) {
	t.Parallel()
	lib, titleID := ambiguousTheGeneral(t)
	theGeneral72 := Metadata{ID: "1", Name: "The General", Year: 1972}
	lib.provider.searches[searchKey("The General", 1972)] = []Candidate{lib.provider.add(theGeneral72)}
	person := storetest.InsertUser(t, lib.database, "alice", "admin")
	if err := lib.chooser.SetOverride(t.Context(), titleID, fieldYear, "1972", person); err != nil {
		t.Fatal(err)
	}
	candidates, _, err := lib.chooser.Candidates(t.Context(), titleID)
	if err != nil || len(candidates) != 1 || candidates[0].Year != 1972 {
		t.Errorf("Candidates after the year is corrected = %+v, %v; want the new search's (D223 §7)", candidates, err)
	}
}

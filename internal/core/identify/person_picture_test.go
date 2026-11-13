// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// rowsAPassWrites is how many rows one pass over the library writes.
func (lib *testLibrary) rowsAPassWrites() int64 {
	lib.t.Helper()
	before, err := lib.database.RowsChanged(lib.t.Context())
	if err != nil {
		lib.t.Fatal(err)
	}
	if _, err = lib.pass.IdentifyLibrary(lib.t.Context(), lib.libraryID); err != nil {
		lib.t.Fatal(err)
	}
	after, err := lib.database.RowsChanged(lib.t.Context())
	if err != nil {
		lib.t.Fatal(err)
	}
	return after - before
}

// fileTwice files the library once, which may still write what its matches left to file, and again,
// which over the unchanged library must write nothing (D303).
func (lib *testLibrary) fileTwice() {
	lib.t.Helper()
	lib.rowsAPassWrites()
	for round := range 2 {
		if written := lib.rowsAPassWrites(); written != 0 {
			lib.t.Errorf("pass %d over an unchanged library wrote %d rows; want none", round+2, written)
		}
	}
}

// personOf is TMDB's person by their id, as their name and picture.
func (lib *testLibrary) personOf(tmdbID string) string {
	lib.t.Helper()
	rows := storetest.QueryRaw(lib.t, lib.database.Path(), fmt.Sprintf(
		"SELECT item_text.name || ' ' || coalesce(person.picture_ref, '-') FROM person "+
			"JOIN item_text ON item_text.item_id = person.id AND item_text.is_original = 1 "+
			"WHERE person.tmdb_id = '%s'", tmdbID))
	if len(rows) != 1 {
		lib.t.Fatalf("TMDB's person %s: %v; want one", tmdbID, rows)
	}
	return rows[0]
}

// matchFilm matches the film of one file, the clock then moved on, so the next match's response is
// fetched later.
func (lib *testLibrary) matchFilm(rel string) {
	lib.t.Helper()
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		lib.t.Fatalf("Match(%s) = %s", rel, outcome)
	}
	lib.clock.Advance(time.Hour)
}

// Two films' stored TMDB responses picture one person differently (each response is TMDB's answer
// when it was fetched): a pass over the unchanged library must still write nothing (D303), where it
// traded the picture between them on every pass (staging, 2026-11-13).
func TestPass_APersonTwoResponsesPictureDifferentlySettles(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	talisk := identify.Credit{
		Role: identify.RoleCast, Name: "Oren Talisk", PersonID: "1158", Character: "Jory Callant",
		PictureRef: "/old.jpg",
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1927)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "9019", Name: "Metropolis", Year: 1927, Credits: []identify.Credit{talisk},
		}),
	}
	talisk.PictureRef = "/new.jpg"
	lib.provider.Searches[identifytest.SearchKey("The Phantom of the Opera", 1925)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "4802", Name: "The Phantom of the Opera", Year: 1925, Credits: []identify.Credit{talisk},
		}),
	}
	metropolis := "Metropolis (1927)/Metropolis (1927).mkv"
	phantom := "The Phantom of the Opera (1925)/The Phantom of the Opera (1925).mkv"
	lib.scan(videoRecord(metropolis), videoRecord(phantom))
	lib.matchFileTitle(metropolis)
	lib.matchFileTitle(phantom)
	lib.fileTwice()
	// Both responses were fetched at once: the later item's names the person.
	if person := lib.personOf("1158"); person != "Oren Talisk /new.jpg" {
		t.Errorf("the person is %q; want the later film's picture", person)
	}
}

// A library of films that credit people in common, each response fetched at its own time and
// naming or picturing some of them differently, is filed twice, and the second time writes nothing
// (D303): each person is named and pictured by the newest response that credits them, whichever
// film a pass files last (the owner, 2026-11-13).
func TestPass_FilmsCreditingPeopleDifferentlySettleOnTheNewestResponse(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	cast := func(tmdbID, name, picture string) identify.Credit {
		return identify.Credit{
			Role: identify.RoleCast, Name: name, PersonID: tmdbID, Character: "Someone", PictureRef: picture,
		}
	}
	director := identify.Credit{
		Role: identify.RoleDirector, Name: "Wend Halvors", PersonID: "300", PictureRef: "/wend.jpg",
	}
	films := []struct {
		id, name string
		year     int
		credits  []identify.Credit
	}{
		{"9019", "Metropolis", 1927, []identify.Credit{
			director, cast("100", "Oren Talisk", "/talisk-newest.jpg"), cast("200", "Ana Kessel", "/kessel.jpg"),
		}},
		{"4802", "The Phantom of the Opera", 1925, []identify.Credit{
			director, cast("100", "Oren Talisk", "/talisk-middle.jpg"), cast("200", "Anna Kessel", "/kessel.jpg"),
		}},
		{"961", "The General", 1926, []identify.Credit{
			cast("100", "Oren Talisk", "/talisk-oldest.jpg"), cast("200", "Ana Kessel", "/kessel-old.jpg"),
		}},
		{"10728", "Faust", 1926, []identify.Credit{director, cast("200", "Ana Kessel", "/kessel-old.jpg")}},
	}
	rels := make([]string, 0, len(films))
	for _, film := range films {
		lib.provider.Searches[identifytest.SearchKey(film.name, film.year)] = []identify.Candidate{
			lib.provider.Add(identify.Metadata{ID: film.id, Name: film.name, Year: film.year, Credits: film.credits}),
		}
		rels = append(rels, fmt.Sprintf("%s (%d)/%s (%d).mkv", film.name, film.year, film.name, film.year))
	}
	lib.scanAll(rels...)
	// Matched oldest first, the first film a pass files is the newest: The General, then Faust and
	// the Phantom fetched at the same time, then Metropolis.
	lib.matchFilm(rels[2])
	lib.matchFileTitle(rels[3])
	lib.matchFilm(rels[1])
	lib.matchFilm(rels[0])
	lib.fileTwice()
	for tmdbID, want := range map[string]string{
		"100": "Oren Talisk /talisk-newest.jpg", "200": "Ana Kessel /kessel.jpg", "300": "Wend Halvors /wend.jpg",
	} {
		if person := lib.personOf(tmdbID); person != want {
			t.Errorf("TMDB's person %s is %q; want %q, the newest response's", tmdbID, person, want)
		}
	}
}

// Two series that credit one person, pictured differently by responses fetched at different times,
// settle as films do: the second filing writes nothing, and the newer response pictures them.
func TestPass_SeriesCreditingAPersonDifferentlySettleOnTheNewestResponse(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "tmdb")
	shared := func(picture string) []identify.Credit {
		return []identify.Credit{
			{Role: identify.RoleCast, Name: "Oren Talisk", PersonID: "1158", Character: "Keeper", PictureRef: picture},
		}
	}
	for _, series := range []identify.Metadata{
		{ID: "501", Name: "Harbour Lights", Year: 2004, SeasonNumbers: []int{1}, Credits: shared("/talisk-newer.jpg")},
		{ID: "502", Name: "Emberwing", Year: 2009, SeasonNumbers: []int{1}, Credits: shared("/talisk-older.jpg")},
	} {
		lib.provider.Searches[identifytest.SearchKey(series.Name, 0)] = []identify.Candidate{lib.provider.Add(series)}
		lib.provider.Seasons[series.ID+"/1"] = []identify.Episode{
			{ID: series.ID + "01", Season: 1, Number: 1, Name: "One"},
		}
	}
	lib.scanAll("Harbour Lights/Season 01/Harbour Lights - S01E01.mkv", "Emberwing/Season 01/Emberwing - S01E01.mkv")
	held, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil || len(held) != 2 {
		t.Fatalf("series: %+v, %v; want two", held, err)
	}
	// The older response, Emberwing's, is matched first.
	slices.SortFunc(held, func(a, b sqlcgen.ListLibrarySeriesRow) int {
		return strings.Compare(ptr.ValueOrZero(a.Name), ptr.ValueOrZero(b.Name))
	})
	for _, series := range held {
		if outcome, err := lib.matcher.Match(t.Context(), series.ID); err != nil || outcome != identify.OutcomeMatched {
			t.Fatalf("Match(series %d) = %s, %v", series.ID, outcome, err)
		}
		lib.clock.Advance(time.Hour)
	}
	lib.fileTwice()
	if person := lib.personOf("1158"); person != "Oren Talisk /talisk-newer.jpg" {
		t.Errorf("the person is %q; want the newer response's picture", person)
	}
}

// A refresh that fetches an older film's response again makes it the newest: it pictures the
// person, though the film's credits are as they were, and the library still settles.
func TestPass_ARefreshedResponsePicturesThePersonItCredits(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	talisk := identify.Credit{
		Role: identify.RoleCast, Name: "Oren Talisk", PersonID: "1158", Character: "Jory Callant",
		PictureRef: "/old.jpg",
	}
	general := identify.Metadata{ID: "961", Name: "The General", Year: 1926, Credits: []identify.Credit{talisk}}
	lib.provider.Searches[identifytest.SearchKey("The General", 1926)] = []identify.Candidate{lib.provider.Add(general)}
	talisk.PictureRef = "/newer.jpg"
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1927)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "9019", Name: "Metropolis", Year: 1927, Credits: []identify.Credit{talisk},
		}),
	}
	generalFile := "The General (1926)/The General (1926).mkv"
	metropolisFile := "Metropolis (1927)/Metropolis (1927).mkv"
	lib.scanAll(generalFile, metropolisFile)
	lib.matchFilm(generalFile)
	lib.matchFilm(metropolisFile)
	lib.fileTwice()
	if person := lib.personOf("1158"); person != "Oren Talisk /newer.jpg" {
		t.Fatalf("the person is %q before the refresh; want the newer response's picture", person)
	}
	general.Credits[0].PictureRef = "/newest.jpg"
	lib.provider.Add(general)
	lib.refresh(lib.titlesByPath()[generalFile].WorkID)
	lib.fileTwice()
	if person := lib.personOf("1158"); person != "Oren Talisk /newest.jpg" {
		t.Errorf("the person is %q after the refresh; want the refreshed response's picture", person)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// A pass over an unchanged library runs the same statements at twice the library's size, and no
// write transaction at all: it reads the library in a few statements, compares before it writes,
// and so runs in a read transaction alone (D303, DL6, DL7, DL9, DL11).
func TestPass_OverAnUnchangedLibraryRunsTheSameStatementsAtTwiceItsSizeAndTakesNoWriter(t *testing.T) {
	t.Parallel()
	shapes := []struct {
		name string
		fill func(t *testing.T, size int) *testLibrary
	}{
		{"films with NFO files", nfoFilms},
		{"films matched at TMDB", matchedFilms},
		{"series with tvshow.nfo files", nfoSeries},
		{"series matched at TMDB", matchedSeries},
	}
	for _, shape := range shapes {
		var statements []int
		var tallies []string
		for _, size := range []int{3, 6} {
			lib := shape.fill(t, size)
			ctx, tally := querytally.New(t.Context())
			if _, err := lib.pass.IdentifyLibrary(ctx, lib.libraryID); err != nil {
				t.Fatal(err)
			}
			if tally.Writes() != 0 {
				t.Errorf("%s, %d of them: an unchanged pass ran %d write transactions, want none: %s",
					shape.name, size, tally.Writes(), tally)
			}
			statements, tallies = append(statements, tally.Statements()), append(tallies, tally.String())
		}
		if statements[0] != statements[1] {
			t.Errorf("%s: an unchanged pass ran %d statements over 3 and %d over 6; want the same:\n%s\n%s",
				shape.name, statements[0], statements[1], tallies[0], tallies[1])
		}
	}
}

// nfoFilms is a library of films, each with its NFO file, with an extra and a locked field.
func nfoFilms(t *testing.T, size int) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "movie", "nfo")
	var records []nodeproto.ScanRecord
	for number := range size {
		folder := fmt.Sprintf("Film %d (%d)", number, 1990+number)
		records = append(records, videoRecord(folder+"/"+folder+".mkv"),
			videoRecord(folder+"/"+folder+"-trailer.mkv"),
			nfoRecord(folder+"/"+folder+".nfo", fmt.Sprintf(
				`<movie><title>Film %d</title><year>%d</year><plot>A plot.</plot><uniqueid type="tmdb">%d</uniqueid>`+
					`<genre>Drama / Crime</genre>`+
					`<actor><name>Ann Actor</name><role>Lead</role><order>0</order></actor>`+
					`<actor><name>Actor %d</name></actor><director>Dee Rector</director>`+
					`<lockdata>true</lockdata></movie>`, number, 1990+number, 100+number, number)))
	}
	lib.scan(records...)
	return lib
}

// matchedFilms is a library of films, each matched at TMDB, one confirmed by a person.
func matchedFilms(t *testing.T, size int) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "movie", "tmdb")
	var records []nodeproto.ScanRecord
	for number := range size {
		name, year := fmt.Sprintf("Film %d", number), 1990+number
		lib.provider.Searches[identifytest.SearchKey(name, year)] = []identify.Candidate{
			lib.provider.Add(identify.Metadata{
				ID: fmt.Sprint(100 + number), Name: name, Year: year, Genres: []string{"Drama", "Crime"},
				Credits: castAndCrew(number),
			}),
		}
		folder := fmt.Sprintf("%s (%d)", name, year)
		records = append(records, videoRecord(folder+"/"+folder+".mkv"))
	}
	lib.scan(records...)
	for path := range lib.titlesByPath() {
		if outcome := lib.matchFileTitle(path); outcome != identify.OutcomeMatched {
			t.Fatalf("%s: %s, want matched", path, outcome)
		}
	}
	// A person's chosen match is confirmed, which a pass never lowers (D172).
	if err := lib.chooser.Choose(t.Context(), lib.titlesByPath()["Film 0 (1990)/Film 0 (1990).mkv"].TitleID,
		"tmdb", "100"); err != nil {
		t.Fatal(err)
	}
	lib.scan(records...)
	return lib
}

// nfoSeries is a library of series, each with its tvshow.nfo and two episodes.
func nfoSeries(t *testing.T, size int) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "series", "nfo")
	var records []nodeproto.ScanRecord
	for number := range size {
		show := fmt.Sprintf("Show %d", number)
		records = append(records,
			nfoRecord(show+"/tvshow.nfo", fmt.Sprintf(`<tvshow><title>%s</title><year>2001</year><genre>Comedy</genre>`+
				`<actor><name>Ann Actor</name></actor><actor><name>%s Star</name></actor></tvshow>`, show, show)),
			videoRecord(show+"/Season 01/"+show+" - S01E01.mkv"),
			videoRecord(show+"/Season 01/"+show+" - S01E02.mkv"))
	}
	lib.scan(records...)
	return lib
}

// matchedSeries is a library of series, each matched at TMDB with its season listed.
func matchedSeries(t *testing.T, size int) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "series", "tmdb")
	var rels []string
	for number := range size {
		show, id := fmt.Sprintf("Show %d", number), fmt.Sprint(500+number)
		lib.provider.Searches[identifytest.SearchKey(show, 0)] = []identify.Candidate{
			lib.provider.Add(identify.Metadata{
				ID: id, Name: show, Year: 2001, Genres: []string{"Comedy"}, Credits: castAndCrew(number),
			}),
		}
		lib.provider.Seasons[id+"/1"] = []identify.Episode{
			{ID: id + "1", Season: 1, Number: 1, Name: "One"}, {ID: id + "2", Season: 1, Number: 2, Name: "Two"},
		}
		rels = append(rels, show+"/Season 01/"+show+" - S01E01.mkv", show+"/Season 01/"+show+" - S01E02.mkv")
	}
	lib.scanAll(rels...)
	series, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil {
		t.Fatal(err)
	}
	for _, show := range series {
		if outcome, err := lib.matcher.Match(t.Context(), show.ID); err != nil || outcome != identify.OutcomeMatched {
			t.Fatalf("Match(series %d) = %s, %v", show.ID, outcome, err)
		}
	}
	lib.runOwedPasses()
	lib.scanAll(rels...)
	return lib
}

// castAndCrew are a work's people at TMDB: one actor in every work, one its own, and a director.
func castAndCrew(number int) []identify.Credit {
	return []identify.Credit{
		{Role: identify.RoleCast, Name: "Ann Actor", PersonID: "1", Character: "Lead", PictureRef: "/ann.jpg"},
		{Role: identify.RoleCast, Name: fmt.Sprintf("Actor %d", number), PersonID: fmt.Sprint(1000 + number)},
		{Role: identify.RoleDirector, Name: "Dee Rector", PersonID: "2"},
	}
}

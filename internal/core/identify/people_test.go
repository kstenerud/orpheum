// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// creditsOf are an item's credits as "role ordinal name (character)", in billing order in each role.
func (lib *testLibrary) creditsOf(itemID int64) []string {
	lib.t.Helper()
	rows, err := lib.database.Read().ListItemCredits(lib.t.Context(), itemID)
	if err != nil {
		lib.t.Fatal(err)
	}
	credits := []string{}
	for _, row := range rows {
		credits = append(credits, fmt.Sprintf("%s %d %s (%s)", row.Job, row.Ordinal, row.Name,
			ptr.ValueOrZero(row.Character)))
	}
	return credits
}

// genresOf are an item's genres in its source's order.
func (lib *testLibrary) genresOf(itemID int64) []string {
	lib.t.Helper()
	rows, err := lib.database.Read().ListItemGenres(lib.t.Context(), itemID)
	if err != nil {
		lib.t.Fatal(err)
	}
	genres := []string{}
	for _, row := range rows {
		genres = append(genres, row.Name)
	}
	return genres
}

// personIDsOf are the people an item credits, in its credits' order.
func (lib *testLibrary) personIDsOf(itemID int64) []int64 {
	lib.t.Helper()
	rows, err := lib.database.Read().ListItemCredits(lib.t.Context(), itemID)
	if err != nil {
		lib.t.Fatal(err)
	}
	ids := []int64{}
	for _, row := range rows {
		ids = append(ids, row.PersonID)
	}
	return ids
}

func (lib *testLibrary) wantList(what string, got, want []string) {
	lib.t.Helper()
	if !slices.Equal(got, want) {
		lib.t.Errorf("%s %q,\nwant %q", what, got, want)
	}
}

// D186: a film's NFO file names its genres and people, and the pass keeps them.
func TestPass_KeepsAnNFOFilmsGenresAndPeople(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	lib.scan(videoRecord("Metropolis (1927)/Metropolis (1927).mkv"),
		nfoRecord("Metropolis (1927)/Metropolis (1927).nfo",
			`<movie><title>Metropolis</title><year>1927</year><genre>Crime / Drama</genre>`+
				`<actor><name>Oren Talisk</name><role>Jory Callant</role><order>1</order></actor>`+
				`<actor><name>Ada Wexcombe</name><role>Tamsin Vell</role><order>0</order></actor>`+
				`<director>Mira Holloway-Pent</director><credits>Mira Holloway-Pent</credits></movie>`))
	titleID := lib.titlesByPath()["Metropolis (1927)/Metropolis (1927).mkv"].WorkID
	lib.wantList("genres", lib.genresOf(titleID), []string{"Crime", "Drama"})
	lib.wantList("credits", lib.creditsOf(titleID), []string{
		"cast 0 Ada Wexcombe (Tamsin Vell)", "cast 1 Oren Talisk (Jory Callant)",
		"director 0 Mira Holloway-Pent ()", "writer 0 Mira Holloway-Pent ()",
	})
	people := lib.personIDsOf(titleID)
	if people[2] != people[3] {
		t.Errorf("Mira Holloway-Pent is two people, %d and %d; want one, credited twice", people[2], people[3])
	}
}

// D186: a film matched at TMDB keeps the genres and people of its stored response; one person
// credited on two films is one person, renamed where TMDB renames them.
func TestPass_KeepsAMatchedFilmsGenresAndPeopleAndOnePersonAcrossFilms(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	talisk := identify.Credit{Role: identify.RoleCast, Name: "Oren Talisk", PersonID: "1158", Character: "Jory Callant"}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1927)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "9019", Name: "Metropolis", Year: 1927, Genres: []string{"Crime", "Drama"},
			Credits: []identify.Credit{
				talisk, {Role: identify.RoleDirector, Name: "Mira Holloway-Pent", PersonID: "638"},
			},
		}),
	}
	talisk.Character = "Bastien Lark"
	lib.provider.Searches[identifytest.SearchKey("The Phantom of the Opera", 1925)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "4802", Name: "The Phantom of the Opera", Year: 1925, Genres: []string{"Drama", "Crime"},
			Credits: []identify.Credit{talisk},
		}),
	}
	metropolis := "Metropolis (1927)/Metropolis (1927).mkv"
	phantom := "The Phantom of the Opera (1925)/The Phantom of the Opera (1925).mkv"
	lib.scan(videoRecord(metropolis), videoRecord(phantom))
	lib.matchFileTitle(metropolis)
	lib.matchFileTitle(phantom)
	lib.runOwedPasses()
	metropolisID, phantomID := lib.titlesByPath()[metropolis].WorkID, lib.titlesByPath()[phantom].WorkID
	lib.wantList("Metropolis's genres", lib.genresOf(metropolisID), []string{"Crime", "Drama"})
	lib.wantList("The Phantom of the Opera's genres", lib.genresOf(phantomID), []string{"Drama", "Crime"})
	lib.wantList("Metropolis's credits", lib.creditsOf(metropolisID), []string{
		"cast 0 Oren Talisk (Jory Callant)", "director 0 Mira Holloway-Pent ()",
	})
	lib.wantList("The Phantom of the Opera's credits", lib.creditsOf(phantomID),
		[]string{"cast 0 Oren Talisk (Bastien Lark)"})
	metropolisPeople, phantomPeople := lib.personIDsOf(metropolisID), lib.personIDsOf(phantomID)
	if metropolisPeople[0] != phantomPeople[0] {
		t.Errorf("Oren Talisk is person %d in Metropolis and %d in The Phantom of the Opera; want one person",
			metropolisPeople[0], phantomPeople[0])
	}
}

// D186: a title identified before genres and people were kept gets them from its stored response at
// the next pass, with no request to its provider.
func TestPass_FillsATitlesGenresAndPeopleFromItsStoredResponseWithNoRequest(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1927)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "9019", Name: "Metropolis", Year: 1927, Genres: []string{"Crime"},
			Credits: []identify.Credit{{Role: identify.RoleCast, Name: "Oren Talisk", PersonID: "1158"}},
		}),
	}
	const metropolis = "Metropolis (1927)/Metropolis (1927).mkv"
	lib.scan(videoRecord(metropolis))
	lib.matchFileTitle(metropolis)
	lib.runOwedPasses()
	titleID := lib.titlesByPath()[metropolis].WorkID
	// As a server that kept none held it: the response stored, nothing read from it yet.
	storetest.ExecRaw(t, lib.database.Path(),
		fmt.Sprintf("DELETE FROM credit WHERE item_id = %d", titleID),
		fmt.Sprintf("DELETE FROM item_genre WHERE item_id = %d", titleID))
	calls := len(lib.provider.Calls)
	lib.scan(videoRecord(metropolis))
	lib.wantList("genres", lib.genresOf(titleID), []string{"Crime"})
	lib.wantList("credits", lib.creditsOf(titleID), []string{"cast 0 Oren Talisk ()"})
	if made := lib.provider.Calls[calls:]; len(made) != 0 {
		t.Errorf("the fill asked its provider %q; want nothing asked", made)
	}
}

// D183: a series' tvshow.nfo names its genres and people, and its series keeps them.
func TestPass_KeepsASeriesNFOsGenresAndPeople(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "nfo")
	lib.scan(nfoRecord("Tin Comet/tvshow.nfo", `<tvshow><title>Tin Comet</title><genre>Science Fiction</genre>`+
		`<actor><name>Nathan Fillion</name><role>Mal</role></actor></tvshow>`),
		videoRecord("Tin Comet/Season 01/Tin Comet - S01E01.mkv"))
	seriesID := lib.seriesOfFile("Tin Comet/Season 01/Tin Comet - S01E01.mkv")
	lib.wantList("genres", lib.genresOf(seriesID), []string{"Science Fiction"})
	lib.wantList("credits", lib.creditsOf(seriesID), []string{"cast 0 Nathan Fillion (Mal)"})
}

// Principles §22, D211: a provider's genres and people are clean where they enter.
func TestMatch_AProvidersGenresAndPeopleAreCleanWhereTheyEnter(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Add(identify.Metadata{
		ID: "9019", Name: "Metropolis", Year: 1927, Genres: []string{"Cri\u202eme", "\u200b"},
		Credits: []identify.Credit{
			{Role: identify.RoleCast, Name: "Oren\u0000 Talisk", PersonID: "1158", Character: "Jo\u2067ry"},
			{Role: identify.RoleCast, Name: "\u200b", PersonID: "1"},
		},
	})
	const metropolis = "Metropolis {tmdb-9019}.mkv"
	lib.scan(videoRecord(metropolis))
	lib.matchFileTitle(metropolis)
	lib.runOwedPasses()
	titleID := lib.titlesByPath()[metropolis].WorkID
	lib.wantList("genres", lib.genresOf(titleID), []string{"Crime"})
	lib.wantList("credits", lib.creditsOf(titleID), []string{"cast 0 Oren Talisk (Jory)"})
}

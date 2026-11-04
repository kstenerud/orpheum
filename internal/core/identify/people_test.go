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
		credits = append(credits, fmt.Sprintf("%s %d %s (%s)", row.Role, row.Ordinal, row.Name,
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
	lib := newTestLibrary(t, "movies", "nfo")
	lib.scan(videoRecord("Metropolis (1995)/Metropolis (1995).mkv"), nfoRecord("Metropolis (1995)/Metropolis (1995).nfo",
		`<movie><title>Metropolis</title><year>1995</year><genre>Crime / Drama</genre>`+
			`<actor><name>Ansel Vardo</name><role>Mira Holloway-Pent</role><order>1</order></actor>`+
			`<actor><name>Ada Wexcombe</name><role>Tamsin Vell</role><order>0</order></actor>`+
			`<director>Oren Talisk</director><credits>Oren Talisk</credits></movie>`))
	titleID := lib.titlesByPath()["Metropolis (1995)/Metropolis (1995).mkv"].TitleID
	lib.wantList("genres", lib.genresOf(titleID), []string{"Crime", "Drama"})
	lib.wantList("credits", lib.creditsOf(titleID), []string{
		"cast 0 Ada Wexcombe (Tamsin Vell)", "cast 1 Ansel Vardo (Mira Holloway-Pent)",
		"director 0 Oren Talisk ()", "writer 0 Oren Talisk ()",
	})
	people := lib.personIDsOf(titleID)
	if people[2] != people[3] {
		t.Errorf("Oren Talisk is two people, %d and %d; want one, credited twice", people[2], people[3])
	}
}

// D186: a film matched at TMDB keeps the genres and people of its stored response; one person
// credited on two films is one person, renamed where TMDB renames them.
func TestPass_KeepsAMatchedFilmsGenresAndPeopleAndOnePersonAcrossFilms(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	vardo := identify.Credit{Role: identify.RoleCast, Name: "Ansel Vardo", PersonID: "1158", Character: "Mira Holloway-Pent"}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "9019", Name: "Metropolis", Year: 1995, Genres: []string{"Crime", "Drama"},
			Credits: []identify.Credit{vardo, {Role: identify.RoleDirector, Name: "Oren Talisk", PersonID: "638"}},
		}),
	}
	vardo.Character = "Bastien Lark"
	lib.provider.Searches[identifytest.SearchKey("The Phantom of the Opera", 1972)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "4802", Name: "The Phantom of the Opera", Year: 1972, Genres: []string{"Drama", "Crime"},
			Credits: []identify.Credit{vardo},
		}),
	}
	heat, phantom := "Metropolis (1995)/Metropolis (1995).mkv", "The Phantom of the Opera (1972)/The Phantom of the Opera (1972).mkv"
	lib.scan(videoRecord(heat), videoRecord(phantom))
	lib.matchFileTitle(heat)
	lib.matchFileTitle(phantom)
	lib.runOwedPasses()
	metropolisID, phantomID := lib.titlesByPath()[heat].TitleID, lib.titlesByPath()[phantom].TitleID
	lib.wantList("Metropolis's genres", lib.genresOf(metropolisID), []string{"Crime", "Drama"})
	lib.wantList("The Phantom of the Opera's genres", lib.genresOf(phantomID), []string{"Drama", "Crime"})
	lib.wantList("Metropolis's credits", lib.creditsOf(metropolisID), []string{
		"cast 0 Ansel Vardo (Mira Holloway-Pent)", "director 0 Oren Talisk ()",
	})
	lib.wantList("The Phantom of the Opera's credits", lib.creditsOf(phantomID),
		[]string{"cast 0 Ansel Vardo (Bastien Lark)"})
	metropolisPeople, phantomPeople := lib.personIDsOf(metropolisID), lib.personIDsOf(phantomID)
	if metropolisPeople[0] != phantomPeople[0] {
		t.Errorf("Ansel Vardo is person %d in Metropolis and %d in The Phantom of the Opera; want one person",
			metropolisPeople[0], phantomPeople[0])
	}
}

// D186: a title identified before genres and people were kept gets them from its stored response at
// the next pass, with no request to its provider.
func TestPass_FillsATitlesGenresAndPeopleFromItsStoredResponseWithNoRequest(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{
			ID: "9019", Name: "Metropolis", Year: 1995, Genres: []string{"Crime"},
			Credits: []identify.Credit{{Role: identify.RoleCast, Name: "Ansel Vardo", PersonID: "1158"}},
		}),
	}
	const heat = "Metropolis (1995)/Metropolis (1995).mkv"
	lib.scan(videoRecord(heat))
	lib.matchFileTitle(heat)
	lib.runOwedPasses()
	titleID := lib.titlesByPath()[heat].TitleID
	// As a server that kept none held it: the response stored, nothing read from it yet.
	storetest.ExecRaw(t, lib.database.Path(),
		fmt.Sprintf("DELETE FROM credit WHERE item_id = %d", titleID),
		fmt.Sprintf("DELETE FROM item_genre WHERE item_id = %d", titleID))
	calls := len(lib.provider.Calls)
	lib.scan(videoRecord(heat))
	lib.wantList("genres", lib.genresOf(titleID), []string{"Crime"})
	lib.wantList("credits", lib.creditsOf(titleID), []string{"cast 0 Ansel Vardo ()"})
	if made := lib.provider.Calls[calls:]; len(made) != 0 {
		t.Errorf("the fill asked its provider %q; want nothing asked", made)
	}
}

// D183: a series' tvshow.nfo names its genres and people, and its series keeps them.
func TestPass_KeepsASeriesNFOsGenresAndPeople(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "nfo")
	lib.scan(nfoRecord("Emberwing/tvshow.nfo", `<tvshow><title>Emberwing</title><genre>Science Fiction</genre>`+
		`<actor><name>Nathan Fillion</name><role>Mal</role></actor></tvshow>`),
		videoRecord("Emberwing/Season 01/Emberwing - S01E01.mkv"))
	seriesID := lib.seriesOfFile("Emberwing/Season 01/Emberwing - S01E01.mkv")
	lib.wantList("genres", lib.genresOf(seriesID), []string{"Science Fiction"})
	lib.wantList("credits", lib.creditsOf(seriesID), []string{"cast 0 Nathan Fillion (Mal)"})
}

// Principles §22, D211: a provider's genres and people are clean where they enter.
func TestMatch_AProvidersGenresAndPeopleAreCleanWhereTheyEnter(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Add(identify.Metadata{
		ID: "9019", Name: "Metropolis", Year: 1995, Genres: []string{"Cri\u202eme", "\u200b"},
		Credits: []identify.Credit{
			{Role: identify.RoleCast, Name: "Al\u0000 Vardo", PersonID: "1158", Character: "Vin\u2067cent"},
			{Role: identify.RoleCast, Name: "\u200b", PersonID: "1"},
		},
	})
	const heat = "Metropolis {tmdb-9019}.mkv"
	lib.scan(videoRecord(heat))
	lib.matchFileTitle(heat)
	lib.runOwedPasses()
	titleID := lib.titlesByPath()[heat].TitleID
	lib.wantList("genres", lib.genresOf(titleID), []string{"Crime"})
	lib.wantList("credits", lib.creditsOf(titleID), []string{"cast 0 Ansel Vardo (Vincent)"})
}

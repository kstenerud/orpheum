// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// releasedOnOf is each film's and series' release day as the pass stored it, by name.
func (lib *testLibrary) releasedOnOf() []string {
	lib.t.Helper()
	return storetest.QueryRaw(lib.t, lib.database.Path(),
		`SELECT item_text.name || ' ' || coalesce(title.released_on, collection.released_on, 'none')
		 FROM item_text LEFT JOIN title ON title.id = item_text.item_id AND title.kind = 'movie'
		 LEFT JOIN collection ON collection.id = item_text.item_id
		 WHERE item_text.is_original = 1 AND (title.id IS NOT NULL OR collection.id IS NOT NULL)
		 ORDER BY item_text.name`)
}

// A film is dated by its NFO's premiered day, else its year alone; a series by its tvshow.nfo's; one
// with neither has no date; a person's year of another year than the source's day dates it by that
// year alone (D366 §1).
func TestPass_AFilmAndASeriesAreDatedByTheirSourcesDayElseTheirYear(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	lib.scan(
		nfoRecord("Metropolis (1995)/Metropolis (1995).nfo",
			`<movie><title>Metropolis</title><year>1995</year><premiered>1995-12-15</premiered></movie>`),
		videoRecord("Metropolis (1995)/Metropolis (1995).mkv"),
		nfoRecord("Arkon (1979)/Arkon (1979).nfo", `<movie><title>Arkon</title><year>1979</year></movie>`),
		videoRecord("Arkon (1979)/Arkon (1979).mkv"),
		nfoRecord("The Freshman/The Freshman.nfo", `<movie><title>The Freshman</title></movie>`),
		videoRecord("The Freshman/The Freshman.mkv"),
	)
	want := []string{"Arkon 19790000", "The Freshman none", "Metropolis 19951215"}
	if got := lib.releasedOnOf(); !slices.Equal(got, want) {
		t.Errorf("the films' days = %v; want %v", got, want)
	}
	metropolisID := lib.titlesByPath()["Metropolis (1995)/Metropolis (1995).mkv"].TitleID
	storetest.ExecRaw(t, lib.database.Path(), fmt.Sprintf(`INSERT INTO field_override (item_id, field, language,
		override_value, origin, set_at) VALUES (%d, 'year', '', '1996', 'typed', 1)`, metropolisID))
	lib.scan(
		nfoRecord("Metropolis (1995)/Metropolis (1995).nfo",
			`<movie><title>Metropolis</title><year>1995</year><premiered>1995-12-15</premiered></movie>`),
		videoRecord("Metropolis (1995)/Metropolis (1995).mkv"),
	)
	if got := lib.releasedOnOf(); !slices.Contains(got, "Metropolis 19960000") {
		t.Errorf("the films' days with Metropolis's year corrected to 1996 = %v; want Metropolis 19960000", got)
	}

	shows := newTestLibrary(t, "series", "nfo")
	shows.scan(
		nfoRecord("Brass Arena/tvshow.nfo",
			`<tvshow><title>Brass Arena</title><year>2008</year><premiered>2008-01-20</premiered></tvshow>`),
		videoRecord("Brass Arena/Season 01/Brass Arena - S01E01.mkv"),
	)
	if got := shows.releasedOnOf(); !slices.Equal(got, []string{"Brass Arena 20080120"}) {
		t.Errorf("the series' day = %v; want its first air date", got)
	}
}

// A film matched at its provider is dated by the provider's release date (D366 §1).
func TestMatch_AFilmIsDatedByItsProvidersReleaseDate(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	dated := heat
	dated.ReleaseDate = "1995-12-15"
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(dated)}
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	lib.scan(videoRecord(rel))
	if got := lib.releasedOnOf(); !slices.Equal(got, []string{"Metropolis 19950000"}) {
		t.Errorf("before its match = %v; want its year from its name alone", got)
	}
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s, want matched", outcome)
	}
	if got := lib.releasedOnOf(); !slices.Equal(got, []string{"Metropolis 19951215"}) {
		t.Errorf("matched = %v; want TMDB's release date", got)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

var orbitorama = identify.Metadata{
	ID: "7615", Name: "Orbitorama", Year: 1999, Overview: "A delivery boy, a thousand years on.",
	IDs: map[string]string{"tvdb": "78871"},
}

// orbitoramaLibrary is a TMDB library holding two episodes of season 2, one of which TMDB lists.
func orbitoramaLibrary(t *testing.T) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "shows", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Orbitorama", 0)] = []identify.Candidate{lib.provider.Add(orbitorama)}
	lib.provider.Seasons["615/2"] = []identify.Episode{{
		ID: "1101", Season: 2, Number: 10, Name: "A Comet of My Own", Overview: "The Professor names his successor.",
		Year: 2000, RuntimeMs: 22 * 60_000,
	}}
	return lib
}

var orbitoramaFiles = []string{"Orbitorama/Season 02/Orbitorama - S02E10.mkv", "Orbitorama/Season 02/Orbitorama - S02E11.mkv"}

func (lib *testLibrary) scanAll(rels ...string) identify.Report {
	lib.t.Helper()
	records := make([]nodeproto.ScanRecord, 0, len(rels))
	for _, rel := range rels {
		records = append(records, videoRecord(rel))
	}
	return lib.scan(records...)
}

// onlySeries is the one series in the library.
func (lib *testLibrary) onlySeries() sqlcgen.GetSeriesForMatchRow {
	lib.t.Helper()
	series, err := lib.database.Read().ListLibrarySeries(lib.t.Context(), &lib.libraryID)
	if err != nil || len(series) != 1 {
		lib.t.Fatalf("series: %+v, %v; want one", series, err)
	}
	row, err := lib.database.Read().GetSeriesForMatch(lib.t.Context(), series[0].ID)
	if err != nil {
		lib.t.Fatal(err)
	}
	return row
}

func TestSeries_AMatchedSeriesNamesItsEpisodesFromOneSeasonListing(t *testing.T) {
	t.Parallel()
	lib := orbitoramaLibrary(t)
	report := lib.scanAll(orbitoramaFiles...)
	series := lib.onlySeries()
	if !slices.Contains(report.AwaitingMatch, series.ID) {
		t.Fatalf("awaiting %v, want the series %d", report.AwaitingMatch, series.ID)
	}
	file := lib.titlesByPath()[orbitoramaFiles[0]]
	if file.FilingReason == nil || *file.FilingReason != "its series is not matched yet" {
		t.Errorf("before the match: %+v", file)
	}
	outcome, err := lib.matcher.Match(t.Context(), series.ID)
	if err != nil || outcome != identify.OutcomeMatched {
		t.Fatalf("Match(series) = %s, %v", outcome, err)
	}
	if want := []string{"search Orbitorama|0", "fetch 615", "season 615/2"}; !slices.Equal(lib.provider.Calls, want) {
		t.Errorf("calls %v, want %v: one listing for the season, not one call an episode", lib.provider.Calls, want)
	}
	checkOrbitorama(t, lib, "after the match")
	calls := len(lib.provider.Calls)
	report = lib.scanAll(orbitoramaFiles...)
	checkOrbitorama(t, lib, "after another pass")
	if len(lib.provider.Calls) != calls || len(report.AwaitingListings) != 0 || len(report.AwaitingMatch) != 0 {
		t.Errorf("another pass called %v, or queued %v and %v; an old unlisted episode is no reason to fetch",
			lib.provider.Calls[calls:], report.AwaitingMatch, report.AwaitingListings)
	}
}

// checkOrbitorama checks S02E10 is named from the listing, S02E11 says TMDB does not list it,
// and the series is TMDB's.
func checkOrbitorama(t *testing.T, lib *testLibrary, when string) {
	t.Helper()
	files := lib.titlesByPath()
	clone, missing := files[orbitoramaFiles[0]], files[orbitoramaFiles[1]]
	fromTMDB := clone.FollowedSource != nil && *clone.FollowedSource == identify.SourceTMDB
	if titleName(clone) != "A Comet of My Own" || clone.IdentificationState != identify.IdentificationIdentified ||
		clone.Filing != identify.FilingListed ||
		!fromTMDB || clone.Year == nil || *clone.Year != 2000 {
		t.Errorf("%s: S02E10 = %+v named %q", when, clone, titleName(clone))
	}
	if missing.Filing != identify.FilingUnidentified || missing.FilingReason == nil ||
		*missing.FilingReason != "tmdb lists no season 2, episode 11" {
		t.Errorf("%s: S02E11 = %+v, want unidentified, saying TMDB does not list it", when, missing)
	}
	series := lib.onlySeries()
	got := []string{
		ptr.ValueOrZero(series.Name), strconv.FormatInt(ptr.ValueOrZero(series.Year), 10),
		ptr.ValueOrZero(series.IdentificationState),
		ptr.ValueOrZero(series.FollowedSource), ptr.ValueOrZero(series.FollowedSourceID),
	}
	want := []string{"Orbitorama", "1999", identify.IdentificationIdentified, identify.SourceTMDB, "7615"}
	if !slices.Equal(got, want) {
		t.Errorf("%s: series name, year, ident, source and id = %v, want %v", when, got, want)
	}
}

func TestSeries_ANewEpisodeFetchesItsSeasonAgain(t *testing.T) {
	t.Parallel()
	lib := orbitoramaLibrary(t)
	lib.scanAll(orbitoramaFiles...)
	series := lib.onlySeries()
	if _, err := lib.matcher.Match(t.Context(), series.ID); err != nil {
		t.Fatal(err)
	}
	lib.clock.Advance(7 * 24 * time.Hour)
	lib.provider.Seasons["615/2"] = append(lib.provider.Seasons["615/2"],
		identify.Episode{ID: "1103", Season: 2, Number: 12, Name: "Raging Bender", Year: 2000})
	const newest = "Orbitorama/Season 02/Orbitorama - S02E12.mkv"
	report := lib.scanAll(append(slices.Clone(orbitoramaFiles), newest)...)
	if !slices.Equal(report.AwaitingListings, []int64{series.ID}) {
		t.Fatalf("awaiting listings %v, want the series: a new episode is a first fetch (D209 §3)",
			report.AwaitingListings)
	}
	outcome, err := lib.matcher.Match(t.Context(), series.ID)
	if err != nil || outcome != identify.OutcomeListingsFetched {
		t.Fatalf("Match = %s, %v", outcome, err)
	}
	if got := titleName(lib.titlesByPath()[newest]); got != "Raging Bender" {
		t.Errorf("the new episode is %q", got)
	}
}

// Two series of one name and no year to tell them apart: the oldest is taken as a guess, marked as
// one, and a person corrects it from the candidates kept for it, which is final (D301).
func TestSeries_OfSeveralByItsNameTheOldestIsGuessedAndAPersonCorrectsIt(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "tmdb")
	uk := lib.provider.Add(identify.Metadata{ID: "2996", Name: "The Ledger Room", Year: 2001})
	american := lib.provider.Add(identify.Metadata{ID: "2316", Name: "The Ledger Room", Year: 2005})
	lib.provider.Searches[identifytest.SearchKey("The Ledger Room", 0)] = []identify.Candidate{american, uk}
	lib.provider.Seasons["2996/2"] = []identify.Episode{
		{ID: "8", Season: 2, Number: 1, Name: "Merger", Year: 2002},
	}
	lib.provider.Seasons["2316/2"] = []identify.Episode{
		{ID: "9", Season: 2, Number: 1, Name: "The Dundies", Year: 2005},
	}
	const rel = "The Ledger Room/Season 2/The Ledger Room - S02E01.mkv"
	lib.scanAll(rel)
	series := lib.onlySeries()
	if outcome, err := lib.matcher.Match(t.Context(), series.ID); err != nil || outcome != identify.OutcomeMatched {
		t.Fatalf("Match = %s, %v; the oldest of that name is taken as a guess", outcome, err)
	}
	if !lib.isGuessed(series.ID) || !slices.Contains(lib.provider.Calls, "fetch series 2996") &&
		!slices.Contains(lib.provider.Calls, "fetch 2996") {
		t.Errorf("the guess is not the 2001 series, marked as a guess: %v", lib.provider.Calls)
	}
	candidates, _, err := lib.chooser.Candidates(t.Context(), series.ID)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("the candidates kept for correcting it = %+v, %v; want both", candidates, err)
	}
	if err := lib.chooser.Choose(t.Context(), series.ID, "tmdb", american.ID); err != nil {
		t.Fatal(err)
	}
	lib.scanAll(rel)
	if got := titleName(lib.titlesByPath()[rel]); got != "The Dundies" {
		t.Errorf("after choosing the American series: %q", got)
	}
	if ident := ptr.ValueOrZero(lib.onlySeries().IdentificationState); ident != identify.IdentificationConfirmed ||
		lib.isGuessed(series.ID) {
		t.Errorf("the chosen series is %s, guessed %t; want confirmed, no guess", ident, lib.isGuessed(series.ID))
	}
	episode := lib.titlesByPath()[rel].TitleID
	if err := lib.chooser.Choose(t.Context(), episode, "tmdb", "9"); err == nil {
		t.Error("an episode's match was chosen; its series is what is matched (D209)")
	}
}

func TestSeries_AYearOrAnIDSettlesTheSearch(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "nfo_then_tmdb")
	uk := lib.provider.Add(identify.Metadata{ID: "2996", Name: "The Ledger Room", Year: 2001})
	american := lib.provider.Add(identify.Metadata{ID: "2316", Name: "The Ledger Room", Year: 2005})
	lib.provider.Add(orbitorama)
	lib.provider.Searches[identifytest.SearchKey("The Ledger Room", 2005)] = []identify.Candidate{american, uk}
	lib.scan(videoRecord("The Ledger Room (2005)/Season 1/S01E01.mkv"),
		videoRecord("Orbitorama/Season 1/S01E01.mkv"),
		nfoRecord("Orbitorama/tvshow.nfo", "<tvshow><title>Orbitorama</title><tvdbid>78871</tvdbid></tvshow>"))
	lib.provider.TMDBIDsByOtherID["78871"] = "615" // the fake's Find serves every source from one table
	series, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil || len(series) != 2 {
		t.Fatalf("series %+v, %v", series, err)
	}
	for _, show := range series {
		if outcome, err := lib.matcher.Match(t.Context(), show.ID); err != nil || outcome != identify.OutcomeMatched {
			t.Errorf("Match(%s) = %s, %v", ptr.ValueOrZero(show.Name), outcome, err)
		}
	}
	if slices.Contains(lib.provider.Calls, "search Orbitorama|0") {
		t.Errorf("calls %v: a TVDB id in hand skips the search", lib.provider.Calls)
	}
}

func TestSeries_AnEpisodeAddedAfterTheSeriesWasRenamedJoinsIt(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "tmdb")
	sunny := lib.provider.Add(identify.Metadata{ID: "2710", Name: "It's Always Sunny in Philadelphia", Year: 2005})
	lib.provider.Seasons["2710/1"] = []identify.Episode{}
	first := "Always Sunny {tmdb-2710}/Season 1/S01E02.mkv"
	lib.scanAll(first)
	outcome, err := lib.matcher.Match(t.Context(), lib.onlySeries().ID)
	if err != nil || outcome != identify.OutcomeMatched ||
		sunny.ID == "" {
		t.Fatalf("Match = %s, %v; calls %v", outcome, err, lib.provider.Calls)
	}
	lib.scanAll("Always Sunny {tmdb-2710}/Season 1/S01E01.mkv", first)
	if series := lib.onlySeries(); ptr.ValueOrZero(series.Name) != sunny.Name {
		t.Errorf("series %+v", series)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

var orbitorama = identify.Metadata{
	ID: "7615", Name: "Orbitorama", Year: 2003, Overview: "A courier, far from home.",
	IDs: map[string]string{"tvdb": "78871"},
}

// orbitoramaLibrary is a TMDB library holding two episodes of season 2, one of which TMDB lists.
func orbitoramaLibrary(t *testing.T) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "series", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Orbitorama", 0)] = []identify.Candidate{lib.provider.Add(orbitorama)}
	lib.provider.Seasons["7615/2"] = []identify.Episode{{
		ID: "1101", Season: 2, Number: 10, Name: "A Comet of My Own", Overview: "A courier meets a copy of himself.",
		Year: 2000, RuntimeMs: 22 * 60_000,
	}}
	return lib
}

var orbitoramaFiles = []string{
	"Orbitorama/Season 02/Orbitorama - S02E10.mkv", "Orbitorama/Season 02/Orbitorama - S02E11.mkv",
}

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
	lib.runOwedPasses()
	if want := []string{"search Orbitorama|0", "fetch 7615", "season 7615/2"}; !slices.Equal(lib.provider.Calls, want) {
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
	if titleName(clone) != "A Comet of My Own" ||
		ptr.ValueOrZero(clone.IdentificationState) != identify.IdentificationIdentified ||
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
	want := []string{"Orbitorama", "2003", identify.IdentificationIdentified, identify.SourceTMDB, "7615"}
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
	lib.provider.Seasons["7615/2"] = append(lib.provider.Seasons["7615/2"],
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
	lib.runOwedPasses()
	if got := titleName(lib.titlesByPath()[newest]); got != "Raging Bender" {
		t.Errorf("the new episode is %q", got)
	}
}

// Two series of one name and no year to tell them apart: the oldest is taken as a guess, marked as
// one, and a person corrects it from the candidates kept for it, which is final (D301).
func TestSeries_OfSeveralByItsNameTheOldestIsGuessedAndAPersonCorrectsIt(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "tmdb")
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
	episode := lib.titlesByPath()[rel].WorkID
	if err := lib.chooser.Choose(t.Context(), episode, "tmdb", "9"); err == nil {
		t.Error("an episode's match was chosen; its series is what is matched (D209)")
	}
}

func TestSeries_AYearOrAnIDSettlesTheSearch(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "nfo_then_tmdb")
	uk := lib.provider.Add(identify.Metadata{ID: "2996", Name: "The Ledger Room", Year: 2001})
	american := lib.provider.Add(identify.Metadata{ID: "2316", Name: "The Ledger Room", Year: 2005})
	lib.provider.Add(orbitorama)
	lib.provider.Searches[identifytest.SearchKey("The Ledger Room", 2005)] = []identify.Candidate{american, uk}
	lib.scan(videoRecord("The Ledger Room (2005)/Season 1/S01E01.mkv"),
		videoRecord("Orbitorama/Season 1/S01E01.mkv"),
		nfoRecord("Orbitorama/tvshow.nfo", "<tvshow><title>Orbitorama</title><tvdbid>78871</tvdbid></tvshow>"))
	lib.provider.TMDBIDsByOtherID["78871"] = "7615" // the fake's Find serves every source from one table
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
	lib := newTestLibrary(t, "series", "tmdb")
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

// A series' match marks its library as owing a pass and wakes the sweep that runs it, rather than
// running the library's pass itself; matches of two series in one library are followed by one
// pass, not two (D303, DL14).
func TestMatch_SeriesMatchesInOneLibraryAreFollowedByOnePass(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Orbitorama", 0)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "7615", Name: "Orbitorama", Year: 2003}),
	}
	lib.provider.Searches[identifytest.SearchKey("Fennimore", 0)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "1400", Name: "Fennimore", Year: 1989}),
	}
	lib.provider.Seasons["7615/1"] = []identify.Episode{{ID: "1", Season: 1, Number: 1, Name: "Comet Pilot 3000"}}
	lib.provider.Seasons["1400/1"] = []identify.Episode{{ID: "2", Season: 1, Number: 1, Name: "Good News, Bad News"}}
	lib.scanAll("Orbitorama/Season 1/S01E01.mkv", "Fennimore/Season 1/S01E01.mkv")
	passes, wakes := 0, 0
	lib.pass.AfterPass(func(context.Context, identify.Report) { passes++ })
	lib.pass.OnPassOwed(func() { wakes++ })
	series, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil || len(series) != 2 {
		t.Fatalf("series %+v, %v", series, err)
	}
	for _, show := range series {
		if outcome, err := lib.matcher.Match(t.Context(), show.ID); err != nil || outcome != identify.OutcomeMatched {
			t.Fatalf("Match(%s) = %s, %v", ptr.ValueOrZero(show.Name), outcome, err)
		}
	}
	if passes != 0 || wakes != 2 {
		t.Errorf("the matches ran %d passes and woke the owed passes' sweep %d times; want none run and two wakes",
			passes, wakes)
	}
	if ran, err := lib.pass.IdentifyOwed(t.Context()); err != nil || ran != 1 || passes != 1 {
		t.Errorf("the owed passes ran %d libraries' passes (%d in all), %v; want the one library's, once",
			ran, passes, err)
	}
	if ran, err := lib.pass.IdentifyOwed(t.Context()); err != nil || ran != 0 {
		t.Errorf("owed passes run again ran %d, %v; want none still owed", ran, err)
	}
	for path, file := range lib.titlesByPath() {
		if file.Filing != identify.FilingListed {
			t.Errorf("%s is filed %s after the owed pass, want listed by its series' listing", path, file.Filing)
		}
	}
}

// The files of placedByNameLibrary: a numbered episode, one with no number its name places, and
// an extra of the same name.
const (
	placedByNameNumbered = "Orbitorama/Season 02/Orbitorama - S02E10.mkv"
	placedByNameUnplaced = "Orbitorama/Orbitorama - Comet Pilott 3000.mkv" // misspelled, as a rip names it
	placedByNameBonus    = "Orbitorama/Extras/Comet Pilot 3000.mkv"
)

// placedByNameLibrary is a series library whose series is matched and its passes run, holding an
// episode with no number that its name places in season 1 as a guess (D307 §2).
func placedByNameLibrary(t *testing.T) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "series", "tmdb")
	listed := orbitorama
	listed.SeasonNumbers = []int{0, 1, 2}
	lib.provider.Searches[identifytest.SearchKey("Orbitorama", 0)] = []identify.Candidate{lib.provider.Add(listed)}
	lib.provider.Seasons["7615/1"] = []identify.Episode{{ID: "1001", Season: 1, Number: 1, Name: "Comet Pilot 3000"}}
	lib.provider.Seasons["7615/2"] = []identify.Episode{{ID: "1101", Season: 2, Number: 10, Name: "A Comet of My Own"}}
	lib.provider.Seasons["7615/0"] = []identify.Episode{}
	lib.scanAll(placedByNameNumbered, placedByNameUnplaced, placedByNameBonus)
	series := lib.onlySeries()
	if outcome, err := lib.matcher.Match(t.Context(), series.ID); err != nil || outcome != identify.OutcomeMatched {
		t.Fatalf("Match(series) = %s, %v", outcome, err)
	}
	lib.runOwedPasses()
	return lib
}

// An episode with no number, in no specials or extras folder, is placed by its name in the one
// episode of every season its series lists whose name is its own, or nearly, and marked a guess;
// the series has every season fetched for it. One in an extras folder stays a special (D306 §2,
// D307 §2).
func TestSeries_AnUnnumberedEpisodeIsPlacedByItsNameAcrossEverySeason(t *testing.T) {
	t.Parallel()
	lib := placedByNameLibrary(t)
	numbered, unplaced, bonus := placedByNameNumbered, placedByNameUnplaced, placedByNameBonus
	if !slices.Contains(lib.provider.Calls, "season 7615/1") {
		t.Errorf("calls %v; want season 1 fetched, which no numbered episode is in", lib.provider.Calls)
	}
	titles := lib.titlesByPath()
	placed := titles[unplaced]
	if titleName(placed) != "Comet Pilot 3000" || !lib.isGuessed(placed.WorkID) {
		t.Errorf("the unnumbered episode = %+v named %q, guessed %v; want S01E01 by its name, a guess", placed,
			titleName(placed), lib.isGuessed(placed.WorkID))
	}
	place := storetest.QueryRaw(t, lib.database.Path(), fmt.Sprintf(
		"SELECT grp.number || 'x' || number_in_group FROM position LEFT JOIN grp ON grp.id = position.grp_id "+
			"WHERE work_id = %d", placed.WorkID))
	if !slices.Equal(place, []string{"1x1"}) {
		t.Errorf("the unnumbered episode is at %v; want 1x1, Comet Pilot 3000's place", place)
	}
	if titles[bonus].WorkID == placed.WorkID || lib.isGuessed(titles[bonus].WorkID) {
		t.Errorf("the extra was placed as the episode, or guessed; want it a special of its own")
	}
	if lib.isGuessed(titles[numbered].WorkID) {
		t.Error("a numbered episode is marked a guess")
	}
}

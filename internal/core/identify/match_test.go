// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

var (
	leon = identify.Metadata{
		ID: "101", Name: "Variety", OriginalName: "Variety", Year: 1994,
		RuntimeMs: 110 * 60_000, Overview: "A hitman takes in a girl.", IDs: map[string]string{"imdb": "tt0345678"},
	}
	heat = identify.Metadata{ID: "9019", Name: "Metropolis", OriginalName: "Metropolis", Year: 1995, RuntimeMs: 170 * 60_000}
)

// matchFileTitle runs the matcher on the title of the file at rel.
func (lib *testLibrary) matchFileTitle(rel string) identify.Outcome {
	lib.t.Helper()
	file, found := lib.titlesByPath()[rel]
	if !found {
		lib.t.Fatalf("no file %s", rel)
	}
	outcome, err := lib.matcher.Match(lib.t.Context(), file.TitleID)
	if err != nil {
		lib.t.Fatalf("Match(%s): %v", rel, err)
	}
	return outcome
}

func TestMatch_ASingleResultMatchingNameAndYearIsAcceptedAndReplayedFromTheStoredResponse(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Variety", 1994)] = []identify.Candidate{
		lib.provider.Add(leon), {ID: "7", Name: "Léon Morin, Priest", Year: 1961},
	}
	const rel = "Variety (1994)/Variety (1994).mkv"
	report := lib.scan(videoRecord(rel))
	if len(report.AwaitingMatch) != 1 || lib.titlesByPath()[rel].Filing != identify.FilingUnidentified {
		t.Fatalf("before matching: %+v, file %+v; want it awaiting its match", report, lib.titlesByPath()[rel])
	}
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s, want matched (its original name and year are the file's)", outcome)
	}
	checkLeon := func(when string) {
		t.Helper()
		file := lib.titlesByPath()[rel]
		if titleName(file) != leon.Name || file.IdentificationState != identify.IdentificationIdentified ||
			file.FollowedSource == nil || *file.FollowedSource != "tmdb" ||
			file.Filing != identify.FilingListed || file.Year == nil || *file.Year != 1994 {
			t.Errorf("%s: %+v (%s), want TMDB's film, listed", when, file, titleName(file))
		}
	}
	checkLeon("after the match")
	calls := len(lib.provider.Calls)
	report = lib.scan(videoRecord(rel))
	checkLeon("after another pass")
	if len(lib.provider.Calls) != calls || len(report.AwaitingMatch) != 0 {
		t.Errorf("another pass called the provider (%v) or queued the film again (%v)", lib.provider.Calls[calls:],
			report.AwaitingMatch)
	}
}

// D232: the candidate is compared by the one identity fold, so a file named without the
// accents its work's name has matches it.
func TestMatch_ACandidateIsNamedAsTheFileIsByTheOneIdentityFold(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Variety", 1994)] = []identify.Candidate{
		lib.provider.Add(leon), {ID: "7", Name: "Léon Morin, Priest", Year: 1961},
	}
	lib.scan(videoRecord("Variety (1994).mkv"))
	if outcome := lib.matchFileTitle("Variety (1994).mkv"); outcome != identify.OutcomeMatched {
		t.Errorf("outcome %s, want matched: Leon Morin is Léon Morin's original name without its accent", outcome)
	}
}

// isGuessed is whether an item's match is marked a guess.
func (lib *testLibrary) isGuessed(itemID int64) bool {
	lib.t.Helper()
	isGuessed, err := lib.database.Read().IsGuessedMatch(lib.t.Context(), itemID)
	if err != nil {
		lib.t.Fatal(err)
	}
	return isGuessed
}

// A search with no candidate of the file's name in or a year off its year is never guessed: it waits
// for a person, its results kept for them (D172 §3, which D301 leaves standing for this).
func TestMatch_AnUnsettledSearchIsNeverGuessed(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	// Two remakes, neither within a year of the one the file names. One a year off would be taken
	// (TestMatch_AYearOffOrASubtitleStillFindsTheOneWork).
	lib.provider.Searches[identifytest.SearchKey("The General", 2004)] = []identify.Candidate{
		{ID: "1", Name: "The General", Year: 1972}, {ID: "2", Name: "The General", Year: 2002},
	}
	lib.scan(videoRecord("The General (2004).mkv"))
	if outcome := lib.matchFileTitle("The General (2004).mkv"); outcome != identify.OutcomeAmbiguous {
		t.Errorf("The General (2004).mkv: outcome %s, want ambiguous", outcome)
	}
	lib.scan(videoRecord("The General (2004).mkv"))
	solaris := lib.titlesByPath()["The General (2004).mkv"]
	if solaris.IdentificationState != identify.IdentificationAmbiguous || solaris.Filing != identify.FilingListed ||
		solaris.FollowedSource != nil {
		t.Errorf("after another pass: %+v; want ambiguous, listed on local data", solaris)
	}
	kept, err := lib.cache.Read().ListMatchCandidates(t.Context(), solaris.TitleID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, candidate := range kept {
		ids = append(ids, candidate.ProviderID)
	}
	if !slices.Equal(ids, []string{"1", "2"}) {
		t.Errorf("kept candidates %v, want the search's, in its order", ids)
	}
}

// Several candidates of the file's name are settled by a guess, marked as one until a person
// chooses: with no year the oldest, with one the most voted, and a film with no year and one
// candidate is a guess too (D301; the owner's NAS, 2026-10-15).
func TestMatch_SeveralOfOneNameAreSettledByAGuessMarkedAsOne(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	oldest := lib.provider.Add(identify.Metadata{ID: "11", Name: "The Kid", Year: 1994})
	lib.provider.Searches[identifytest.SearchKey("The Kid", 0)] = []identify.Candidate{
		{ID: "12", Name: "The Kid", Year: 2019, Votes: 9000},
		oldest,
		{ID: "13", Name: "The Kid", Year: 0},
	}
	disneys := lib.provider.Add(identify.Metadata{ID: "21", Name: "The Thief of Bagdad", Year: 1992})
	disneys.Votes = 11000
	lib.provider.Searches[identifytest.SearchKey("The Thief of Bagdad", 1992)] = []identify.Candidate{
		{ID: "22", Name: "The Thief of Bagdad", Year: 1992, Votes: 40}, disneys,
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 0)] = []identify.Candidate{lib.provider.Add(heat)}
	files := map[string]string{"The Kid.mkv": "11", "The Thief of Bagdad (1992).mkv": "21", "Metropolis.mkv": "949"}
	lib.scan(videoRecord("The Kid.mkv"), videoRecord("The Thief of Bagdad (1992).mkv"), videoRecord("Metropolis.mkv"))
	for rel, want := range files {
		if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
			t.Errorf("%s: outcome %s, want matched by a guess", rel, outcome)
		}
		title := lib.titlesByPath()[rel]
		if !slices.Contains(lib.provider.Calls, "fetch "+want) {
			t.Errorf("%s: %s was not fetched as its match: %v", rel, want, lib.provider.Calls)
		}
		if !lib.isGuessed(title.TitleID) {
			t.Errorf("%s is not marked a guess", rel)
		}
	}
	// A person's choice is no guess, even of the same work.
	lion := lib.titlesByPath()["The Kid.mkv"]
	if err := lib.chooser.Choose(t.Context(), lion.TitleID, "tmdb", "11"); err != nil {
		t.Fatal(err)
	}
	if lib.isGuessed(lion.TitleID) {
		t.Error("The Kid is still marked a guess after a person chose it")
	}
}

// The owner's NAS, 2026-10-13: films with one plain answer waited for a person's choice. A year
// one off, a subtitle the release name carries, an ampersand spelled out, each still finds the one
// work; two within a year stay a person's choice.
func TestMatch_AYearOffOrASubtitleStillFindsTheOneWork(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Larkhollow", 1997)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "10", Name: "Larkhollow", Year: 1998}),
	}
	lib.provider.Searches[identifytest.SearchKey("Wild Rex 2 The Dust Road", 1981)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "11", Name: "Wild Rex 2", Year: 1981}),
		{ID: "12", Name: "Wild Rex", Year: 1979},
	}
	lib.provider.Searches[identifytest.SearchKey("Wit and Wilfulness", 2005)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "13", Name: "Wit & Wilfulness", Year: 2005}),
		{ID: "14", Name: "Wit and Wilfulness", Year: 1940},
	}
	// Neither is named Wings, and two of its year are no single work under another name (D307 §1).
	lib.provider.Searches[identifytest.SearchKey("Wings", 1986)] = []identify.Candidate{
		{ID: "15", Name: "Wing", Year: 1986}, {ID: "18", Name: "Wing Nation", Year: 1986},
	}
	lib.provider.Searches[identifytest.SearchKey("Quillfen", 2014)] = []identify.Candidate{
		{ID: "16", Name: "Quillfen", Year: 2013}, {ID: "17", Name: "Quillfen", Year: 2015},
	}
	files := map[string]identify.Outcome{
		"Larkhollow 1997 REMASTERED JAPANESE 1080p BluRay H264 AAC-OSPREY.mkv": identify.OutcomeMatched,
		"Wild.Rex.2.The.Dust.Road.1981.1080p.BluRay.H264.AAC-THISTLE.mp4":     identify.OutcomeMatched,
		"Wit.and.Wilfulness.2005.1080p.BluRay.x264.anoXmous_.mp4":            identify.OutcomeMatched,
		"Wings (1986).mkv":  identify.OutcomeAmbiguous,
		"Quillfen (2014).mkv": identify.OutcomeAmbiguous,
	}
	var records []nodeproto.ScanRecord
	for path := range files {
		records = append(records, videoRecord(path))
	}
	lib.scan(records...)
	for path, want := range files {
		if outcome := lib.matchFileTitle(path); outcome != want {
			t.Errorf("%s: outcome %s; want %s", path, outcome, want)
		}
	}
}

// A work released under another name is guessed from the provider's search (D307 §1): with a year,
// the one candidate of that year; with none, the oldest of the one or two found. A search of words
// that found more is a person's choice.
func TestMatch_AWorkUnderAnotherNameIsGuessed(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Kumo no Michi", 1971)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "20", Name: "The Cloud Road", Year: 1971}),
		{ID: "21", Name: "The Cloud Road Returns", Year: 1985},
	}
	lib.provider.Searches[identifytest.SearchKey("Hoshi no Mori", 0)] = []identify.Candidate{
		{ID: "23", Name: "Forest of Stars", Year: 2016},
		lib.provider.Add(identify.Metadata{ID: "22", Name: "Star Forest", Year: 2007}),
	}
	lib.provider.Searches[identifytest.SearchKey("Lanterns", 0)] = []identify.Candidate{
		{ID: "24", Name: "Lanterns of Iron", Year: 1990},
		{ID: "25", Name: "Paper Lanterns", Year: 1999},
		{ID: "26", Name: "The Lantern Keeper", Year: 2011},
	}
	files := map[string]identify.Outcome{
		"Kumo no Michi (1971).mkv": identify.OutcomeMatched,
		"Hoshi no Mori.mkv":        identify.OutcomeMatched,
		"Lanterns.mkv":             identify.OutcomeAmbiguous,
	}
	var records []nodeproto.ScanRecord
	for path := range files {
		records = append(records, videoRecord(path))
	}
	lib.scan(records...)
	for path, want := range files {
		if outcome := lib.matchFileTitle(path); outcome != want {
			t.Errorf("%s: outcome %s; want %s", path, outcome, want)
		}
	}
	for _, path := range []string{"Kumo no Michi (1971).mkv", "Hoshi no Mori.mkv"} {
		if title := lib.titlesByPath()[path]; !lib.isGuessed(title.TitleID) {
			t.Errorf("%s is matched but not marked a guess", path)
		}
	}
	if name := titleName(lib.titlesByPath()["Hoshi no Mori.mkv"]); name != "Star Forest" {
		t.Errorf("Hoshi no Mori is matched to %q; want Star Forest, the older of the two", name)
	}
}

// A film left ambiguous under earlier rules, whose kept results the rules now settle, is owed its
// first match again, and the next match accepts it; one they do not settle, and a person's choice,
// stay as they were (the owner's NAS, 2026-10-13).
func TestMatch_AnAmbiguousFilmTheRulesNowSettleIsMatchedAgain(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	blue := lib.provider.Add(identify.Metadata{ID: "10", Name: "Larkhollow", Year: 1998})
	lib.provider.Searches[identifytest.SearchKey("Larkhollow", 1997)] = []identify.Candidate{
		blue, {ID: "11", Name: "Larkhollow", Year: 1998},
	}
	lib.provider.Searches[identifytest.SearchKey("Quillfen", 2014)] = []identify.Candidate{
		{ID: "16", Name: "Quillfen", Year: 2013}, {ID: "17", Name: "Quillfen", Year: 2015},
	}
	lib.scan(videoRecord("Larkhollow (1997).mkv"), videoRecord("Quillfen (2014).mkv"))
	for _, rel := range []string{"Larkhollow (1997).mkv", "Quillfen (2014).mkv"} {
		if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeAmbiguous {
			t.Fatalf("%s: %s; want ambiguous to begin with", rel, outcome)
		}
	}
	// As earlier rules left it: its kept results now hold its one answer, a year off.
	blueTitle := lib.titlesByPath()["Larkhollow (1997).mkv"].TitleID
	err := lib.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		if err := queries.DeleteMatchCandidates(t.Context(), blueTitle); err != nil {
			return err //nolint:wrapcheck // the test reads it as is
		}
		year := int64(1998)
		return queries.InsertMatchCandidate(t.Context(), cachegen.InsertMatchCandidateParams{
			ItemID: blueTitle, Rank: 0, Provider: "tmdb", ProviderID: "10", Name: new("Larkhollow"), Year: &year,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	lib.provider.Searches[identifytest.SearchKey("Larkhollow", 1997)] = []identify.Candidate{blue}
	returned, err := lib.matcher.ReconsiderAmbiguous(t.Context())
	if err != nil || returned != 1 {
		t.Fatalf("ReconsiderAmbiguous = %d, %v; want Larkhollow alone returned to its first match", returned, err)
	}
	if outcome := lib.matchFileTitle("Larkhollow (1997).mkv"); outcome != identify.OutcomeMatched {
		t.Errorf("Larkhollow: %s; want matched once returned", outcome)
	}
	quillfen := lib.titlesByPath()["Quillfen (2014).mkv"]
	if quillfen.IdentificationState != identify.IdentificationAmbiguous {
		t.Errorf("Quillfen is %s; two a year off its own stay a person's choice", quillfen.IdentificationState)
	}
}

func TestMatch_AProviderIDInHandSkipsTheSearch(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Add(heat)
	lib.provider.Add(leon)
	lib.provider.TMDBIDsByOtherID["tt0345678"] = leon.ID
	lib.scan(videoRecord("Metropolis {tmdb-9019}.mkv"), videoRecord("Variety [imdbid-tt0345678].mkv"))
	lib.matchFileTitle("Metropolis {tmdb-9019}.mkv")
	lib.matchFileTitle("Variety [imdbid-tt0345678].mkv")
	want := []string{"fetch 949", "find imdb tt0345678", "fetch 101"}
	if !slices.Equal(lib.provider.Calls, want) {
		t.Errorf("calls %v, want %v: no search", lib.provider.Calls, want)
	}
	if file := lib.titlesByPath()["Variety [imdbid-tt0345678].mkv"]; titleName(file) != leon.Name {
		t.Errorf("the IMDb-tagged file is %q", titleName(file))
	}
}

func TestMatch_NothingFoundIsRecordedAndNotSearchedAgain(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo_then_tmdb")
	const rel = "home video (2011).mkv"
	lib.scan(videoRecord(rel))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeNoMatch {
		t.Fatalf("outcome %s, want no match", outcome)
	}
	report := lib.scan(videoRecord(rel))
	file := lib.titlesByPath()[rel]
	why := identify.NoMatchReason("nfo_then_tmdb")
	if file.Filing != identify.FilingUnidentified || file.FilingReason == nil || *file.FilingReason != why {
		t.Errorf("file %+v, want unidentified, saying why", file)
	}
	if len(report.AwaitingMatch) != 0 {
		t.Errorf("the pass queued it again: %v", report.AwaitingMatch)
	}
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeNothingToDo || len(lib.provider.Calls) != 1 {
		t.Errorf("matching again: %s after calls %v; want nothing done, one search", outcome, lib.provider.Calls)
	}
}

func TestMatch_ASecondTitleForTheSameWorkJoinsTheFirst(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(heat)}
	lib.scan(videoRecord("Metropolis {tmdb-9019}.mkv"), videoRecord("other/Metropolis (1995).mkv"))
	lib.matchFileTitle("Metropolis {tmdb-9019}.mkv")
	lib.matchFileTitle("other/Metropolis (1995).mkv")
	files := lib.titlesByPath()
	first, second := files["Metropolis {tmdb-9019}.mkv"], files["other/Metropolis (1995).mkv"]
	if first.TitleID != second.TitleID {
		t.Errorf("titles %d and %d; one work found twice should be one title with two versions",
			first.TitleID, second.TitleID)
	}
	lib.scan(videoRecord("Metropolis {tmdb-9019}.mkv"), videoRecord("other/Metropolis (1995).mkv"))
	if files := lib.titlesByPath(); files["other/Metropolis (1995).mkv"].TitleID != first.TitleID {
		t.Error("another pass split the joined title")
	}
}

func TestMatch_AnOverrideSurvivesTheMatchAndReDerivation(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(heat)}
	const rel = "Metropolis (1995).mkv"
	lib.scan(videoRecord(rel))
	titleID := lib.titlesByPath()[rel].TitleID
	err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.InsertOverrideIfAbsent(t.Context(), sqlcgen.InsertOverrideIfAbsentParams{
			ItemID: titleID, Field: identify.FieldName, OverrideValue: "Metropolis (director's copy)", Origin: "user",
			SetAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	lib.matchFileTitle(rel)
	lib.scan(videoRecord(rel))
	file := lib.titlesByPath()[rel]
	if titleName(file) != "Metropolis (director's copy)" || file.FollowedSource == nil || *file.FollowedSource != "tmdb" {
		t.Errorf("file %+v named %q; want TMDB's film under the person's name", file, titleName(file))
	}
}

func TestMatch_AProvidersRefusalReturnsForTheQueueToActOn(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.scan(videoRecord("Metropolis (1995).mkv"))
	lib.provider.SetFailure(&outbound.RateLimitedError{Provider: "TMDB", RetryAfter: 3})
	_, err := lib.matcher.Match(t.Context(), lib.titlesByPath()["Metropolis (1995).mkv"].TitleID)
	if limited, isLimited := errors.AsType[*outbound.RateLimitedError](err); !isLimited || limited.RetryAfter != 3 {
		t.Errorf("Match = %v, want the provider's RateLimitedError", err)
	}
	file := lib.titlesByPath()["Metropolis (1995).mkv"]
	if file.IdentificationState != identify.IdentificationUnidentified || file.FollowedSource != nil {
		t.Errorf("a refused call changed the title: %+v", file)
	}
}

func TestHandle_AsksTheProviderWithTheJobsImportanceAndPassesOnItsParking(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.scan(videoRecord("Metropolis (1995).mkv"), videoRecord("Nosferatu (1998).mkv"), videoRecord("Wing (1979).mkv"))
	titles := lib.titlesByPath()
	for path, check := range map[string]struct {
		priority int64
		want     providers.Importance
	}{
		"Metropolis (1995).mkv":  {0, providers.Background},
		"Nosferatu (1998).mkv": {jobs.PriorityPersonsRequest, providers.PersonsRequest},
	} {
		lib.provider.Importances = nil
		job := jobs.Job{
			Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: titles[path].TitleID}, Priority: check.priority,
		}
		if err := lib.matcher.Handle(t.Context(), job); err != nil {
			t.Fatal(err)
		}
		if len(lib.provider.Importances) == 0 || lib.provider.Importances[0] != check.want {
			t.Errorf("a job of priority %d asked the provider as %v, want %s", check.priority,
				lib.provider.Importances, check.want)
		}
	}
	parking := jobs.Parking{Destination: jobs.DestinationTMDBAPI, Reason: "unreachable"}
	lib.provider.SetFailure(jobs.Park(parking, errors.New("connection refused")))
	alien := jobs.Job{Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: titles["Wing (1979).mkv"].TitleID}}
	if parked, isParked := jobqueue.ParkingOf(lib.matcher.Handle(t.Context(), alien)); !isParked || parked != parking {
		t.Errorf("the provider parked the fetch as %+v (%v); want its parking passed to the queue", parked, isParked)
	}
}

func TestPass_ATMDBOnlyLibraryIgnoresNFOFiles(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	const body = "<movie><title>Not What The Path Says</title><year>1995</year><tmdbid>9019</tmdbid>" +
		"<lockdata>true</lockdata></movie>"
	report := lib.scan(videoRecord("Metropolis (1995).mkv"), nfoRecord("Metropolis (1995).nfo", body))
	file := lib.titlesByPath()["Metropolis (1995).mkv"]
	if titleName(file) != "Metropolis" || file.FollowedSource != nil || file.Filing != identify.FilingUnidentified ||
		len(report.AwaitingMatch) != 1 {
		t.Errorf("file %+v named %q, awaiting %v; want the path's name, awaiting TMDB (D183: ignore NFO files)",
			file, titleName(file), report.AwaitingMatch)
	}
	ids, err := lib.database.Read().ListItemProviderIDs(t.Context(), file.TitleID)
	if err != nil || len(ids) != 0 {
		t.Errorf("ids %+v (%v); the NFO's id should not have been read", ids, err)
	}
	overrides, err := lib.database.Read().ListOverrides(t.Context(), file.TitleID)
	if err != nil || len(overrides) != 0 {
		t.Errorf("overrides %+v (%v); the NFO's locked fields should not have been read", overrides, err)
	}
}

// Principles §22, D211: what a provider answers is clean where it enters, whoever wrote it.
func TestMatch_AProvidersTextIsCleanWhereItEnters(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	hostile := identify.Metadata{ID: "9019", Name: "Metropolis\u202Evkm.exe", Year: 1995, Overview: "One.\u200B\n\n\n\nTwo."}
	lib.provider.Add(hostile)
	lib.scan(videoRecord("Metropolis {tmdb-9019}.mkv"))
	lib.matchFileTitle("Metropolis {tmdb-9019}.mkv")
	if got := titleName(lib.titlesByPath()["Metropolis {tmdb-9019}.mkv"]); got != "Heatvkm.exe" {
		t.Errorf("stored name %q; want the override gone", got)
	}
}

// Principles §22: a person's override is clean where it enters.
func TestChooser_AnOverrideIsCleanWhereItEnters(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
	lib.scan(videoRecord("Metropolis (1995).mkv"))
	titleID := lib.titlesByPath()["Metropolis (1995).mkv"].TitleID
	person := storetest.InsertUser(t, lib.database, "alice", "admin")
	err := lib.chooser.SetOverride(t.Context(), titleID, identify.FieldName, "Mine\u2067\u0000", person)
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := lib.database.Read().ListOverrides(t.Context(), titleID)
	if err != nil || len(overrides) != 1 || overrides[0].OverrideValue != "Mine" {
		t.Errorf("stored override %+v, %v; want it clean", overrides, err)
	}
}

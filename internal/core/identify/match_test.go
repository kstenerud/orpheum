// SPDX-License-Identifier: GPL-3.0-or-later

package identify_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
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

func TestMatch_AnAmbiguousSearchIsNeverGuessed(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	// Two remakes a year apart, neither in the year the file names; and a file with no year.
	lib.provider.Searches[identifytest.SearchKey("The General", 2003)] = []identify.Candidate{
		{ID: "1", Name: "The General", Year: 1972}, {ID: "2", Name: "The General", Year: 2002},
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 0)] = []identify.Candidate{lib.provider.Add(heat)}
	lib.scan(videoRecord("The General (2003).mkv"), videoRecord("Metropolis.mkv"))
	for _, rel := range []string{"The General (2003).mkv", "Metropolis.mkv"} {
		if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeAmbiguous {
			t.Errorf("%s: outcome %s, want ambiguous", rel, outcome)
		}
	}
	lib.scan(videoRecord("The General (2003).mkv"), videoRecord("Metropolis.mkv"))
	solaris := lib.titlesByPath()["The General (2003).mkv"]
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
	if slices.Contains(lib.provider.Calls, "fetch 949") {
		t.Error("a film with no year was fetched: its single result was taken as its match")
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

func TestHandle_TellsTheQueueHowToWait(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.scan(videoRecord("Metropolis (1995).mkv"))
	job := jobs.Job{
		Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: lib.titlesByPath()["Metropolis (1995).mkv"].TitleID},
	}
	lib.provider.SetFailure(&outbound.RateLimitedError{Provider: "TMDB", RetryAfter: time.Minute})
	err := lib.matcher.Handle(t.Context(), job)
	retry, isRetry := errors.AsType[*jobs.RetryAtError](err)
	if !isRetry || !retry.At.Equal(lib.clock.Now().Add(time.Minute)) {
		t.Errorf("rate limited: %v, want RetryAtError a minute from now", err)
	}
	lib.provider.SetFailure(fmt.Errorf("tmdb: %w", identify.ErrNoKey))
	if err := lib.matcher.Handle(t.Context(), job); err == nil || !errors.Is(err, identify.ErrNoKey) {
		t.Errorf("no key: %v, want it blocked", err)
	}
	lib.provider.SetFailure(nil)
	if err := lib.matcher.Handle(t.Context(), job); err != nil {
		t.Errorf("a search with no result: %v, want the job done", err)
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

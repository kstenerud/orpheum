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
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

var (
	theo = identify.Metadata{
		ID: "101", Name: "Théo: The Guardian", OriginalName: "Théo", Year: 1994,
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
	outcome, err := lib.matcher.Match(lib.t.Context(), file.WorkID)
	if err != nil {
		lib.t.Fatalf("Match(%s): %v", rel, err)
	}
	return outcome
}

func TestMatch_ASingleResultMatchingNameAndYearIsAcceptedAndReplayedFromTheStoredResponse(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Théo", 1994)] = []identify.Candidate{
		lib.provider.Add(theo), {ID: "7", Name: "Léon Morin, Priest", Year: 1961},
	}
	const rel = "Théo (1994)/Théo (1994).mkv"
	report := lib.scan(videoRecord(rel))
	if len(report.AwaitingMatch) != 1 || lib.titlesByPath()[rel].Filing != identify.FilingUnidentified {
		t.Fatalf("before matching: %+v, file %+v; want it awaiting its match", report, lib.titlesByPath()[rel])
	}
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s, want matched (its original name and year are the file's)", outcome)
	}
	checkTheo := func(when string) {
		t.Helper()
		file := lib.titlesByPath()[rel]
		if titleName(file) != theo.Name ||
			ptr.ValueOrZero(file.IdentificationState) != identify.IdentificationIdentified ||
			file.FollowedSource == nil || *file.FollowedSource != "tmdb" ||
			file.Filing != identify.FilingListed || file.Year == nil || *file.Year != 1994 {
			t.Errorf("%s: %+v (%s), want TMDB's film, listed", when, file, titleName(file))
		}
	}
	checkTheo("after the match")
	calls := len(lib.provider.Calls)
	report = lib.scan(videoRecord(rel))
	checkTheo("after another pass")
	if len(lib.provider.Calls) != calls || len(report.AwaitingMatch) != 0 {
		t.Errorf("another pass called the provider (%v) or queued the film again (%v)", lib.provider.Calls[calls:],
			report.AwaitingMatch)
	}
}

// D232: the candidate is compared by the one identity fold, so a file named without the
// accents its work's name has matches it.
func TestMatch_ACandidateIsNamedAsTheFileIsByTheOneIdentityFold(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Theo", 1994)] = []identify.Candidate{
		lib.provider.Add(theo), {ID: "7", Name: "Léon Morin, Priest", Year: 1961},
	}
	lib.scan(videoRecord("Theo (1994).mkv"))
	if outcome := lib.matchFileTitle("Theo (1994).mkv"); outcome != identify.OutcomeMatched {
		t.Errorf("outcome %s, want matched: Theo is Théo's original name without its accent", outcome)
	}
}

// A leading article is no part of a work's name: the file's "The Girl in the Attic" is TMDB's one "Girl
// in the Attic" of its year, and a file named without it matches a work named with it (the owner's
// staging, 2026-10-21). An article alone is a name.
func TestMatch_ALeadingArticleIsNoPartOfTheName(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		file, query, candidate string
		year                   int
	}{
		{"The Girl in the Attic (2016).mkv", "The Girl in the Attic", "Girl in the Attic", 2016},
		{"Navigator (2015).mkv", "Navigator", "The Navigator", 2015},
		{"A Fool There Was (2018).mkv", "A Fool There Was", "Quiet Place", 2018},
	} {
		lib := newTestLibrary(t, "movie", "tmdb")
		year := each.year
		lib.provider.Searches[identifytest.SearchKey(each.query, year)] = []identify.Candidate{
			lib.provider.Add(identify.Metadata{
				ID: "413337", Name: each.candidate, OriginalName: each.candidate,
				Year: year,
			}),
			{ID: "945222", Name: "Mira Holloway: " + each.candidate, Year: year},
		}
		lib.scan(videoRecord(each.file))
		if outcome := lib.matchFileTitle(each.file); outcome != identify.OutcomeMatched {
			t.Errorf("%s: outcome %s, want matched to %q", each.file, outcome, each.candidate)
		}
	}
	if workName := identify.WorkName("The"); workName != "the" {
		t.Errorf("WorkName(\"The\") = %q; an article alone is the whole name", workName)
	}
}

// A name matched both as it is and with an article set aside is the one named exactly: the file's
// "Crowd" is TMDB's "Crowd", not the better voted "The Crowd" of its year, and the
// other way round; matched clearly, no guess (the owner's staging, 2026-10-27).
func TestMatch_ANameMatchedExactlyWinsOverOneMatchedWithAnArticleAside(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		file, query, want string
	}{
		{"Crowd (2022).mkv", "Crowd", "31"},
		{"The Crowd (2022).mkv", "The Crowd", "32"},
	} {
		lib := newTestLibrary(t, "movie", "tmdb")
		plain := lib.provider.Add(identify.Metadata{
			ID: "31", Name: "Crowd", OriginalName: "Crowd", Year: 2022,
		})
		plain.Votes = 50
		withArticle := lib.provider.Add(identify.Metadata{
			ID: "32", Name: "The Crowd", OriginalName: "The Crowd", Year: 2022,
		})
		withArticle.Votes = 5000
		if each.want == "32" {
			plain.Votes, withArticle.Votes = 5000, 50
		}
		lib.provider.Searches[identifytest.SearchKey(each.query, 2022)] = []identify.Candidate{withArticle, plain}
		lib.scan(videoRecord(each.file))
		if outcome := lib.matchFileTitle(each.file); outcome != identify.OutcomeMatched {
			t.Fatalf("%s: outcome %s, want matched", each.file, outcome)
		}
		if !slices.Contains(lib.provider.Calls, "fetch "+each.want) {
			t.Errorf("%s: %s was not fetched as its match: %v", each.file, each.want, lib.provider.Calls)
		}
		if lib.isGuessed(lib.titlesByPath()[each.file].WorkID) {
			t.Errorf("%s is marked a guess; want it matched clearly by its exact name", each.file)
		}
	}
}

// The year the file or its NFO gives is the stronger cue: the exact name settles only among
// candidates of the year, so the file's "Crowd" (2022) is "The Crowd" of 2022, not an
// "Crowd" of 2021; fuzzier matches only once the year settles nothing (the owner, 2026-10-27).
func TestMatch_TheYearIsAStrongerCueThanTheExactName(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Crowd", 2022)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "41", Name: "Crowd", OriginalName: "Crowd", Year: 2021}),
		lib.provider.Add(identify.Metadata{
			ID: "42", Name: "The Crowd", OriginalName: "The Crowd", Year: 2022,
		}),
	}
	const rel = "Crowd (2022).mkv"
	lib.scan(videoRecord(rel))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s, want matched", outcome)
	}
	if !slices.Contains(lib.provider.Calls, "fetch 42") {
		t.Errorf("the 2022 film was not fetched as its match: %v", lib.provider.Calls)
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
	lib := newTestLibrary(t, "movie", "tmdb")
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
	if ptr.ValueOrZero(solaris.IdentificationState) != identify.IdentificationAmbiguous ||
		solaris.Filing != identify.FilingListed ||
		solaris.FollowedSource != nil {
		t.Errorf("after another pass: %+v; want ambiguous, listed on local data", solaris)
	}
	kept, err := lib.cache.Read().ListMatchCandidates(t.Context(), solaris.WorkID)
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
// chooses: with no year the oldest of those near in votes (D412 §3), with one the most voted, and
// a film with no year and one candidate is a guess too (D301; the owner's NAS, 2026-10-15).
func TestMatch_SeveralOfOneNameAreSettledByAGuessMarkedAsOne(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	oldest := lib.provider.Add(identify.Metadata{ID: "11", Name: "The Marten King", Year: 1994})
	oldest.Votes = 2000 // within ten times the remake's, as the real films' are (D412 §3)
	lib.provider.Searches[identifytest.SearchKey("The Marten King", 0)] = []identify.Candidate{
		{ID: "12", Name: "The Marten King", Year: 2019, Votes: 9000},
		oldest,
		{ID: "13", Name: "The Marten King", Year: 0},
	}
	disneys := lib.provider.Add(identify.Metadata{ID: "21", Name: "Alcazar", Year: 1992})
	disneys.Votes = 11000
	lib.provider.Searches[identifytest.SearchKey("Alcazar", 1992)] = []identify.Candidate{
		{ID: "22", Name: "Alcazar", Year: 1992, Votes: 40}, disneys,
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 0)] = []identify.Candidate{lib.provider.Add(heat)}
	files := map[string]string{"The Marten King.mkv": "11", "Alcazar (1992).mkv": "21", "Metropolis.mkv": "949"}
	lib.scan(videoRecord("The Marten King.mkv"), videoRecord("Alcazar (1992).mkv"), videoRecord("Metropolis.mkv"))
	for rel, want := range files {
		if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
			t.Errorf("%s: outcome %s, want matched by a guess", rel, outcome)
		}
		title := lib.titlesByPath()[rel]
		if !slices.Contains(lib.provider.Calls, "fetch "+want) {
			t.Errorf("%s: %s was not fetched as its match: %v", rel, want, lib.provider.Calls)
		}
		if !lib.isGuessed(title.WorkID) {
			t.Errorf("%s is not marked a guess", rel)
		}
	}
	// A person's choice is no guess, even of the same work.
	lion := lib.titlesByPath()["The Marten King.mkv"]
	if err := lib.chooser.Choose(t.Context(), lion.WorkID, "tmdb", "11"); err != nil {
		t.Fatal(err)
	}
	if lib.isGuessed(lion.WorkID) {
		t.Error("The Marten King is still marked a guess after a person chose it")
	}
}

// The owner's NAS, 2026-10-13: films with one plain answer waited for a person's choice. A year
// one off, a subtitle the release name carries, an ampersand spelled out, each still finds the one
// work; two within a year stay a person's choice.
func TestMatch_AYearOffOrASubtitleStillFindsTheOneWork(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Hollow Violet", 1997)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "10", Name: "Hollow Violet", Year: 1998}),
	}
	lib.provider.Searches[identifytest.SearchKey("Wild Rex 2 The Dust Road", 1981)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "11", Name: "Wild Rex 2", Year: 1981}),
		{ID: "12", Name: "Wild Rex", Year: 1979},
	}
	lib.provider.Searches[identifytest.SearchKey("Grace And Folly", 2005)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "13", Name: "Grace & Folly", Year: 2005}),
		{ID: "14", Name: "Grace and Folly", Year: 1940},
	}
	// Neither is named Arkons, and two of its year are no single work under another name (D307 §1).
	lib.provider.Searches[identifytest.SearchKey("Arkons", 1986)] = []identify.Candidate{
		{ID: "15", Name: "Arkon", Year: 1986}, {ID: "18", Name: "Arkon Nation", Year: 1986},
	}
	lib.provider.Searches[identifytest.SearchKey("Kestrel", 2014)] = []identify.Candidate{
		{ID: "16", Name: "Kestrel", Year: 2013}, {ID: "17", Name: "Kestrel", Year: 2015},
	}
	files := map[string]identify.Outcome{
		"Hollow Violet 1997 REMASTERED JAPANESE 1080p BluRay H264 AAC-OSPREY.mkv": identify.OutcomeMatched,
		"Wild.Rex.2.The.Dust.Road.1981.1080p.BluRay.H264.AAC-THISTLE.mp4":     identify.OutcomeMatched,
		"Grace.And.Folly.2005.1080p.BluRay.x264.anoXmous_.mp4":                 identify.OutcomeMatched,
		"Arkons (1986).mkv":  identify.OutcomeAmbiguous,
		"Kestrel (2014).mkv": identify.OutcomeAmbiguous,
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
	lib := newTestLibrary(t, "movie", "tmdb")
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
		if title := lib.titlesByPath()[path]; !lib.isGuessed(title.WorkID) {
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
	lib := newTestLibrary(t, "movie", "tmdb")
	blue := lib.provider.Add(identify.Metadata{ID: "10", Name: "Hollow Violet", Year: 1998})
	lib.provider.Searches[identifytest.SearchKey("Hollow Violet", 1997)] = []identify.Candidate{
		blue, {ID: "11", Name: "Hollow Violet", Year: 1998},
	}
	lib.provider.Searches[identifytest.SearchKey("Kestrel", 2014)] = []identify.Candidate{
		{ID: "16", Name: "Kestrel", Year: 2013}, {ID: "17", Name: "Kestrel", Year: 2015},
	}
	lib.scan(videoRecord("Hollow Violet (1997).mkv"), videoRecord("Kestrel (2014).mkv"))
	for _, rel := range []string{"Hollow Violet (1997).mkv", "Kestrel (2014).mkv"} {
		if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeAmbiguous {
			t.Fatalf("%s: %s; want ambiguous to begin with", rel, outcome)
		}
	}
	// As earlier rules left it: its kept results now hold its one answer, a year off.
	blueTitle := lib.titlesByPath()["Hollow Violet (1997).mkv"].WorkID
	err := lib.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		if err := queries.DeleteMatchCandidates(t.Context(), blueTitle); err != nil {
			return err //nolint:wrapcheck // the test reads it as is
		}
		year := int64(1998)
		return queries.InsertMatchCandidate(t.Context(), cachegen.InsertMatchCandidateParams{
			ItemID: blueTitle, Rank: 0, Provider: "tmdb", ProviderID: "10", Name: new("Hollow Violet"), Year: &year,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	lib.provider.Searches[identifytest.SearchKey("Hollow Violet", 1997)] = []identify.Candidate{blue}
	returned, err := lib.matcher.ReconsiderAmbiguous(t.Context())
	if err != nil || returned != 1 {
		t.Fatalf("ReconsiderAmbiguous = %d, %v; want Hollow Violet alone returned to its first match", returned, err)
	}
	if outcome := lib.matchFileTitle("Hollow Violet (1997).mkv"); outcome != identify.OutcomeMatched {
		t.Errorf("Hollow Violet: %s; want matched once returned", outcome)
	}
	kestrel := lib.titlesByPath()["Kestrel (2014).mkv"]
	if ptr.ValueOrZero(kestrel.IdentificationState) != identify.IdentificationAmbiguous {
		t.Errorf("Kestrel is %s; two a year off its own stay a person's choice",
			ptr.ValueOrZero(kestrel.IdentificationState))
	}
}

func TestMatch_AProviderIDInHandSkipsTheSearch(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Add(heat)
	lib.provider.Add(theo)
	lib.provider.TMDBIDsByOtherID["tt0345678"] = theo.ID
	lib.scan(videoRecord("Metropolis {tmdb-9019}.mkv"), videoRecord("The Cameraman [imdbid-tt0345678].mkv"))
	lib.matchFileTitle("Metropolis {tmdb-9019}.mkv")
	lib.matchFileTitle("The Cameraman [imdbid-tt0345678].mkv")
	want := []string{"fetch 949", "find imdb tt0345678", "fetch 101"}
	if !slices.Equal(lib.provider.Calls, want) {
		t.Errorf("calls %v, want %v: no search", lib.provider.Calls, want)
	}
	if file := lib.titlesByPath()["The Cameraman [imdbid-tt0345678].mkv"]; titleName(file) != theo.Name {
		t.Errorf("the IMDb-tagged file is %q", titleName(file))
	}
}

func TestMatch_NothingFoundIsRecordedAndNotSearchedAgain(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo_then_tmdb")
	const rel = "home video (2011).mkv"
	lib.scan(videoRecord(rel))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeNoMatch {
		t.Fatalf("outcome %s, want no match", outcome)
	}
	// The first match searches the name, and its other spellings (spellingVariants).
	searched := len(lib.provider.Calls)
	report := lib.scan(videoRecord(rel))
	file := lib.titlesByPath()[rel]
	why := identify.NoMatchReason("nfo_then_tmdb")
	if file.Filing != identify.FilingUnidentified || file.FilingReason == nil || *file.FilingReason != why {
		t.Errorf("file %+v, want unidentified, saying why", file)
	}
	if len(report.AwaitingMatch) != 0 {
		t.Errorf("the pass queued it again: %v", report.AwaitingMatch)
	}
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeNothingToDo ||
		len(lib.provider.Calls) != searched {
		t.Errorf("matching again: %s after calls %v; want nothing done, no search past the first match's %d",
			outcome, lib.provider.Calls, searched)
	}
}

func TestMatch_ASecondTitleForTheSameWorkJoinsTheFirst(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(heat)}
	lib.scan(videoRecord("Metropolis {tmdb-9019}.mkv"), videoRecord("other/Metropolis (1995).mkv"))
	lib.matchFileTitle("Metropolis {tmdb-9019}.mkv")
	lib.matchFileTitle("other/Metropolis (1995).mkv")
	files := lib.titlesByPath()
	first, second := files["Metropolis {tmdb-9019}.mkv"], files["other/Metropolis (1995).mkv"]
	if first.WorkID != second.WorkID {
		t.Errorf("titles %d and %d; one work found twice should be one title with two versions",
			first.WorkID, second.WorkID)
	}
	lib.scan(videoRecord("Metropolis {tmdb-9019}.mkv"), videoRecord("other/Metropolis (1995).mkv"))
	if files := lib.titlesByPath(); files["other/Metropolis (1995).mkv"].WorkID != first.WorkID {
		t.Error("another pass split the joined title")
	}
}

func TestMatch_AnOverrideSurvivesTheMatchAndReDerivation(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(heat)}
	const rel = "Metropolis (1995).mkv"
	lib.scan(videoRecord(rel))
	titleID := lib.titlesByPath()[rel].WorkID
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
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.scan(videoRecord("Metropolis (1995).mkv"))
	lib.provider.SetFailure(&outbound.RateLimitedError{Provider: "TMDB", RetryAfter: 3})
	_, err := lib.matcher.Match(t.Context(), lib.titlesByPath()["Metropolis (1995).mkv"].WorkID)
	if limited, isLimited := errors.AsType[*outbound.RateLimitedError](err); !isLimited || limited.RetryAfter != 3 {
		t.Errorf("Match = %v, want the provider's RateLimitedError", err)
	}
	file := lib.titlesByPath()["Metropolis (1995).mkv"]
	if ptr.ValueOrZero(file.IdentificationState) != identify.IdentificationUnidentified || file.FollowedSource != nil {
		t.Errorf("a refused call changed the title: %+v", file)
	}
}

func TestHandle_AsksTheProviderWithTheJobsImportanceAndPassesOnItsParking(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.scan(videoRecord("Metropolis (1995).mkv"), videoRecord("Nosferatu (1998).mkv"), videoRecord("Arkon (1979).mkv"))
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
			Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: titles[path].WorkID}, Priority: check.priority,
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
	arkon := jobs.Job{Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: titles["Arkon (1979).mkv"].WorkID}}
	if parked, isParked := jobqueue.ParkingOf(lib.matcher.Handle(t.Context(), arkon)); !isParked || parked != parking {
		t.Errorf("the provider parked the fetch as %+v (%v); want its parking passed to the queue", parked, isParked)
	}
}

func TestPass_ATMDBOnlyLibraryIgnoresNFOFiles(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	const body = "<movie><title>Not What The Path Says</title><year>1995</year><tmdbid>9019</tmdbid>" +
		"<lockdata>true</lockdata></movie>"
	report := lib.scan(videoRecord("Metropolis (1995).mkv"), nfoRecord("Metropolis (1995).nfo", body))
	file := lib.titlesByPath()["Metropolis (1995).mkv"]
	if titleName(file) != "Metropolis" || file.FollowedSource != nil || file.Filing != identify.FilingUnidentified ||
		len(report.AwaitingMatch) != 1 {
		t.Errorf("file %+v named %q, awaiting %v; want the path's name, awaiting TMDB (D183: ignore NFO files)",
			file, titleName(file), report.AwaitingMatch)
	}
	ids, err := lib.database.Read().ListItemProviderIDs(t.Context(), file.WorkID)
	if err != nil || len(ids) != 0 {
		t.Errorf("ids %+v (%v); the NFO's id should not have been read", ids, err)
	}
	overrides, err := lib.database.Read().ListOverrides(t.Context(), file.WorkID)
	if err != nil || len(overrides) != 0 {
		t.Errorf("overrides %+v (%v); the NFO's locked fields should not have been read", overrides, err)
	}
}

// Principles §22, D211: what a provider answers is clean where it enters, whoever wrote it.
func TestMatch_AProvidersTextIsCleanWhereItEnters(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
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
	lib := newTestLibrary(t, "movie", "nfo")
	lib.scan(videoRecord("Metropolis (1995).mkv"))
	titleID := lib.titlesByPath()["Metropolis (1995).mkv"].WorkID
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

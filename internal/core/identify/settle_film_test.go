// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"bytes"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

// D365: a film's candidates are ranked by how their runtimes fit its files, then by year, then by
// votes; a series keeps D301 §1.

// videoLasting is videoRecord of a file whose probe says it lasts lengthMs.
func videoLasting(path string, lengthMs int64) nodeproto.ScanRecord {
	record := videoRecord(path)
	record.Observed.Probe.Format.DurationMs = lengthMs
	return record
}

const minuteMs = 60_000

// staging's file, 92.1 minutes, and TMDB's two films of its name: an obscure one of the file's year,
// 88 minutes, and the film it is, 92 minutes, dated a year later at TMDB (the owner's staging,
// 2026-10-27).
const (
	crowdRelease   = "Crowd.2022.1080p.WEBRip.x264.AAC-KESTREL.mp4"
	crowdReleaseMs = 5_527_264
	obscureCrowd   = "1120937"
	theCrowd       = "1020910"
)

// crowdCandidates gives the fake TMDB staging's two candidates for the file's name and year.
func crowdCandidates(lib *testLibrary) {
	obscure := lib.provider.Add(identify.Metadata{
		ID: obscureCrowd, Name: "Crowd", OriginalName: "Crowd", Year: 2022, RuntimeMs: 88 * minuteMs,
	})
	obscure.Votes = 30
	film := lib.provider.Add(identify.Metadata{
		ID: theCrowd, Name: "Crowd", OriginalName: "Crowd", Year: 2023, RuntimeMs: 92 * minuteMs,
	})
	film.Votes = 20
	lib.provider.Searches[identifytest.SearchKey("Crowd", 2022)] = []identify.Candidate{obscure, film}
}

// D365 §1, §2, §4: of the file's name, its year's film whose runtime the file's length contradicts is
// passed over for the film a year later that fits it, fewer votes and all, which is a guess; the
// runtimes are read from each candidate's details, kept with the candidates, and the one taken is
// stored from the details read for its runtime, not fetched again.
func TestMatch_AFilmsCandidateWhoseRuntimeFitsTheFileIsTaken(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	crowdCandidates(lib)
	lib.scan(videoLasting(crowdRelease, crowdReleaseMs))
	if outcome := lib.matchFileTitle(crowdRelease); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s, want matched", outcome)
	}
	if id, _ := lib.followed(crowdRelease); id != theCrowd {
		t.Errorf("the file follows %s; want %s, whose 92 minutes fit its 92.1", id, theCrowd)
	}
	title := lib.titlesByPath()[crowdRelease]
	if !lib.isGuessed(title.TitleID) {
		t.Error("the match is not marked a guess; two of the file's name were found")
	}
	fetches := map[string]int{}
	for _, call := range lib.provider.Calls {
		if id, isFetch := strings.CutPrefix(call, "fetch "); isFetch {
			fetches[id]++
		}
	}
	if fetches[obscureCrowd] != 1 || fetches[theCrowd] != 1 {
		t.Errorf("fetches %v; want each candidate's details once, the one taken stored from them", fetches)
	}
	kept, err := lib.cache.Read().ListMatchCandidates(t.Context(), title.TitleID)
	if err != nil {
		t.Fatal(err)
	}
	runtimes := map[string]int64{}
	for _, candidate := range kept {
		runtimes[candidate.ProviderID] = ptr.ValueOrZero(candidate.RuntimeMs)
	}
	if runtimes[obscureCrowd] != 88*minuteMs || runtimes[theCrowd] != 92*minuteMs {
		t.Errorf("kept runtimes %v; want each candidate's, kept with it", runtimes)
	}
}

// D365 §4: a runtime kept with a candidate is not fetched again when the film is matched again, by
// a rescan; only the work taken is fetched, for its record.
func TestRescan_AKeptCandidatesRuntimeIsNotFetchedAgain(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	crowdCandidates(lib)
	lib.scan(videoLasting(crowdRelease, crowdReleaseMs))
	lib.matchFileTitle(crowdRelease)
	before := len(lib.provider.Calls)
	rescan := lib.rescanning()
	rescan.library()
	rescan.run()
	if id, _ := lib.followed(crowdRelease); id != theCrowd {
		t.Errorf("after the rescan the file follows %s; want %s", id, theCrowd)
	}
	if calls := lib.provider.Calls[before:]; slices.Contains(calls, "fetch "+obscureCrowd) {
		t.Errorf("the rescan fetched the candidate not taken again: %v; its runtime is kept", calls)
	}
}

// D365 §1: a runtime fits within the larger of 3 minutes and 3% of it; a candidate that fits comes
// before the rest whatever their votes, and one whose runtime is unknown ranks with one the file
// contradicts (D412 §2), as does one the file is a longer cut of, with a year in the name.
func TestMatch_AFilmsCandidatesAreRankedFitsThenTheRest(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		name     string
		lengthMs int64
		first    identify.Metadata // the more voted
		second   identify.Metadata
		want     string
	}{
		{
			// 3% of 160 minutes is 4.8: a file 4.5 minutes longer fits it, as 3 minutes would not.
			name: "three percent of a long film", lengthMs: 164*minuteMs + 30_000,
			first:  identify.Metadata{ID: "1", Name: "Faust", Year: 2021, RuntimeMs: 155 * minuteMs},
			second: identify.Metadata{ID: "2", Name: "Faust", Year: 2021, RuntimeMs: 160 * minuteMs},
			want:   "2",
		},
		{
			// 3% of 80 minutes is 2.4: 3 minutes is the margin, and a file 2.9 minutes longer fits.
			name: "three minutes of a short film", lengthMs: 82*minuteMs + 54_000,
			first:  identify.Metadata{ID: "1", Name: "Faust", Year: 2021, RuntimeMs: 75 * minuteMs},
			second: identify.Metadata{ID: "2", Name: "Faust", Year: 2021, RuntimeMs: 80 * minuteMs},
			want:   "2",
		},
		{
			name: "unknown with contradicts", lengthMs: 100 * minuteMs,
			first:  identify.Metadata{ID: "1", Name: "Faust", Year: 2021, RuntimeMs: 130 * minuteMs},
			second: identify.Metadata{ID: "2", Name: "Faust", Year: 2021},
			want:   "1",
		},
		{
			name: "a longer cut with contradicts, with a year", lengthMs: 100 * minuteMs,
			first:  identify.Metadata{ID: "1", Name: "Faust", Year: 2021, RuntimeMs: 130 * minuteMs},
			second: identify.Metadata{ID: "2", Name: "Faust", Year: 2021, RuntimeMs: 90 * minuteMs},
			want:   "1",
		},
		{
			name: "fits before unknown", lengthMs: 100 * minuteMs,
			first:  identify.Metadata{ID: "1", Name: "Faust", Year: 2021},
			second: identify.Metadata{ID: "2", Name: "Faust", Year: 2021, RuntimeMs: 99 * minuteMs},
			want:   "2",
		},
	} {
		t.Run(each.name, func(t *testing.T) {
			t.Parallel()
			lib := newTestLibrary(t, "movie", "tmdb")
			first, second := lib.provider.Add(each.first), lib.provider.Add(each.second)
			first.Votes, second.Votes = 9000, 10
			lib.provider.Searches[identifytest.SearchKey("Faust", 2021)] = []identify.Candidate{first, second}
			const rel = "Faust (2021).mkv"
			lib.scan(videoLasting(rel, each.lengthMs))
			lib.matchFileTitle(rel)
			if id, _ := lib.followed(rel); id != each.want {
				t.Errorf("the file follows %s; want %s", id, each.want)
			}
		})
	}
}

// D365 §2: a year in the name admits a year either side and no more; within a class of fit, the
// named year's film comes before one a year off, however voted; and several a year off, none
// fitting, stay a person's choice (D301 §1), one fitting being taken.
func TestMatch_AFilmsYearAdmitsAYearEitherSideItsOwnFirst(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	ofTheYear := lib.provider.Add(identify.Metadata{ID: "1", Name: "The General", Year: 2002, RuntimeMs: 99 * minuteMs})
	yearOn := lib.provider.Add(identify.Metadata{ID: "2", Name: "The General", Year: 2003, RuntimeMs: 100 * minuteMs})
	yearOn.Votes = 9000
	twoOn := lib.provider.Add(identify.Metadata{ID: "3", Name: "The General", Year: 2004, RuntimeMs: 100 * minuteMs})
	lib.provider.Searches[identifytest.SearchKey("The General", 2002)] = []identify.Candidate{yearOn, ofTheYear}
	lib.provider.Searches[identifytest.SearchKey("The General", 2006)] = []identify.Candidate{twoOn}
	lib.provider.Searches[identifytest.SearchKey("Kestrel", 2014)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "16", Name: "Kestrel", Year: 2013, RuntimeMs: 140 * minuteMs}),
		lib.provider.Add(identify.Metadata{ID: "17", Name: "Kestrel", Year: 2015, RuntimeMs: 98 * minuteMs}),
	}
	lib.provider.Searches[identifytest.SearchKey("Mirage", 2014)] = []identify.Candidate{
		lib.provider.Add(identify.Metadata{ID: "26", Name: "Mirage", Year: 2013, RuntimeMs: 140 * minuteMs}),
		lib.provider.Add(identify.Metadata{ID: "27", Name: "Mirage", Year: 2015, RuntimeMs: 150 * minuteMs}),
	}
	lib.scan(videoLasting("The General (2002).mkv", 100*minuteMs), videoLasting("The General (2006).mkv", 100*minuteMs),
		videoLasting("Kestrel (2014).mkv", 98*minuteMs), videoLasting("Mirage (2014).mkv", 98*minuteMs))
	for rel, want := range map[string]identify.Outcome{
		"The General (2002).mkv": identify.OutcomeMatched, "The General (2006).mkv": identify.OutcomeAmbiguous,
		"Kestrel (2014).mkv": identify.OutcomeMatched, "Mirage (2014).mkv": identify.OutcomeAmbiguous,
	} {
		if outcome := lib.matchFileTitle(rel); outcome != want {
			t.Errorf("%s: outcome %s; want %s", rel, outcome, want)
		}
	}
	if id, _ := lib.followed("The General (2002).mkv"); id != "1" {
		t.Errorf("The General (2002) follows %s; want 1, of its year, both fitting", id)
	}
	if id, _ := lib.followed("Kestrel (2014).mkv"); id != "17" {
		t.Errorf("Kestrel (2014) follows %s; want 17, a year off and fitting", id)
	}
}

// D365 §3 with D301 §1 and D412 §3, §4: with no year in the name and votes near each other's, as
// the real films' are, the oldest is taken where the runtimes do not
// separate them (Disney's Alcazar over a later one), and the fit comes first where they do.
func TestMatch_AFilmWithNoYearTakesTheOldestOfThoseThatFitBest(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	disneys := lib.provider.Add(identify.Metadata{ID: "4812", Name: "Alcazar", Year: 1992, RuntimeMs: 90 * minuteMs})
	remake := lib.provider.Add(identify.Metadata{ID: "480817", Name: "Alcazar", Year: 2019, RuntimeMs: 128 * minuteMs})
	disneys.Votes, remake.Votes = 9000, 15000
	lib.provider.Searches[identifytest.SearchKey("Alcazar", 0)] = []identify.Candidate{remake, disneys}
	older := lib.provider.Add(identify.Metadata{ID: "30", Name: "Clarabel", Year: 1954, RuntimeMs: 113 * minuteMs})
	newer := lib.provider.Add(identify.Metadata{ID: "31", Name: "Clarabel", Year: 1995, RuntimeMs: 127 * minuteMs})
	lib.provider.Searches[identifytest.SearchKey("Clarabel", 0)] = []identify.Candidate{older, newer}
	const sabrina, unprobed = "Clarabel.mkv", "Alcazar.mkv"
	lib.scan(videoLasting(sabrina, 127*minuteMs), videoLasting(unprobed, 0))
	for _, each := range []struct{ rel, want, why string }{
		{sabrina, "31", "the newer, whose runtime fits the file's, the older's only as a longer cut"},
		{unprobed, "812", "the oldest, no length to fit to"},
	} {
		lib.matchFileTitle(each.rel)
		if id, _ := lib.followed(each.rel); id != each.want {
			t.Errorf("%s follows %s; want %s, %s", each.rel, id, each.want, each.why)
		}
	}
	both := newTestLibrary(t, "movie", "tmdb")
	disneys = both.provider.Add(identify.Metadata{ID: "4812", Name: "Alcazar", Year: 1992})
	remake = both.provider.Add(identify.Metadata{ID: "480817", Name: "Alcazar", Year: 2019})
	disneys.Votes, remake.Votes = 9000, 15000
	both.provider.Searches[identifytest.SearchKey("Alcazar", 0)] = []identify.Candidate{remake, disneys}
	both.scan(videoLasting(unprobed, 100*minuteMs))
	both.matchFileTitle(unprobed)
	if id, _ := both.followed(unprobed); id != "812" {
		t.Errorf("%s follows %s; want 812, the oldest, neither runtime known", unprobed, id)
	}
}

// D412 §2: a candidate whose runtime is unknown ranks with those the file contradicts: the owner's
// Wings.mp4, 148.8 minutes, a longer cut than Cameron's 137, went to an Wings of 2014 TMDB knows
// no runtime of and one vote (the owner, 2026-11-02; TMDB's answers as staging kept them). Cameron's
// is taken, the most voted by far, the oldest and a longer cut's fit (D412 §3, §4).
func TestMatch_AFilmWithNoYearIsNotTakenByAnUnknownRuntimeOverTheOldest(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	add := func(id string, year int, runtimeMs int64, votes int) identify.Candidate {
		candidate := lib.provider.Add(identify.Metadata{
			ID: id, Name: "Wings", OriginalName: "Wings", Year: year, RuntimeMs: runtimeMs,
		})
		candidate.Votes = votes
		return candidate
	}
	lib.provider.Searches[identifytest.SearchKey("Wings", 0)] = []identify.Candidate{
		add("679", 1986, 137*minuteMs, 11339),
		{ID: "3930", Name: "Monsters vs Wings", Year: 2009, Votes: 5185},
		add("504935", 1993, 78*minuteMs, 5),
		add("467396", 2017, 23*minuteMs, 3),
		add("486595", 2014, 0, 1),
	}
	const rel = "Wings.mp4"
	lib.scan(videoLasting(rel, 8_925_516))
	lib.matchFileTitle(rel)
	if id, _ := lib.followed(rel); id != "679" {
		t.Errorf("%s follows %s; want 679, the oldest, none fitting and an unknown runtime no better", rel, id)
	}
}

// D412 §3: with no year in the name, the well-known is the likely one: the most voted is taken, but
// only by ten times the votes or more; nearer, the oldest still is, and a handful of votes over none
// decides nothing.
func TestMatch_AFilmWithNoYearTakesTheMostVotedOnlyByAWideMargin(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	search := func(name string, older, newer int) {
		one := lib.provider.Add(identify.Metadata{ID: name + "-1950", Name: name, Year: 1950})
		other := lib.provider.Add(identify.Metadata{ID: name + "-2001", Name: name, Year: 2001})
		one.Votes, other.Votes = older, newer
		lib.provider.Searches[identifytest.SearchKey(name, 0)] = []identify.Candidate{one, other}
	}
	search("Marrow", 40, 6000)
	search("Thistle", 700, 6000)
	search("Fennel", 0, 9)
	for _, each := range []struct{ rel, want, why string }{
		{"Marrow.mkv", "Marrow-2001", "the newer, ten times as voted and more"},
		{"Thistle.mkv", "Thistle-1950", "the older, the newer not ten times as voted"},
		{"Fennel.mkv", "Fennel-1950", "the older, nine votes too few to count"},
	} {
		lib.scan(videoLasting(each.rel, 100*minuteMs))
		lib.matchFileTitle(each.rel)
		if id, _ := lib.followed(each.rel); id != each.want {
			t.Errorf("%s follows %s; want %s, %s", each.rel, id, each.want, each.why)
		}
	}
}

// D412 §4: with no year in the name, a file up to 30% longer than a candidate's runtime fits it as a
// longer cut, 50% where its name says it is one, over the oldest the file contradicts: the owner's
// Wings.mp4, 148.8 minutes, is Cameron's 137-minute Wings (here a stand-in with an older namesake
// near it in votes); but a close fit comes before a longer cut's (Clarabel,
// TestMatch_AFilmWithNoYearTakesTheOldestOfThoseThatFitBest).
func TestMatch_AFilmWithNoYearFitsALongerCut(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		rel      string
		lengthMs int64
		want     string
		why      string
	}{
		{"Harrow.mp4", 8_925_516, "1986", "148.8 minutes, a longer cut of its 137"},
		{"Harrow.Extended.mkv", 190 * minuteMs, "1986", "190 minutes, a longer cut of its 137 by its name"},
		{"Harrow.mkv", 190 * minuteMs, "1979", "the oldest, 190 minutes too long for a cut not named one"},
	} {
		// A library each, as two files of one name would be versions of one title.
		lib := newTestLibrary(t, "movie", "tmdb")
		older := lib.provider.Add(identify.Metadata{ID: "1979", Name: "Harrow", Year: 1979, RuntimeMs: 95 * minuteMs})
		camerons := lib.provider.Add(identify.Metadata{
			ID: "1986", Name: "Harrow", Year: 1986, RuntimeMs: 137 * minuteMs,
		})
		older.Votes, camerons.Votes = 5000, 9000
		lib.provider.Searches[identifytest.SearchKey("Harrow", 0)] = []identify.Candidate{camerons, older}
		lib.scan(videoLasting(each.rel, each.lengthMs))
		lib.matchFileTitle(each.rel)
		if id, _ := lib.followed(each.rel); id != each.want {
			t.Errorf("%s follows %s; want %s, %s", each.rel, id, each.want, each.why)
		}
	}
}

// D412 §1: with no year in the name, a candidate whose original name is the file's comes before one
// named so only in translation, where nothing else separates them; but it never knocks one out:
// the owner's Blind & Ugly is named in English, the film German, Blind & Hässlich.
func TestMatch_AFilmWithNoYearPrefersItsOriginalNameButKeepsATranslatedOne(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	translated := lib.provider.Add(identify.Metadata{
		ID: "60", Name: "Harbour Lights", OriginalName: "Hafenlichter", Year: 1960,
	})
	original := lib.provider.Add(identify.Metadata{
		ID: "90", Name: "Harbour Lights", OriginalName: "Harbour Lights", Year: 1990,
	})
	translated.Votes, original.Votes = 300, 200
	lib.provider.Searches[identifytest.SearchKey("Harbour Lights", 0)] = []identify.Candidate{translated, original}
	german := lib.provider.Add(identify.Metadata{
		ID: "480500", Name: "Blind & Ugly", OriginalName: "Blind & Hässlich", Year: 2017,
	})
	short := lib.provider.Add(identify.Metadata{
		ID: "900001", Name: "Blind & Ugly", OriginalName: "Blind & Ugly", Year: 2020,
	})
	german.Votes, short.Votes = 400, 2
	lib.provider.Searches[identifytest.SearchKey("Blind & Ugly", 0)] = []identify.Candidate{short, german}
	for _, each := range []struct{ rel, want, why string }{
		{"Harbour Lights.mkv", "90", "the one so named originally, the older named so only in translation"},
		{"Pale.and.Kind.mkv", "379138", "the German film, named in English, far the more voted"},
	} {
		lib.scan(videoLasting(each.rel, 100*minuteMs))
		lib.matchFileTitle(each.rel)
		if id, _ := lib.followed(each.rel); id != each.want {
			t.Errorf("%s follows %s; want %s, %s", each.rel, id, each.want, each.why)
		}
	}
}

// D365 §5: the log says, for a film whose name admitted several candidates, each one's year,
// runtime, votes and fit, and the one taken.
func TestMatch_AFilmsChoiceAmongItsCandidatesIsLogged(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	crowdCandidates(lib)
	var logged bytes.Buffer
	lib.matcher = identify.NewMatcher(lib.database, lib.cache, lib.pass, lib.clock,
		slog.New(slog.NewTextHandler(&logged, nil)))
	lib.scan(videoLasting(crowdRelease, crowdReleaseMs))
	lib.matchFileTitle(crowdRelease)
	itemID := strconv.FormatInt(lib.titlesByPath()[crowdRelease].TitleID, 10)
	for _, want := range []string{
		`msg="a film's candidate" itemId=` + itemID +
			` candidate=1020910 year=2023 runtimeMs=5520000 votes=20 fit=fits`,
		`msg="a film's candidate" itemId=` + itemID +
			` candidate=1120937 year=2022 runtimeMs=5280000 votes=30 fit=edition`,
		`msg="a film's candidates settled" itemId=` + itemID + ` year=2022 fileLengthsMs=[5527264] taken=1020910` +
			` isGuess=true`,
	} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("the log lacks %s:\n%s", want, logged.String())
		}
	}
}

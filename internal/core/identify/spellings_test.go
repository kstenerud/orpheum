// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A name's other spellings are the other side of the Atlantic's, its "and" as "&" or "&" as "and",
// the name with or without its leading article, and its numbers as Roman or Arabic numerals, each the
// whole name changed one way.
func TestSpellingVariants_AreTheOtherWaysANameIsWritten(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		name string
		want []string
	}{
		{"Harbor Lights", []string{"Harbour Lights", "The Harbor Lights"}},
		{"The Colour of Magic", []string{"The Color of Magic", "Colour of Magic"}},
		{"Rocky II", []string{"The Rocky II", "Rocky 2"}},
		{"Ocean's 11", []string{"The Ocean's 11", "Ocean's XI"}},
		{"THEATER OF BLOOD", []string{"THEATRE OF BLOOD", "The THEATER OF BLOOD"}},
		{"Organized Crime", []string{"Organised Crime", "The Organized Crime"}},
		// A short word keeps its ending, an -ise word is -ise both sides, and a lone I is a word.
		{"I Prize the Size", []string{"The I Prize the Size"}},
		{"Paradise", []string{"The Paradise"}},
		// "and" and "&" are each other's spelling, a word "and" alone: "Band" keeps its "and".
		{"Pale and Kind", []string{"Blind & Ugly", "The Pale and Kind"}},
		{"Show & Tell", []string{"Show and Tell", "The Show & Tell"}},
		{"Band of Brothers", []string{"The Band of Brothers"}},
		// A name every way changes is searched every way.
		{"The Colour and the Shape II", []string{
			"The Color and the Shape II", "The Colour & the Shape II", "Colour and the Shape II",
			"The Colour and the Shape 2",
		}},
	} {
		if got := identify.SpellingVariants(each.name); !slices.Equal(got, each.want) {
			t.Errorf("SpellingVariants(%q) = %q; want %q", each.name, got, each.want)
		}
	}
}

// A search that finds no work of the name searches its other spellings, and settles on what one of
// them finds: TMDB has Edric Pellow's series as "Harbour Lights", and a search for "Harbor Lights"
// finds only works with the word in their names (the owner's staging, 2026-10-21).
func TestMatch_ASeriesSpelledTheOtherWayIsFoundByItsOtherSpelling(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "tmdb")
	civilisation := identify.Metadata{ID: "4808", Name: "Harbour Lights", Year: 1969}
	lib.provider.Searches[identifytest.SearchKey("Harbor Lights", 0)] = []identify.Candidate{
		{ID: "70027", Name: "The Greeks: Crucible of Harbor Lights", Year: 2000},
		{ID: "202999", Name: "Lost Harbor Lights", Year: 2019},
	}
	lib.provider.Searches[identifytest.SearchKey("Harbour Lights", 0)] = []identify.Candidate{
		lib.provider.Add(civilisation),
	}
	lib.scanAll("Harbor Lights/Season 01/Harbor Lights - S01E01.mkv")
	series := lib.onlySeries()
	outcome, err := lib.matcher.Match(t.Context(), series.ID)
	if err != nil || outcome != identify.OutcomeMatched {
		t.Fatalf("Match = %s, %v; want matched by its other spelling", outcome, err)
	}
	if matched := lib.onlySeries(); matched.FollowedSourceID == nil || *matched.FollowedSourceID != "9285" {
		t.Errorf("the series follows %v; want TMDB's Harbour Lights, 9285", matched.FollowedSourceID)
	}
}

// A release that writes its work's "&" as "and" finds it by the "&": TMDB's search for "Blind and
// Ugly" finds nothing, and for "Blind & Ugly" the German film of 2017 the release is (the owner,
// 2026-11-02, IMDb tt0567890).
func TestMatch_AFilmWhoseNameSaysAndForItsAmpersandIsFoundByIt(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	film := lib.provider.Add(identify.Metadata{
		ID: "480500", Name: "Blind & Ugly", OriginalName: "Blind & Hässlich", Year: 2017,
	})
	lib.provider.Searches[identifytest.SearchKey("Blind & Ugly", 2017)] = []identify.Candidate{film}
	const rel = "Pale.and.Kind.2017.GERMAN.1080p.WEBRip.x265-OSPREY/Pale.and.Kind.2017.GERMAN.1080p.WEBRip.x265-OSPREY.mp4"
	lib.scan(videoRecord(rel))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s; want matched by the name's ampersand", outcome)
	}
	if id, _ := lib.followed(rel); id != "379138" {
		t.Errorf("the file follows %q; want TMDB's Blind & Ugly, 379138", id)
	}
}

// A file's misspelling of its work's name, a letter off in a long enough name and its year, is
// guessed, marked as a guess for a person to confirm (D301); a short name a letter off, a plural,
// another year, or two so named are not (the owner, 2026-10-21).
func TestMatch_ANameALetterOffIsGuessedOnlyWhereItCannotBeAnotherWork(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	potter := identify.Metadata{ID: "4803", Name: "The Thief of Bagdad", Year: 2001}
	const misspelt = "The Theif of Bagdad"
	lib.provider.Searches[identifytest.SearchKey(misspelt, 2001)] = []identify.Candidate{
		lib.provider.Add(potter), {ID: "673", Name: "The Thief of Bagdad: Behind the Magic", Year: 2001},
	}
	lib.provider.Searches[identifytest.SearchKey("Hest", 1995)] = []identify.Candidate{
		{ID: "9019", Name: "Metropolis", Year: 1995}, {ID: "950", Name: "Hesta", Year: 1995},
	}
	lib.provider.Searches[identifytest.SearchKey("Brindlewood", 2000)] = []identify.Candidate{
		{ID: "98", Name: "Arenaman", Year: 2000},
		{ID: "99", Name: "Arenaman Games", Year: 2010},
		{ID: "97", Name: "Arenaman Days", Year: 2000},
	}
	lib.provider.Searches[identifytest.SearchKey("Nanuk of the North", 2015)] = []identify.Candidate{
		{ID: "480400", Name: "Nanook of the North", Year: 2014},
		{ID: "1", Name: "Nanook of the North Revisited", Year: 2016},
		{ID: "2", Name: "Nanook of the North Days", Year: 2017},
	}
	files := map[string]identify.Outcome{
		"The Theif of Bagdad (2001).mkv": identify.OutcomeMatched,
		"Hest (1995).mkv":        identify.OutcomeAmbiguous,
		"Brindlewood (2000).mkv":  identify.OutcomeAmbiguous,
		"Nanuk of the North (2015).mkv": identify.OutcomeAmbiguous,
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
	if potterID := lib.titlesByPath()[misspelt+" (2001).mkv"].TitleID; !lib.isGuessed(potterID) {
		t.Error("the misspelt The Thief of Bagdad is matched but not marked a guess")
	}
}

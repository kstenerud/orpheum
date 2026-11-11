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
		{"Colonization", []string{"Colonisation", "The Colonization"}},
		{"The Colour of Magic", []string{"The Color of Magic", "Colour of Magic"}},
		{"Rocky II", []string{"The Rocky II", "Rocky 2"}},
		{"Ocean's 11", []string{"The Ocean's 11", "Ocean's XI"}},
		{"THEATER OF BLOOD", []string{"THEATRE OF BLOOD", "The THEATER OF BLOOD"}},
		{"Organized Crime", []string{"Organised Crime", "The Organized Crime"}},
		// A short word keeps its ending, an -ise word is -ise both sides, and a lone I is a word.
		{"I Prize the Size", []string{"The I Prize the Size"}},
		{"Paradise", []string{"The Paradise"}},
		// "and" and "&" are each other's spelling, a word "and" alone: "Band" keeps its "and".
		{"Pale and Kind", []string{"Pale & Kind", "The Pale and Kind"}},
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
// them finds: TMDB spells a series the British way, and a search of the American spelling finds
// only works with the words in their names (the owner's staging, 2026-10-21, the owner's series).
func TestMatch_ASeriesSpelledTheOtherWayIsFoundByItsOtherSpelling(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "tmdb")
	harbour := identify.Metadata{ID: "4808", Name: "Harbour Lights", Year: 1969}
	lib.provider.Searches[identifytest.SearchKey("Harbor Lights", 0)] = []identify.Candidate{
		{ID: "70027", Name: "The Ferries: Crucible of Harbor Lights", Year: 2000},
		{ID: "202999", Name: "Lost Harbor Lights", Year: 2019},
	}
	lib.provider.Searches[identifytest.SearchKey("Harbour Lights", 0)] = []identify.Candidate{
		lib.provider.Add(harbour),
	}
	lib.scanAll("Harbor Lights/Season 01/Harbor Lights - S01E01.mkv")
	series := lib.onlySeries()
	outcome, err := lib.matcher.Match(t.Context(), series.ID)
	if err != nil || outcome != identify.OutcomeMatched {
		t.Fatalf("Match = %s, %v; want matched by its other spelling", outcome, err)
	}
	if matched := lib.onlySeries(); matched.FollowedSourceID == nil || *matched.FollowedSourceID != "4808" {
		t.Errorf("the series follows %v; want TMDB's Harbour Lights, 4808", matched.FollowedSourceID)
	}
}

// A release that writes its work's "&" as "and" finds it by the "&": TMDB's search for "Pale and
// Kind" finds nothing, and for "Pale & Kind" the German film of 2017 the release is (the owner's
// film, 2026-11-02).
func TestMatch_AFilmWhoseNameSaysAndForItsAmpersandIsFoundByIt(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	film := lib.provider.Add(identify.Metadata{
		ID: "480500", Name: "Pale & Kind", OriginalName: "Blass & Freundlich", Year: 2017,
	})
	lib.provider.Searches[identifytest.SearchKey("Pale & Kind", 2017)] = []identify.Candidate{film}
	const release = "Pale.and.Kind.2017.GERMAN.1080p.WEBRip.x265-OSPREY"
	const rel = release + "/" + release + ".mp4"
	lib.scan(videoRecord(rel))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s; want matched by the name's ampersand", outcome)
	}
	if id, _ := lib.followed(rel); id != "480500" {
		t.Errorf("the file follows %q; want TMDB's Pale & Kind, 480500", id)
	}
}

// A file's misspelling of its work's name, a letter off in a long enough name and its year, is
// guessed, marked as a guess for a person to confirm (D301); a short name a letter off, a plural,
// another year, or two so named are not (the owner, 2026-10-21).
func TestMatch_ANameALetterOffIsGuessedOnlyWhereItCannotBeAnotherWork(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	metropolis := identify.Metadata{ID: "4803", Name: "Metropolis", Year: 1927}
	const misspelt = "Metropols"
	lib.provider.Searches[identifytest.SearchKey(misspelt, 1927)] = []identify.Candidate{
		lib.provider.Add(metropolis), {ID: "673", Name: "Making Metropolis", Year: 1927},
	}
	lib.provider.Searches[identifytest.SearchKey("Hest", 1995)] = []identify.Candidate{
		{ID: "948", Name: "Hust", Year: 1995}, {ID: "950", Name: "Hesta", Year: 1995},
	}
	lib.provider.Searches[identifytest.SearchKey("Lanterns", 2000)] = []identify.Candidate{
		{ID: "98", Name: "Lantern", Year: 2000},
		{ID: "99", Name: "Lantern Games", Year: 2010},
		{ID: "97", Name: "Lantern Days", Year: 2000},
	}
	lib.provider.Searches[identifytest.SearchKey("Nanuk of the North", 1923)] = []identify.Candidate{
		{ID: "480400", Name: "Nanook of the North", Year: 1922},
		{ID: "1", Name: "Nanook of the North Revisited", Year: 1924},
		{ID: "2", Name: "Nanook of the North Days", Year: 1925},
	}
	files := map[string]identify.Outcome{
		"Metropols (1927).mkv":          identify.OutcomeMatched,
		"Hest (1995).mkv":               identify.OutcomeAmbiguous,
		"Lanterns (2000).mkv":           identify.OutcomeAmbiguous,
		"Nanuk of the North (1923).mkv": identify.OutcomeAmbiguous,
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
	if metropolisID := lib.titlesByPath()[misspelt+" (1927).mkv"].WorkID; !lib.isGuessed(metropolisID) {
		t.Error("the misspelt Metropolis is matched but not marked a guess")
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
)

// A name's other spellings are the other side of the Atlantic's, the name with or without its
// leading article, and its numbers as Roman or Arabic numerals, each the whole name changed one way.
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
	lib := newTestLibrary(t, "shows", "tmdb")
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

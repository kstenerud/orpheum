// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
)

// harborLightsLibrary is a series library holding Harbor Lights, which a search of its name finds
// nothing of, and TMDB's Harbour Lights, by its id and by its IMDb id.
func harborLightsLibrary(t *testing.T) (*testLibrary, int64) {
	t.Helper()
	lib := newTestLibrary(t, "shows", "tmdb")
	lib.provider.Add(identify.Metadata{ID: "4808", Name: "Harbour Lights", Year: 1969})
	lib.provider.TMDBIDsByOtherID["tt0456789"] = "9285"
	lib.scanAll("Harbor Lights/Season 01/Harbor Lights - S01E01.mkv")
	series := lib.onlySeries()
	if outcome, err := lib.matcher.Match(t.Context(), series.ID); err != nil || outcome != identify.OutcomeNoMatch {
		t.Fatalf("Match = %s, %v; want no match", outcome, err)
	}
	return lib, series.ID
}

// A person searches for a name of their own, and chooses among what it found (the owner's staging,
// 2026-10-21).
func TestSearchAs_SearchesTheNameAPersonTypedAndKeepsWhatItFound(t *testing.T) {
	t.Parallel()
	lib, seriesID := harborLightsLibrary(t)
	lib.provider.Searches[identifytest.SearchKey("Edric Pellow Harbour Lights", 1969)] = []identify.Candidate{
		{ID: "4808", Name: "Harbour Lights", Year: 1969},
	}
	found, provider, err := lib.chooser.SearchAs(t.Context(), seriesID, " Edric Pellow Harbour Lights ", 1969)
	if err != nil || provider != "tmdb" || len(found) != 1 || found[0].ID != "4808" {
		t.Fatalf("SearchAs = %+v, %q, %v; want TMDB's Harbour Lights", found, provider, err)
	}
	kept, _, err := lib.chooser.Candidates(t.Context(), seriesID)
	if err != nil || len(kept) != 1 || kept[0].ID != "4808" {
		t.Errorf("the candidates kept are %+v, %v; want what the person's search found", kept, err)
	}
}

// A person pastes the work's address or id at TMDB or IMDb, and the item is matched to it, as a
// choice; an address of the other kind, or no address at all, is refused (the owner's staging,
// 2026-10-21).
func TestChooseByLink_MatchesTheWorkAPastedAddressOrIDNames(t *testing.T) {
	t.Parallel()
	for _, link := range []string{
		"https://www.themoviedb.org/tv/4808-civilisation", "9285", "https://www.imdb.com/title/tt0456789/",
		" tt0456789 ",
	} {
		lib, seriesID := harborLightsLibrary(t)
		if err := lib.chooser.ChooseByLink(t.Context(), seriesID, link); err != nil {
			t.Errorf("ChooseByLink(%q): %v", link, err)
			continue
		}
		series := lib.onlySeries()
		if series.FollowedSourceID == nil || *series.FollowedSourceID != "9285" ||
			series.IdentificationState == nil || *series.IdentificationState != identify.IdentificationConfirmed {
			t.Errorf("after %q the series is %+v; want TMDB's Harbour Lights, confirmed", link, series)
		}
	}
	lib, seriesID := harborLightsLibrary(t)
	for _, link := range []string{"https://www.themoviedb.org/movie/4808-civilisation", "Harbour Lights", ""} {
		if err := lib.chooser.ChooseByLink(t.Context(), seriesID, link); !errors.Is(err, identify.ErrBadLink) {
			t.Errorf("ChooseByLink(%q) = %v; want ErrBadLink", link, err)
		}
	}
	if err := lib.chooser.ChooseByLink(t.Context(), seriesID, "tt9999999"); !errors.Is(err, identify.ErrNoMatch) {
		t.Errorf("an IMDb id TMDB has no work for: %v; want ErrNoMatch", err)
	}
}

// Searching again for a series' candidates, as its page asks when none are kept, tries the name's
// other spellings as the first search does: Harbor Lights finds TMDB's Harbour Lights.
func TestCandidates_SearchingAgainTriesTheNamesOtherSpellings(t *testing.T) {
	t.Parallel()
	lib, seriesID := harborLightsLibrary(t)
	lib.provider.Searches[identifytest.SearchKey("Harbour Lights", 0)] = []identify.Candidate{
		{ID: "4808", Name: "Harbour Lights", Year: 1969},
	}
	found, provider, err := lib.chooser.Candidates(t.Context(), seriesID)
	if err != nil || provider != "tmdb" || len(found) != 1 || found[0].ID != "4808" {
		t.Errorf("Candidates = %+v, %q, %v; want TMDB's Harbour Lights, by its British spelling", found, provider, err)
	}
}

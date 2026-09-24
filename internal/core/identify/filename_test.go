// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"maps"
	"regexp"
	"strings"
	"testing"
)

func TestParsePath_ACorpusOfNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want Parsed
	}{
		// Films, as people name them.
		{"The Gold Rush (1999)/The Gold Rush (1999).mkv", Parsed{Kind: "movie", Name: "The Gold Rush", Year: 1999}},
		{"The.Gold.Rush.1999.1080p.BluRay.x264-GROUP.mkv", Parsed{Kind: "movie", Name: "The Gold Rush", Year: 1999}},
		{"Metropolis (1995)/movie.mkv", Parsed{Kind: "movie", Name: "Metropolis", Year: 1995}},
		{"Metropolis (1995)/Metropolis (1995) - 720p.mkv", Parsed{Kind: "movie", Name: "Metropolis", Year: 1995}},
		{"1917 (2019)/1917 (2019).mkv", Parsed{Kind: "movie", Name: "1917", Year: 2019}},
		{"7th Heaven (1968).mkv", Parsed{Kind: "movie", Name: "7th Heaven", Year: 1968}},
		{"Glass Runner 2061 (2017).mkv", Parsed{Kind: "movie", Name: "Glass Runner 2061", Year: 2017}},
		{"untitled rip final v2.mkv", Parsed{Kind: "movie", Name: "untitled rip final v2"}},
		// Not Latin: D49's cases, which ASCII character classes fail silently.
		{"Fièvre (2001)/Fièvre (2001).mkv", Parsed{Kind: "movie", Name: "Fièvre", Year: 2001}},
		{"狂った一頁 (2015).mkv", Parsed{Kind: "movie", Name: "狂った一頁", Year: 2015}},
		{"아리랑 (2019).mkv", Parsed{Kind: "movie", Name: "아리랑", Year: 2019}},
		{"Броненосец Потёмкин.1999.mkv", Parsed{Kind: "movie", Name: "Броненосец Потёмкин", Year: 1999}},
		// Provider ids in the name skip the search (D172).
		{"The Gold Rush (1999) {tmdb-4801}/The Gold Rush (1999).mkv", Parsed{
			Kind: "movie", Name: "The Gold Rush", Year: 1999, ProviderIDs: map[string]string{"tmdb": "4801"},
		}},
		{"Metropolis [imdbid-tt0234567].mkv", Parsed{
			Kind: "movie", Name: "Metropolis", ProviderIDs: map[string]string{"imdb": "tt0234567"},
		}},
		// Extras are typed by a suffix; containment makes them extras (D162).
		{"Metropolis (1995)/Metropolis (1995)-trailer.mkv", Parsed{
			Kind: "movie", Name: "Metropolis", Year: 1995, ExtraSuffix: "trailer",
		}},
		// An extra's own name; which film it belongs to is containment, decided above the parser.
		{"Metropolis (1995)/trailers/Metropolis teaser.mkv", Parsed{Kind: "movie", Name: "Metropolis teaser"}},
		// Episodes.
		{"Test Show/Season 01/Test Show - S01E01.mkv", Parsed{
			Kind: "episode", Name: "Test Show", Season: 1, Episode: 1,
		}},
		{"Show.Name.S02E10.720p.HDTV.mkv", Parsed{Kind: "episode", Name: "Show Name", Season: 2, Episode: 10}},
		{"Show/Season 1/S01E03-E04.mkv", Parsed{Kind: "episode", Name: "Show", Season: 1, Episode: 3, EpisodeEnd: 4}},
		{"Show (2005)/s03e07.mkv", Parsed{Kind: "episode", Name: "Show", Year: 2005, Season: 3, Episode: 7}},
		{"Doctor Wren (2005)/3x07 - 42.mkv", Parsed{
			Kind: "episode", Name: "Doctor Wren", Year: 2005, Season: 3, Episode: 7,
		}},
		{"狂った一頁/Season 01/狂った一頁 S01E02.mkv", Parsed{Kind: "episode", Name: "狂った一頁", Season: 1, Episode: 2}},
		{"Show/Staffel 2/Folge S02E05.mkv", Parsed{Kind: "episode", Name: "Show", Season: 2, Episode: 5}},
		{"Show/Season 1/Show - S01E02 - The Pilot.mkv", Parsed{Kind: "episode", Name: "Show", Season: 1, Episode: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			got := ParsePath(tc.path)
			if tc.want.ProviderIDs == nil {
				tc.want.ProviderIDs = map[string]string{}
			}
			if got.Kind != tc.want.Kind || got.Name != tc.want.Name || got.Year != tc.want.Year ||
				got.Season != tc.want.Season || got.Episode != tc.want.Episode ||
				got.EpisodeEnd != tc.want.EpisodeEnd || got.ExtraSuffix != tc.want.ExtraSuffix ||
				!maps.Equal(got.ProviderIDs, tc.want.ProviderIDs) {
				t.Errorf("ParsePath(%q)\n got %+v\nwant %+v", tc.path, got, tc.want)
			}
		})
	}
}

// D49: \w, \d, \s, \b and [[:alpha:]] are ASCII-only in Go, and an expression using one fails
// silently on a non-Latin name.
func TestParsePath_NoExpressionUsesAnASCIIOnlyClass(t *testing.T) {
	t.Parallel()
	patterns := []*regexp.Regexp{providerIDPattern, episodePattern, crossPattern, seasonDirPattern, yearPattern}
	for _, pattern := range patterns {
		for _, banned := range []string{`\w`, `\W`, `\d`, `\D`, `\s`, `\S`, `\b`, `[[:alpha:]]`, `[[:alnum:]]`} {
			if strings.Contains(pattern.String(), banned) {
				t.Errorf("%s uses %s (D49): use \\p{L}, \\p{N} or \\p{Z}", pattern, banned)
			}
		}
	}
}

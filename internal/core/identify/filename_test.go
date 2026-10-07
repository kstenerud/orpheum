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
		want ParsedPath
	}{
		// Films, as people name them.
		{"The Gold Rush (1999)/The Gold Rush (1999).mkv", ParsedPath{Kind: "movie", Name: "The Gold Rush", Year: 1999}},
		{"The.Gold.Rush.1999.1080p.BluRay.x264-GROUP.mkv", ParsedPath{Kind: "movie", Name: "The Gold Rush", Year: 1999}},
		{"Metropolis (1995)/movie.mkv", ParsedPath{Kind: "movie", Name: "Metropolis", Year: 1995}},
		{"Metropolis (1995)/Metropolis (1995) - 720p.mkv", ParsedPath{Kind: "movie", Name: "Metropolis", Year: 1995}},
		{"1917 (2019)/1917 (2019).mkv", ParsedPath{Kind: "movie", Name: "1917", Year: 2019}},
		{"7th Heaven (1968).mkv", ParsedPath{Kind: "movie", Name: "7th Heaven", Year: 1968}},
		{"Glass Runner 2061 (2017).mkv", ParsedPath{Kind: "movie", Name: "Glass Runner 2061", Year: 2017}},
		{"untitled rip final v2.mkv", ParsedPath{Kind: "movie", Name: "untitled rip final v2"}},
		// Not Latin: D49's cases, which ASCII character classes fail silently.
		{"Fièvre (2001)/Fièvre (2001).mkv", ParsedPath{Kind: "movie", Name: "Fièvre", Year: 2001}},
		{"狂った一頁 (2015).mkv", ParsedPath{Kind: "movie", Name: "狂った一頁", Year: 2015}},
		{"아리랑 (2019).mkv", ParsedPath{Kind: "movie", Name: "아리랑", Year: 2019}},
		{"Броненосец Потёмкин.1999.mkv", ParsedPath{Kind: "movie", Name: "Броненосец Потёмкин", Year: 1999}},
		// Provider ids in the name skip the search (D172).
		{"The Gold Rush (1999) {tmdb-4801}/The Gold Rush (1999).mkv", ParsedPath{
			Kind: "movie", Name: "The Gold Rush", Year: 1999, ProviderIDs: map[string]string{"tmdb": "4801"},
		}},
		{"Metropolis [imdbid-tt0234567].mkv", ParsedPath{
			Kind: "movie", Name: "Metropolis", ProviderIDs: map[string]string{"imdb": "tt0234567"},
		}},
		// Extras are typed by a suffix; containment makes them extras (D162).
		{"Metropolis (1995)/Metropolis (1995)-trailer.mkv", ParsedPath{
			Kind: "movie", Name: "Metropolis", Year: 1995, ExtraSuffix: "trailer",
		}},
		// An extra's own name; which film it belongs to is containment, decided above the parser.
		{"Metropolis (1995)/trailers/Metropolis teaser.mkv", ParsedPath{Kind: "movie", Name: "Metropolis teaser"}},
		// Episodes.
		{"Test Show/Season 01/Test Show - S01E01.mkv", ParsedPath{
			Kind: "episode", Name: "Test Show", Season: 1, Episode: 1,
		}},
		{"Show.Name.S02E10.720p.HDTV.mkv", ParsedPath{Kind: "episode", Name: "Show Name", Season: 2, Episode: 10}},
		{"Show/Season 1/S01E03-E04.mkv", ParsedPath{
			Kind: "episode", Name: "Show", Season: 1, Episode: 3, EpisodeEnd: 4,
		}},
		{"Show (2005)/s03e07.mkv", ParsedPath{Kind: "episode", Name: "Show", Year: 2005, Season: 3, Episode: 7}},
		{"Doctor Wren (2005)/3x07 - 42.mkv", ParsedPath{
			Kind: "episode", Name: "Doctor Wren", Year: 2005, Season: 3, Episode: 7,
		}},
		{"狂った一頁/Season 01/狂った一頁 S01E02.mkv", ParsedPath{Kind: "episode", Name: "狂った一頁", Season: 1, Episode: 2}},
		{"Show/Staffel 2/Folge S02E05.mkv", ParsedPath{Kind: "episode", Name: "Show", Season: 2, Episode: 5}},
		// A specials folder is season 0's, and names no series: every show's would be one otherwise.
		{"Show/Specials/Show - S00E01.mkv", ParsedPath{Kind: "episode", Name: "Show", Season: 0, Episode: 1}},
		{"Show/Season 1/Show - S01E02 - The Pilot.mkv", ParsedPath{
			Kind: "episode", Name: "Show", Season: 1, Episode: 2,
		}},
		// A season pack's folder names a season, not a series: the folder above it names the series,
		// or, with none, what the pack's name says before its season marker.
		{"Copper Kettle/Copper.Kettle.S02.COMPLETE.720p.ATVP.WEBRip.x264-HARBOR[SHELF]/Copper.Kettle.S02E01.720p.mkv", ParsedPath{
			Kind: "episode", Name: "Copper Kettle", Season: 2, Episode: 1,
		}},
		{"Copper Kettle/copper-kettle-s04/Copper.Kettle.S04E01.1080p.HEVC.x265-WICKER[PORCH.ex].mkv", ParsedPath{
			Kind: "episode", Name: "Copper Kettle", Season: 4, Episode: 1,
		}},
		{"Wexford Lasses/Wexford Lasses Season 1 Complete 720p HDTV x264 [i_c]/Wexford Lasses S01E01.mp4", ParsedPath{
			Kind: "episode", Name: "Wexford Lasses", Season: 1, Episode: 1,
		}},
		{"Slow.Ferrets.S01.COMPLETE.720p.ATVP.WEBRip.x264-HARBOR[SHELF]/Slow.Ferrets.S01E01.720p.mkv", ParsedPath{
			Kind: "episode", Name: "Slow Ferrets", Season: 1, Episode: 1,
		}},
		{"Quillwick.1982.S01-S04.1080p.BluRay.EAC3.2.0.x265-FERN/S01/Quillwick.S01E01.mkv", ParsedPath{
			Kind: "episode", Name: "Quillwick", Year: 1982, Season: 1, Episode: 1,
		}},
		// "Seasons 1 to 5" in a show's own folder is no season marker: the folder names the series.
		{
			"Star Wren Lower Rooms 2020 Seasons 1 to 5 720p Complete WEB [i_c]/Season 1/Lower Rooms - S01E01.mkv",
			ParsedPath{Kind: "episode", Name: "Star Wren Lower Rooms", Year: 2020, Season: 1, Episode: 1},
		},
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
	patterns := []*regexp.Regexp{
		providerIDPattern, episodePattern, seasonXEpisodePattern, seasonFolderPattern, seasonPackPattern, yearPattern,
	}
	for _, pattern := range patterns {
		for _, banned := range []string{`\w`, `\W`, `\d`, `\D`, `\s`, `\S`, `\b`, `[[:alpha:]]`, `[[:alnum:]]`} {
			if strings.Contains(pattern.String(), banned) {
				t.Errorf("%s uses %s (D49): use \\p{L}, \\p{N} or \\p{Z}", pattern, banned)
			}
		}
	}
}

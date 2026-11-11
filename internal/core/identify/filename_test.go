// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"maps"
	"reflect"
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
		{"The Gold Rush (1925)/The Gold Rush (1925).mkv", ParsedPath{Kind: "movie", Name: "The Gold Rush", Year: 1925}},
		{"The.Gold.Rush.1925.1080p.BluRay.x264-GROUP.mkv", ParsedPath{
			Kind: "movie", Name: "The Gold Rush", Year: 1925,
		}},
		{"Metropolis (1927)/movie.mkv", ParsedPath{Kind: "movie", Name: "Metropolis", Year: 1927}},
		{"Metropolis (1927)/Metropolis (1927) - 720p.mkv", ParsedPath{Kind: "movie", Name: "Metropolis", Year: 1927}},
		{"1917 (2019)/1917 (2019).mkv", ParsedPath{Kind: "movie", Name: "1917", Year: 2019}},
		{"7th Heaven (1927).mkv", ParsedPath{Kind: "movie", Name: "7th Heaven", Year: 1927}},
		{"Glass Runner 2061 (2017).mkv", ParsedPath{Kind: "movie", Name: "Glass Runner 2061", Year: 2017}},
		{"untitled rip final v2.mkv", ParsedPath{Kind: "movie", Name: "untitled rip final v2"}},
		// The competitors' corpora (design/research/library-scanner-practice-2026-10-16.md §5.3): a
		// release group's tag before the name, a title that is a year before its year, and an IMDb
		// id alone in brackets.
		{"[KESTREL.EX] Movie Name (2019) 1080p.mkv", ParsedPath{Kind: "movie", Name: "Movie Name", Year: 2019}},
		{"1917.2019.1080p.mkv", ParsedPath{Kind: "movie", Name: "1917", Year: 2019}},
		{"Metropolis [tt0234567].mkv", ParsedPath{
			Kind: "movie", Name: "Metropolis", ProviderIDs: map[string]string{"imdb": "tt0234567"},
		}},
		// A season and an episode said as separate words, which fell to season 1 silently.
		{"the ledger room s2 ep3 FINAL.mp4", ParsedPath{
			Kind: "episode", Name: "the ledger room", Season: 2, Episode: 3,
		}},
		{"Show S03 EP05 Name.mkv", ParsedPath{Kind: "episode", Name: "Show", Season: 3, Episode: 5}},
		{"Lockfast (1989) SE7 - EP5.mkv", ParsedPath{
			Kind: "episode", Name: "Lockfast", Year: 1989, Season: 7, Episode: 5,
		}},
		{"Show Season 2 Episode 5.mkv", ParsedPath{Kind: "episode", Name: "Show", Season: 2, Episode: 5}},
		// Not Latin: D49's cases, which ASCII character classes fail silently.
		{"Fièvre (1921)/Fièvre (1921).mkv", ParsedPath{Kind: "movie", Name: "Fièvre", Year: 1921}},
		{"狂った一頁 (1926).mkv", ParsedPath{Kind: "movie", Name: "狂った一頁", Year: 1926}},
		{"아리랑 (1926).mkv", ParsedPath{Kind: "movie", Name: "아리랑", Year: 1926}},
		{"Броненосец Потёмкин.1925.mkv", ParsedPath{Kind: "movie", Name: "Броненосец Потёмкин", Year: 1925}},
		// Provider ids in the name skip the search (D172).
		{"The Gold Rush (1925) {tmdb-4801}/The Gold Rush (1925).mkv", ParsedPath{
			Kind: "movie", Name: "The Gold Rush", Year: 1925, ProviderIDs: map[string]string{"tmdb": "4801"},
		}},
		{"Metropolis [imdbid-tt0234567].mkv", ParsedPath{
			Kind: "movie", Name: "Metropolis", ProviderIDs: map[string]string{"imdb": "tt0234567"},
		}},
		// Extras are typed by a suffix; containment makes them extras (D162).
		{"Metropolis (1927)/Metropolis (1927)-trailer.mkv", ParsedPath{
			Kind: "movie", Name: "Metropolis", Year: 1927, ExtraSuffix: "trailer",
		}},
		// An extra's own name; which film it belongs to is containment, decided above the parser.
		{"Metropolis (1927)/trailers/Metropolis teaser.mkv", ParsedPath{Kind: "movie", Name: "Metropolis teaser"}},
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
		{"Gus Doyle/Gus.Doyle.S02.COMPLETE.720p.ATVP.WEBRip.x264-HARBOR[SHELF]/Gus.Doyle.S02E01.720p.mkv", ParsedPath{
			Kind: "episode", Name: "Gus Doyle", Season: 2, Episode: 1,
		}},
		{"Gus Doyle/gus-doyle-s04/Gus.Doyle.S04E01.1080p.HEVC.x265-WICKER[PORCH.ex].mkv", ParsedPath{
			Kind: "episode", Name: "Gus Doyle", Season: 4, Episode: 1,
		}},
		{"Harlow Girls/Harlow Girls Season 1 Complete 720p HDTV x264 [i_c]/Harlow Girls S01E01.mp4", ParsedPath{
			Kind: "episode", Name: "Harlow Girls", Season: 1, Episode: 1,
		}},
		{"Quiet.Ravens.S01.COMPLETE.720p.ATVP.WEBRip.x264-HARBOR[SHELF]/Quiet.Ravens.S01E01.720p.mkv", ParsedPath{
			Kind: "episode", Name: "Quiet Ravens", Season: 1, Episode: 1,
		}},
		{"Greywolf.1982.S01-S04.1080p.BluRay.EAC3.2.0.x265-FERN/S01/Greywolf.S01E01.mkv", ParsedPath{
			Kind: "episode", Name: "Greywolf", Year: 1982, Season: 1, Episode: 1,
		}},
		// "Seasons 1 to 5" in a show's own folder is no season marker: the folder names the series.
		{
			"Nova Trail Night Berths 2020 Seasons 1 to 5 720p Complete WEB [i_c]/Season 1/Night Berths - S01E01.mkv",
			ParsedPath{Kind: "episode", Name: "Nova Trail Night Berths", Year: 2020, Season: 1, Episode: 1},
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
		episodeRunPattern, numberRunPattern, datePattern,
	}
	for _, pattern := range patterns {
		for _, banned := range []string{`\w`, `\W`, `\d`, `\D`, `\s`, `\S`, `\b`, `[[:alpha:]]`, `[[:alnum:]]`} {
			if strings.Contains(pattern.String(), banned) {
				t.Errorf("%s uses %s (D49): use \\p{L}, \\p{N} or \\p{Z}", pattern, banned)
			}
		}
	}
}

// Episodes as the owner's NAS lays them out (2026-10-13): a folder per episode, a folder per
// episode's number, a site's name before the release, extras and specials inside a series.
func TestParsePath_AnEpisodesFolderOrNumberNamesNoSeriesOfItsOwn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path            string
		name            string
		season, episode int
	}{
		{
			"Arkon Horizon/Arkon.Horizon.S01E01.1080p.x265-LANTERN/Arkon.Horizon.S01E01.1080p.x265-LANTERN.mkv",
			"Arkon Horizon", 1, 1,
		},
		{"Nova Trail Night Berths/S04/Nova.Trail.Night.Berths.S04E01.1080p.HEVC.x265-WICKER[SHELF]/" +
			"Nova.Trail.Night.Berths.S04E01.1080p.HEVC.x265-WICKER.mkv", "Nova Trail Night Berths", 4, 1},
		{
			"Ashwood/01.Ashwood.S01.720p.Bluray.x264.anoXmous/E01/Ashwood.S01E01.720p.Bluray.x264.anoXmous_.mp4",
			"Ashwood", 1, 1,
		},
		{"Paired at First Light/www.example.org    -    Wed.At.First.Light.AU.S13E01.1080p.HDTV.H264-TALLOW/" +
			"Wed.At.First.Light.AU.S13E01.1080p.HDTV.H264-TALLOW.mkv", "Paired at First Light", 13, 1},
		{"Arkon.Horizon.S01E02.1080p.x265-LANTERN/Arkon.Horizon.S01E02.1080p.x265-LANTERN.mkv", "Arkon Horizon", 1, 2},
		// A folder holding a season's is the series', though its name says specials (staging's series
		// 2405, 2026-10-28, named "" and matched nothing); and with no season folder, a name still.
		{"Some.Fathers.Dont.Arf.Try.Complete.Series.Plus.Christmas.Specials.DVDRip.H264(X-RG)/Series 1/" +
			"S01E01.mp4", "Some Fathers Dont Arf Try", 1, 1},
		{
			"Some.Fathers.Dont.Arf.Try.Complete.Series.Plus.Christmas.Specials.DVDRip.H264(X-RG)/S01E02.mp4",
			"Some Fathers Dont Arf Try", 1, 2,
		},
	}
	for _, test := range cases {
		parsed := ParsePath(test.path)
		if parsed.Kind != "episode" || parsed.Name != test.name || parsed.Season != test.season ||
			parsed.Episode != test.episode {
			t.Errorf("ParsePath(%q) = %q %q S%dE%d; want episode %q S%dE%d", test.path, parsed.Kind, parsed.Name,
				parsed.Season, parsed.Episode, test.name, test.season, test.episode)
		}
	}
}

func TestParseEpisodePath_InASeriesLibraryEverythingIsAnEpisodeOrASpecial(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want ParsedPath
	}{
		{"The Wanderer (1967)/The Wanderer - 01 - The Bells of Saint Quill.mkv", ParsedPath{
			Kind: "episode", Name: "The Wanderer", Year: 1967, Season: 1, Episode: 1,
			EpisodeName: "The Bells of Saint Quill",
		}},
		// The owner's NAS, 2026-10-16: an episode number of one digit after the season's "x".
		{"Grey Wolf/Greywolf - 1x4 - Born to be King.avi", ParsedPath{
			Kind: "episode", Name: "Grey Wolf", Season: 1, Episode: 4, EpisodeName: "Born to be King",
		}},
		// The owner's NAS, 2026-10-16: the series' name and the episode's number, with no marker.
		{"Cidade das Gaivotas/Cidade das Gaivotas 01 - A Coroa do Faroleiro.mkv", ParsedPath{
			Kind: "episode", Name: "Cidade das Gaivotas", Season: 1, Episode: 1, EpisodeName: "A Coroa do Faroleiro",
		}},
		{"VANISHED/VANISHED 02 Palm of the Hand (BDRip 1080p AC3 10bit).mkv", ParsedPath{
			Kind: "episode", Name: "VANISHED", Season: 1, Episode: 2, EpisodeName: "Palm of the Hand",
		}},
		{
			"Tsuki no Onna/[Hoshizora]_Kaito_Kumo_-_Tsuki_no_Onna_-_03_v2_[720p-10bit] [C66E9481].mkv",
			ParsedPath{Kind: "episode", Name: "Tsuki no Onna", Season: 1, Episode: 3},
		},
		// A miniseries' parts.
		{"Solitary Wren/Solitary Wren part 2.mkv", ParsedPath{
			Kind: "episode", Name: "Solitary Wren", Season: 1, Episode: 2,
		}},
		{"The Hollow Garden/The Hollow Garden - Part 3 - The Human Zoo.avi", ParsedPath{
			Kind: "episode", Name: "The Hollow Garden", Season: 1, Episode: 3, EpisodeName: "The Human Zoo",
		}},
		{"Pale Harrow/Pale Harrow BBC 1996 2 of 4.avi", ParsedPath{
			Kind: "episode", Name: "Pale Harrow", Season: 1, Episode: 2,
		}},
		{
			"Brindlewood/Brindlewood Season 5  [1080p x265 10bit FS89 Joy]/Featurettes/Six Seasons and A Finale.mkv",
			ParsedPath{
				Kind: "episode", Name: "Brindlewood", EpisodeName: "Six Seasons and A Finale", IsUnnumbered: true,
			},
		},
		{"Nova Trail Far Orbit Seven/Season 1/Nova Trail Far Orbit Seven - Season 1 - DVD Extras/" +
			"Far Orbit Seven A Bold Beginning.mkv", ParsedPath{
			Kind: "episode", Name: "Nova Trail Far Orbit Seven", EpisodeName: "Far Orbit Seven A Bold Beginning",
			IsUnnumbered: true,
		}},
		{"The Lab Crowd/Extras/The Lab Crowd Season 3 - Extras/Out-takes.avi", ParsedPath{
			Kind: "episode", Name: "The Lab Crowd", EpisodeName: "Out-takes", IsUnnumbered: true,
		}},
		{"Some.Fathers.Dont.Arf.Try.Complete.Series.Plus.Christmas.Specials.DVDRip.H264(BINGOWINGZ-UKB-RG)/" +
			"Christmas SPECIALS/Learning To Row.mp4", ParsedPath{
			Kind: "episode", Name: "Some Fathers Dont Arf Try",
			EpisodeName: "Learning To Row", IsUnnumbered: true,
		}},
		{
			"Grey Comet/The Promised Land 2020/Grey Comet The Promised Land 2020 1080p BluRay HEVC x265 5.1 BONE.mkv",
			ParsedPath{
				Kind: "episode", Name: "Grey Comet", EpisodeName: "Grey Comet The Promised Land",
				IsUnnumbered: true,
			},
		},
		{"The Hendersons/The Hendersons S05 720p x265/Special features - Deleted Scenes.mkv", ParsedPath{
			Kind: "episode", Name: "The Hendersons", EpisodeName: "Special features", IsUnnumbered: true,
		}},
	}
	for _, test := range cases {
		parsed := ParseEpisodePath(test.path)
		parsed.ProviderIDs, parsed.ExtraSuffix = nil, ""
		if !reflect.DeepEqual(parsed, test.want) {
			t.Errorf("ParseEpisodePath(%q) =\n %+v\nwant\n %+v", test.path, parsed, test.want)
		}
	}
}

// The owner's library, 2026-11-03: "Iron Ledger 1992" is a series' whole name, not Iron Ledger of 1992, and a name
// beneath its folder says so by carrying on past the number ("Iron Ledger 1992 - The Iron Ledger Story
// (2020)"). Where nothing beneath carries on, or the folder's year is in brackets, it stays the year.
func TestParsePath_AYearEndingASeriesFolderIsPartOfItsNameWhereANameBeneathCarriesOn(t *testing.T) {
	t.Parallel()
	const (
		show = "Iron Ledger 1992/"
		pack = "Iron Ledger 1992 - The Iron Ledger Story (2020) S01 Hindi (1080p WEBRip x265 10bit) - [Wayfarerboy]/"
		file = "Iron.Ledger.1992.The.Iron.Ledger.Story.S01E01.WEBRip.1080p.HEVC.10bit-[Wayfarerboy].mkv"
	)
	episode := func(name string, year int) ParsedPath {
		return ParsedPath{Kind: "episode", Name: name, Year: year, Season: 1, Episode: 1}
	}
	cases := []parsedCase{
		// The real paths: the pack's folder gives the whole name and its own year.
		{show + pack + file, episode("Iron Ledger 1992 - The Iron Ledger Story", 2020)},
		// The file's own name carries on, with no year of its own.
		{show + file, episode("Iron Ledger 1992 The Iron Ledger Story", 0)},
		// Nothing beneath carries on: the number stays the year.
		{show + "Season 1/Iron Ledger 1992 S01E01.mkv", episode("Iron Ledger", 1992)},
		{"Moss and Bramble 2020/Season 1/Moss.and.Bramble.S01E01.mkv", episode("Moss and Bramble", 2020)},
		// A year in brackets is the series' own, whatever follows beneath.
		{"Iron Ledger (1992)/" + file, episode("Iron Ledger", 1992)},
	}
	checkParsed(t, "ParsePath", ParsePath, cases)
	checkParsed(t, "ParseEpisodePath", ParseEpisodePath, cases)
	extra := ParseEpisodePath(show + pack + "Featurettes/Making Of.mkv")
	if extra.Name != "Iron Ledger 1992 - The Iron Ledger Story" || extra.Year != 2020 {
		t.Errorf("an extra in the pack is of %q (%d); want the whole name, 2020", extra.Name, extra.Year)
	}
}

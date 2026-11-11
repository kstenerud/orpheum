// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import "testing"

// Our own corpus of episodes' names, as filename_corpus_films_test.go's of films (D306 §1).

func TestParsePath_OurCorpusOfEpisodes(t *testing.T) {
	t.Parallel()
	episode := func(name string, year, season, first, last int) ParsedPath {
		return ParsedPath{Kind: "episode", Name: name, Year: year, Season: season, Episode: first, EpisodeEnd: last}
	}
	checkParsed(t, "ParsePath", ParsePath, []parsedCase{
		// The marker's spellings.
		{
			"Moss and Bramble/Season 1/Moss and Bramble - S01E01 - The Long Thaw.mkv",
			episode("Moss and Bramble", 0, 1, 1, 0),
		},
		{
			"Moss and Bramble/Season 01/Moss.and.Bramble.S01E02.720p.WEB.h264-MARLIN.mkv",
			episode("Moss and Bramble", 0, 1, 2, 0),
		},
		{"Moss and Bramble/Season 1/moss.and.bramble.s01e03.mkv", episode("Moss and Bramble", 0, 1, 3, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble S01 E04.mkv", episode("Moss and Bramble", 0, 1, 4, 0)},
		{"Moss and Bramble/Season 1/Moss_and_Bramble_S01E05.mkv", episode("Moss and Bramble", 0, 1, 5, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - s1e6.mkv", episode("Moss and Bramble", 0, 1, 6, 0)},
		{"Moss and Bramble/Season 1/Moss.and.Bramble.S01.E07.mkv", episode("Moss and Bramble", 0, 1, 7, 0)},
		{"Moss and Bramble/Season 1/Moss-and-Bramble-S01E08.mkv", episode("Moss and Bramble", 0, 1, 8, 0)},
		{"Moss and Bramble/Season 1/S01E09.mkv", episode("Moss and Bramble", 0, 1, 9, 0)},
		{"Moss and Bramble/Season 1/s01e10 - Lanterns.mkv", episode("Moss and Bramble", 0, 1, 10, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - 1x11 - Kites.mkv", episode("Moss and Bramble", 0, 1, 11, 0)},
		{"Moss and Bramble/Season 1/Moss.and.Bramble.1x12.mkv", episode("Moss and Bramble", 0, 1, 12, 0)},
		{"Moss and Bramble/Season 10/Moss and Bramble - 10x100.mkv", episode("Moss and Bramble", 0, 10, 100, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E101.mkv", episode("Moss and Bramble", 0, 1, 101, 0)},
		{"Moss and Bramble/Season 12/Moss.and.Bramble.S12E1004.mkv", episode("Moss and Bramble", 0, 12, 1004, 0)},
		{"Moss and Bramble/Season 2/Moss and Bramble S02 EP05.mkv", episode("Moss and Bramble", 0, 2, 5, 0)},
		{"Moss and Bramble/Season 2/moss and bramble s2 ep6 final.mp4", episode("Moss and Bramble", 0, 2, 6, 0)},
		{"Moss and Bramble/Moss and Bramble Season 2 Episode 7.mkv", episode("Moss and Bramble", 0, 2, 7, 0)},
		{"Moss and Bramble/Moss and Bramble SE2 - EP8.mkv", episode("Moss and Bramble", 0, 2, 8, 0)},
		{"Moss and Bramble/Moss and Bramble Staffel 2 Folge 9.mkv", episode("Moss and Bramble", 0, 2, 9, 0)},
		{"Moss and Bramble/Moss and Bramble Saison 2 Episode 10.mkv", episode("Moss and Bramble", 0, 2, 10, 0)},
		{"Moss and Bramble/Moss and Bramble Saison 2 Épisode 11.mkv", episode("Moss and Bramble", 0, 2, 11, 0)},
		{"Moss and Bramble/Moss and Bramble Temporada 2 Episodio 12.mkv", episode("Moss and Bramble", 0, 2, 12, 0)},

		// Multi-episode files.
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E07E08.mkv", episode("Moss and Bramble", 0, 1, 7, 8)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E09-E10.mkv", episode("Moss and Bramble", 0, 1, 9, 10)},
		{"Moss and Bramble/Season 1/Moss.and.Bramble.S01E09-E10.720p.mkv", episode("Moss and Bramble", 0, 1, 9, 10)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E11-12.mkv", episode("Moss and Bramble", 0, 1, 11, 12)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E13E14E15.mkv", episode("Moss and Bramble", 0, 1, 13, 15)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E16-E17-E18.mkv", episode("Moss and Bramble", 0, 1, 16, 18)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E19 - S01E20.mkv", episode("Moss and Bramble", 0, 1, 19, 20)},
		{"Moss and Bramble/Season 1/Moss.and.Bramble.S01E21.S01E22.mkv", episode("Moss and Bramble", 0, 1, 21, 22)},
		{
			"Moss and Bramble/Season 1/Moss.and.Bramble.S01E23-S01E24.1080p.mkv",
			episode("Moss and Bramble", 0, 1, 23, 24),
		},
		{"Moss and Bramble/Season 1/Moss and Bramble s01e01e02 - Pilot.mkv", episode("Moss and Bramble", 0, 1, 1, 2)},
		{"Moss and Bramble/Season 1/Moss and Bramble - 1x05-06.mkv", episode("Moss and Bramble", 0, 1, 5, 6)},
		{"Moss and Bramble/Season 1/Moss and Bramble - 1x07x08.mkv", episode("Moss and Bramble", 0, 1, 7, 8)},
		{"Moss and Bramble/Season 1/Moss and Bramble - 1x09-1x10.mkv", episode("Moss and Bramble", 0, 1, 9, 10)},
		// A number after a marker that is no episode: a resolution, a year, a lower number.
		{"Moss and Bramble/Season 1/Moss.and.Bramble.S01E02-1080p.mkv", episode("Moss and Bramble", 0, 1, 2, 0)},
		{"Moss and Bramble/Season 1/Moss.and.Bramble.S01E02.1080p.mkv", episode("Moss and Bramble", 0, 1, 2, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E04 - 2019.mkv", episode("Moss and Bramble", 0, 1, 4, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E05-03.mkv", episode("Moss and Bramble", 0, 1, 5, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E06 S02E01.mkv", episode("Moss and Bramble", 0, 1, 6, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - 1x07-2x01.mkv", episode("Moss and Bramble", 0, 1, 7, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - 1x08-720p.mkv", episode("Moss and Bramble", 0, 1, 8, 0)},
		{"Moss and Bramble/Season 1/Moss and Bramble - 1x01x02x03.mkv", episode("Moss and Bramble", 0, 1, 1, 3)},
		{
			"Moss and Bramble/Season 1/Moss and Bramble - S01E09 Episode Ten.mkv",
			episode("Moss and Bramble", 0, 1, 9, 0),
		},
		{
			"Moss and Bramble/Season 1/Moss and Bramble S01E03 720p x264 DDP5.1.mkv",
			episode("Moss and Bramble", 0, 1, 3, 0),
		},
		{"Moss and Bramble/Season 1/Moss.and.Bramble.S01E05.10bit.x265.mkv", episode("Moss and Bramble", 0, 1, 5, 0)},
		{"Moss and Bramble/Season 1/Moss.and.Bramble.S01E06.H.265.DDP5.1.mkv", episode("Moss and Bramble", 0, 1, 6, 0)},
		{
			"Moss and Bramble/Season 1/Moss and Bramble - S01E04 - 7.1 Surround.mkv",
			episode("Moss and Bramble", 0, 1, 4, 0),
		},

		// An episode's own name with a number in it.
		{
			"Moss and Bramble/Season 1/Moss and Bramble - S01E05 - 1000 Paper Cranes.mkv",
			episode("Moss and Bramble", 0, 1, 5, 0),
		},
		{"Moss and Bramble/Season 2/Moss and Bramble - S02E06 - 24 Hours.mkv", episode("Moss and Bramble", 0, 2, 6, 0)},
		{"Moss and Bramble/Season 2/Moss and Bramble - S02E07 - Part 2.mkv", episode("Moss and Bramble", 0, 2, 7, 0)},
		{"Moss and Bramble/Season 2/Moss and Bramble - S02E08 - 2 of 4.mkv", episode("Moss and Bramble", 0, 2, 8, 0)},
		{
			"Moss and Bramble/Season 2/Moss and Bramble - S02E09 - The 1999 Flood.mkv",
			episode("Moss and Bramble", 0, 2, 9, 0),
		},
		{
			"Moss and Bramble/Season 2/Moss and Bramble - S02E10 - Chapter IV.mkv",
			episode("Moss and Bramble", 0, 2, 10, 0),
		},
		{
			"Moss and Bramble/Season 2/Moss and Bramble - S02E11 - Season 3 Begins.mkv",
			episode("Moss and Bramble", 0, 2, 11, 0),
		},

		// Season folders.
		{"Moss and Bramble/S02/Moss and Bramble - S02E01.mkv", episode("Moss and Bramble", 0, 2, 1, 0)},
		{"Moss and Bramble/Series 2/Moss and Bramble - S02E02.mkv", episode("Moss and Bramble", 0, 2, 2, 0)},
		{"Moss and Bramble/Staffel 2/Moss and Bramble - S02E03.mkv", episode("Moss and Bramble", 0, 2, 3, 0)},
		{"Moss and Bramble/Saison 2/Moss and Bramble - S02E04.mkv", episode("Moss and Bramble", 0, 2, 4, 0)},
		{"Moss and Bramble/Temporada 2/Moss and Bramble - S02E05.mkv", episode("Moss and Bramble", 0, 2, 5, 0)},
		{"Moss and Bramble/season 02/Moss and Bramble - S02E06.mkv", episode("Moss and Bramble", 0, 2, 6, 0)},
		{"Moss and Bramble/Season.2/Moss and Bramble - S02E07.mkv", episode("Moss and Bramble", 0, 2, 7, 0)},
		{"Moss and Bramble/Season 2 (2021)/Moss and Bramble - S02E08.mkv", episode("Moss and Bramble", 0, 2, 8, 0)},
		// Specials: season 0's.
		{
			"Moss and Bramble/Specials/Moss and Bramble - S00E05 - Winter Lanterns.mkv",
			episode("Moss and Bramble", 0, 0, 5, 0),
		},
		{"Moss and Bramble/Season 0/Moss and Bramble - S00E06.mkv", episode("Moss and Bramble", 0, 0, 6, 0)},
		{"Moss and Bramble/Season 00/Moss and Bramble - S00E07.mkv", episode("Moss and Bramble", 0, 0, 7, 0)},
		{"Moss and Bramble/SPECIALS/Moss.and.Bramble.S00E08.mkv", episode("Moss and Bramble", 0, 0, 8, 0)},
		{"Moss and Bramble/Christmas Specials/Moss and Bramble - S00E09.mkv", episode("Moss and Bramble", 0, 0, 9, 0)},
		{"Moss and Bramble/Moss and Bramble - S00E10.mkv", episode("Moss and Bramble", 0, 0, 10, 0)},

		// A year with the series.
		{"Kestrel Point (2019)/Season 1/Kestrel Point (2019) - S01E01.mkv", episode("Kestrel Point", 2019, 1, 1, 0)},
		{"Kestrel Point (2019)/Season 2/Kestrel.Point.2019.S02E03.1080p.mkv", episode("Kestrel Point", 2019, 2, 3, 0)},
		{"Kestrel.Point.2019.S01E04.mkv", episode("Kestrel Point", 2019, 1, 4, 0)},
		{"Kestrel Point (2019) - S01E05.mkv", episode("Kestrel Point", 2019, 1, 5, 0)},
		{"Kestrel Point [2019]/Kestrel Point - S01E06.mkv", episode("Kestrel Point", 2019, 1, 6, 0)},
		{"Kestrel Point (1987)/Season 1/Kestrel Point - S01E01.mkv", episode("Kestrel Point", 1987, 1, 1, 0)},

		// Numbers and years in a series' name.
		{"24 Hours of Nightjar/Season 3/24.Hours.of.Nightjar.S03E07.mkv", episode("24 Hours of Nightjar", 0, 3, 7, 0)},
		{"24.Hours.of.Nightjar.S03E08.mkv", episode("24 Hours of Nightjar", 0, 3, 8, 0)},
		{"The 300 Rooms/Season 1/The 300 Rooms - 1x05 - Room 301.mkv", episode("The 300 Rooms", 0, 1, 5, 0)},
		{"The.300.Rooms.S01E06.mkv", episode("The 300 Rooms", 0, 1, 6, 0)},
		{"4-2-7 Dispatch/Season 2/4-2-7.Dispatch.S02E03.mkv", episode("4-2-7 Dispatch", 0, 2, 3, 0)},
		{"4-2-7.Dispatch.S02E04.720p.mkv", episode("4-2-7 Dispatch", 0, 2, 4, 0)},
		{"Twelve/Season 1/Twelve - S01E12.mkv", episode("Twelve", 0, 1, 12, 0)},
		{"1983 (2018)/Season 1/1983 - S01E01.mkv", episode("1983", 2018, 1, 1, 0)},
		{"1983/Season 1/1983 - S01E02.mkv", episode("1983", 0, 1, 2, 0)},
		{"2061.S01E02.mkv", episode("2061", 0, 1, 2, 0)},
		{"Station 2061 (2022)/Season 1/Station 2061 - S01E03.mkv", episode("Station 2061", 2022, 1, 3, 0)},
		{"Station.2061.2022.S01E04.mkv", episode("Station 2061", 2022, 1, 4, 0)},
		{"Nightjar II/Season 1/Nightjar II - S01E01.mkv", episode("Nightjar II", 0, 1, 1, 0)},
		{"Seven Seas Ferry/Season 7/Seven Seas Ferry - S07E07.mkv", episode("Seven Seas Ferry", 0, 7, 7, 0)},
		{"Route 9 Diner/Season 1/Route 9 Diner - 1x09.mkv", episode("Route 9 Diner", 0, 1, 9, 0)},

		// Season packs and episode folders.
		{
			"Moss.and.Bramble.S03.Complete.1080p.WEB-MARLIN/Moss.and.Bramble.S03E01.1080p.mkv",
			episode("Moss and Bramble", 0, 3, 1, 0),
		},
		{
			"Moss and Bramble/Moss.and.Bramble.S03.1080p/Moss.and.Bramble.S03E02.mkv",
			episode("Moss and Bramble", 0, 3, 2, 0),
		},
		{
			"Moss and Bramble Season 4 Complete 720p/Moss and Bramble S04E01.mkv",
			episode("Moss and Bramble", 0, 4, 1, 0),
		},
		{
			"Moss.and.Bramble.Complete.Series.720p/Season 1/Moss.and.Bramble.S01E01.mkv",
			episode("Moss and Bramble", 0, 1, 1, 0),
		},
		{
			"Moss and Bramble/Moss.and.Bramble.S01E03.720p-MARLIN/Moss.and.Bramble.S01E03.720p-MARLIN.mkv",
			episode("Moss and Bramble", 0, 1, 3, 0),
		},
		{
			"Moss.and.Bramble.S01E04.720p-MARLIN/Moss.and.Bramble.S01E04.720p-MARLIN.mkv",
			episode("Moss and Bramble", 0, 1, 4, 0),
		},
		{
			"Moss and Bramble/Moss.and.Bramble.S01-S03.1080p/S02/Moss.and.Bramble.S02E05.mkv",
			episode("Moss and Bramble", 0, 2, 5, 0),
		},
		{
			"Moss and Bramble/Moss.and.Bramble.S01.720p/E06/Moss.and.Bramble.S01E06.mkv",
			episode("Moss and Bramble", 0, 1, 6, 0),
		},

		// Not Latin.
		{"Ведьмин Лес/Season 1/Ведьмин Лес - S01E01.mkv", episode("Ведьмин Лес", 0, 1, 1, 0)},
		{"Ведьмин.Лес.S01E02.1080p.mkv", episode("Ведьмин Лес", 0, 1, 2, 0)},
		{"Les Phares du Nord/Saison 1/Les Phares du Nord - S01E02.mkv", episode("Les Phares du Nord", 0, 1, 2, 0)},
		{"灯台の町/Season 1/灯台の町 S01E03.mkv", episode("灯台の町", 0, 1, 3, 0)},
		{"灯台の町.S01E04.1080p.mkv", episode("灯台の町", 0, 1, 4, 0)},
		{"Ψαράδες/Season 2/Ψαράδες.S02E01.mkv", episode("Ψαράδες", 0, 2, 1, 0)},
		{"Élan Vital/Staffel 1/Élan Vital - 1x02.mkv", episode("Élan Vital", 0, 1, 2, 0)},
		{"ＮＩＧＨＴＪＡＲ/Season 1/ＮＩＧＨＴＪＡＲ S01E01.mkv", episode("ＮＩＧＨＴＪＡＲ", 0, 1, 1, 0)},
		{"등대 마을/Season 1/등대 마을 - S01E05.mkv", episode("등대 마을", 0, 1, 5, 0)},

		// Provider ids, a release group, a site, the case.
		{"Moss and Bramble [tvdbid-389123]/Season 1/Moss and Bramble - S01E01.mkv", ParsedPath{
			Kind: "episode", Name: "Moss and Bramble", Season: 1, Episode: 1,
			ProviderIDs: map[string]string{"tvdb": "389123"},
		}},
		{"Moss and Bramble {tmdb-99123}/Season 1/Moss and Bramble - S01E02.mkv", ParsedPath{
			Kind: "episode", Name: "Moss and Bramble", Season: 1, Episode: 2,
			ProviderIDs: map[string]string{"tmdb": "99123"},
		}},
		{"Moss and Bramble (2020) [imdbid-tt9123456]/Season 1/Moss and Bramble - S01E03.mkv", ParsedPath{
			Kind: "episode", Name: "Moss and Bramble", Year: 2020, Season: 1, Episode: 3,
			ProviderIDs: map[string]string{"imdb": "tt9123456"},
		}},
		{"[KITE] Moss and Bramble - S01E02 [720p].mkv", episode("Moss and Bramble", 0, 1, 2, 0)},
		{"[KITE] Moss.and.Bramble.S01E03.mkv", episode("Moss and Bramble", 0, 1, 3, 0)},
		{"[KITE] Moss and Bramble - S01E04 [1080p][A1B2C3D4].mkv", episode("Moss and Bramble", 0, 1, 4, 0)},
		{"www.KestrelLinks.org - Moss.and.Bramble.S02E02.mkv", episode("Moss and Bramble", 0, 2, 2, 0)},
		{"MOSS.AND.BRAMBLE.S02E03.MKV", episode("MOSS AND BRAMBLE", 0, 2, 3, 0)},
		{"moss and bramble/season 2/moss and bramble - s02e04.mkv", episode("moss and bramble", 0, 2, 4, 0)},
		{"Moss and Bramble/Season 1/Moss.and.Bramble.S01E01.720p-sample.mkv", ParsedPath{
			Kind: "episode", Name: "Moss and Bramble", Season: 1, Episode: 1, ExtraSuffix: "sample",
		}},
		{"Moss and Bramble/Season 1/Moss and Bramble - S01E02-featurette.mkv", ParsedPath{
			Kind: "episode", Name: "Moss and Bramble", Season: 1, Episode: 2, ExtraSuffix: "featurette",
		}},
	})
}

func TestParseEpisodePath_OurCorpusOfASeriesLibrary(t *testing.T) {
	t.Parallel()
	numbered := func(name string, season, number int, episodeName string) ParsedPath {
		return ParsedPath{Kind: "episode", Name: name, Season: season, Episode: number, EpisodeName: episodeName}
	}
	unnumbered := func(name, episodeName string) ParsedPath {
		return ParsedPath{Kind: "episode", Name: name, EpisodeName: episodeName, IsUnnumbered: true}
	}
	checkParsed(t, "ParseEpisodePath", ParseEpisodePath, []parsedCase{
		// A marker places it as in any library.
		{"Orchard Road/Season 1/Orchard Road - S01E01 - Lanterns.mkv", numbered("Orchard Road", 1, 1, "")},
		{"Orchard Road/Orchard Road - 2x10 - Rain.mkv", numbered("Orchard Road", 2, 10, "")},
		{"Orchard Road/orchard road s2 ep3 final.mp4", numbered("Orchard Road", 2, 3, "")},
		{"Orchard Road/Orchard Road S03 EP05 Rain.mkv", numbered("Orchard Road", 3, 5, "")},
		{"Orchard Road/Staffel 2/Orchard Road Staffel 2 Folge 7.mkv", numbered("Orchard Road", 2, 7, "")},

		// No marker: the number after the series' name, between dashes, or in "1x4".
		{"Orchard Road/Orchard Road - 02.mkv", numbered("Orchard Road", 1, 2, "")},
		{"Orchard Road/Orchard Road 03 The Ferry.mkv", numbered("Orchard Road", 1, 3, "The Ferry")},
		{"Orchard Road/Orchard Road - 1x4 - The Ferry Home.mkv", numbered("Orchard Road", 1, 4, "The Ferry Home")},
		{"Orchard Road/Orchard Road 2x5.mkv", numbered("Orchard Road", 2, 5, "")},
		{"Orchard Road/Season 2/Orchard Road - 05 - Night Market.mkv", numbered("Orchard Road", 2, 5, "Night Market")},
		{"Orchard Road/S02/Orchard Road - 03 - Fog.mkv", numbered("Orchard Road", 2, 3, "Fog")},
		{"Orchard Road/Series 3/Orchard Road - 01 - Tide.mkv", numbered("Orchard Road", 3, 1, "Tide")},
		{"Orchard Road/Staffel 4/Orchard Road - 02 - Ebbe.mkv", numbered("Orchard Road", 4, 2, "Ebbe")},
		{"Orchard Road/Saison 5/Orchard Road - 06 - Marée.mkv", numbered("Orchard Road", 5, 6, "Marée")},
		{"Orchard Road/Temporada 6/Orchard Road - 07 - Marea.mkv", numbered("Orchard Road", 6, 7, "Marea")},
		{"Orchard Road/Season 07/Orchard Road - 08 - Kites.mkv", numbered("Orchard Road", 7, 8, "Kites")},
		{"Orchard Road/season.8/Orchard Road - 09 - Gulls.mkv", numbered("Orchard Road", 8, 9, "Gulls")},
		{
			"Orchard Road/Orchard.Road.S04.1080p.WEB/Orchard Road - 02 - Kites.mkv",
			numbered("Orchard Road", 4, 2, "Kites"),
		},
		{"Orchard Road/Orchard Road - 10 - The 39 Steps.mkv", numbered("Orchard Road", 1, 10, "The 39 Steps")},
		{"Orchard Road/Orchard Road - 11 - Part 2.mkv", numbered("Orchard Road", 1, 11, "Part 2")},
		{"Orchard Road/Orchard Road - 12 - 1999.mkv", numbered("Orchard Road", 1, 12, "1999")},
		{"Orchard Road/Orchard_Road_-_13_[720p].mkv", numbered("Orchard Road", 1, 13, "")},
		{"Orchard Road/Orchard.Road.14.720p.mkv", numbered("Orchard Road", 1, 14, "")},
		{"Orchard Road/Orchard Road 15v2.mkv", numbered("Orchard Road", 1, 15, "")},
		{"Orchard Road/Orchard Road - 16 (1080p).mkv", numbered("Orchard Road", 1, 16, "")},
		{"orchard road/ORCHARD ROAD - 17.mkv", numbered("orchard road", 1, 17, "")},
		{"Orchard Road/Orchard Road - 18 - Lanterns.mkv", numbered("Orchard Road", 1, 18, "Lanterns")},

		// "E05", "Episode 5", and a folder named for the number.
		{"Orchard Road/Orchard Road E07.mkv", numbered("Orchard Road", 1, 7, "Orchard Road E07")},
		{"Orchard Road/Season 3/Episode 4.mkv", numbered("Orchard Road", 3, 4, "Episode 4")},
		{"Orchard Road/Season 3/Ep 5.mkv", numbered("Orchard Road", 3, 5, "Ep 5")},
		{"Orchard Road/Season 1/E05/orchard.road.e05.mkv", numbered("Orchard Road", 1, 5, "orchard road e05")},
		{"Orchard Road/Season 1/E06/video.mkv", numbered("Orchard Road", 1, 6, "video")},
		{"Orchard Road/Season 2/Episode 07/Orchard Road.mkv", numbered("Orchard Road", 2, 7, "Orchard Road")},

		// A miniseries' parts.
		{"Lanternfall/Lanternfall Part 2.mkv", numbered("Lanternfall", 1, 2, "")},
		{"Lanternfall/Lanternfall - Part 3 - The Ember.mkv", numbered("Lanternfall", 1, 3, "The Ember")},
		{"Lanternfall/Lanternfall (3 of 6).mkv", numbered("Lanternfall", 1, 3, "")},
		{"Lanternfall/Lanternfall.4of6.720p.mkv", numbered("Lanternfall", 1, 4, "")},
		{"Lanternfall/Lanternfall 5 of 6 - The Last Light.mkv", numbered("Lanternfall", 1, 5, "The Last Light")},
		{"Lanternfall/Lanternfall.Part.1.1080p.mkv", numbered("Lanternfall", 1, 1, "")},
		{"Lanternfall/Lanternfall part1.mkv", numbered("Lanternfall", 1, 1, "")},
		// A part named in words or numerals is left for a person, never misnumbered.
		{"Lanternfall/Lanternfall - Pt.II.mkv", unnumbered("Lanternfall", "Lanternfall - Pt.II")},
		{"Lanternfall/Lanternfall Part Two.mkv", unnumbered("Lanternfall", "Lanternfall Part Two")},

		// Absolute numbering, fansub tags and checksums.
		{"Kitsune Relay/[Hanabi] Kitsune Relay - 07 [1080p][ABCD1234].mkv", numbered("Kitsune Relay", 1, 7, "")},
		{"Kitsune Relay/[Hanabi] Kitsune Relay - 103 [1080p].mkv", numbered("Kitsune Relay", 1, 103, "")},
		{"Kitsune Relay/[Hanabi]_Kitsune_Relay_-_08_[720p][1A2B3C4D].mkv", numbered("Kitsune Relay", 1, 8, "")},
		{"Kitsune Relay/Kitsune Relay - 09v2 [BD 1080p].mkv", numbered("Kitsune Relay", 1, 9, "")},
		{"Kitsune Relay/Kitsune Relay 12 [ABCD1234].mkv", numbered("Kitsune Relay", 1, 12, "")},
		{"Kitsune Relay/Kitsune Relay - 05 (1080p) [ABCDEF12].mkv", numbered("Kitsune Relay", 1, 5, "")},
		{"Kitsune Relay/Kitsune Relay - 04 - The Fox Who Ran.mkv", numbered("Kitsune Relay", 1, 4, "The Fox Who Ran")},
		{"Kitsune Relay/[Group2023] Kitsune Relay - 06 [1080p].mkv", numbered("Kitsune Relay", 1, 6, "")},
		{"Kitsune Relay/[Hanabi] Kitsune Relay - 13 [12345678].mkv", numbered("Kitsune Relay", 1, 13, "")},
		{"Kitsune Relay/[Hanabi] Kitsune Relay - 14 [10bit][x265].mkv", numbered("Kitsune Relay", 1, 14, "")},
		{"Kitsune Relay/Kitsune Relay - 15 [Hanabi][1080p].mkv", numbered("Kitsune Relay", 1, 15, "")},
		{"Kitsune Relay (2023)/[Hanabi] Kitsune Relay - 02 [1080p].mkv", ParsedPath{
			Kind: "episode", Name: "Kitsune Relay", Year: 2023, Season: 1, Episode: 2,
		}},
		{"Relay 7/[Hanabi] Relay 7 - 03 [1080p].mkv", numbered("Relay 7", 1, 3, "")},
		{"Kitsune Relay/Season 2/[Hanabi] Kitsune Relay - 16 [1080p].mkv", numbered("Kitsune Relay", 2, 16, "")},
		// More digits than an episode's number is read with: left for a person.
		{
			"Kitsune Relay/[Hanabi] Kitsune Relay - 1085 [1080p].mkv",
			unnumbered("Kitsune Relay", "Kitsune Relay - 1085"),
		},

		// Numbers in the series' name.
		{"Station 9/Station 9 - 04 - Relief.mkv", numbered("Station 9", 1, 4, "Relief")},
		{"Station 9/Station 9 05.mkv", numbered("Station 9", 1, 5, "")},
		{"The 300 Rooms/The 300 Rooms - 07.mkv", numbered("The 300 Rooms", 1, 7, "")},
		{"24 Hours of Nightjar/24 Hours of Nightjar 06.mkv", numbered("24 Hours of Nightjar", 1, 6, "")},
		{"4-2-7 Dispatch/4-2-7 Dispatch - 08.mkv", numbered("4-2-7 Dispatch", 1, 8, "")},
		{"1983 (2018)/1983 - 02 - Snow.mkv", ParsedPath{
			Kind: "episode", Name: "1983", Year: 2018, Season: 1, Episode: 2, EpisodeName: "Snow",
		}},
		// A number inside the episode's own name is not the episode's.
		{"Orchard Road/Specials/The 12 Lanterns.mkv", unnumbered("Orchard Road", "The 12 Lanterns")},
		{"Orchard Road/Season 1/Lantern 42.mkv", unnumbered("Orchard Road", "Lantern 42")},

		// Specials and season 0.
		{"Orchard Road/Specials/Lantern Night.mkv", unnumbered("Orchard Road", "Lantern Night")},
		{"Orchard Road/Season 0/The Ferry Christmas.mkv", unnumbered("Orchard Road", "The Ferry Christmas")},
		{"Orchard Road/Specials/Orchard Road - S00E02.mkv", numbered("Orchard Road", 0, 2, "")},
		{"Orchard Road/Specials/Orchard Road - 03 - Holiday.mkv", unnumbered("Orchard Road", "Holiday")},
		{"Orchard Road/Orchard Road - The Lost Reel.mkv", unnumbered("Orchard Road", "Orchard Road - The Lost Reel")},
		{"Orchard Road/Holiday Specials/Snowbound Ferry.mkv", unnumbered("Orchard Road", "Snowbound Ferry")},
		{
			"Orchard Road/Orchard Road The Movie (2022)/Orchard Road The Movie (2022).mkv",
			unnumbered("Orchard Road", "Orchard Road The Movie"),
		},

		// Extras inside a series.
		{"Orchard Road/Featurettes/Making the Ferry.mkv", unnumbered("Orchard Road", "Making the Ferry")},
		{"Orchard Road/Behind The Scenes/On Location.mkv", unnumbered("Orchard Road", "On Location")},
		{"Orchard Road/Season 1/Deleted Scenes/Market Cut.mkv", unnumbered("Orchard Road", "Market Cut")},
		{"Orchard Road/Extras/Bloopers.mkv", unnumbered("Orchard Road", "Bloopers")},
		{"Orchard Road/Interviews/Cast Interview.mkv", unnumbered("Orchard Road", "Cast Interview")},
		{"Orchard Road/Season 2/Bonus/Season 2 Recap.mkv", unnumbered("Orchard Road", "Season 2 Recap")},
		{"Orchard Road/Trailers/Orchard Road Season 2.mkv", unnumbered("Orchard Road", "Orchard Road Season 2")},
		{"Orchard Road/Orchard Road-trailer.mkv", ParsedPath{
			Kind: "episode", Name: "Orchard Road", EpisodeName: "Orchard Road", IsUnnumbered: true,
			ExtraSuffix: "trailer",
		}},
		{"Orchard Road/Season 1/Orchard.Road.S01E01.720p-sample.mkv", ParsedPath{
			Kind: "episode", Name: "Orchard Road", Season: 1, Episode: 1, ExtraSuffix: "sample",
		}},

		// Stacked parts and dates are out of the beta (D306 §4, §5): never misnumbered.
		{"Orchard Road/Season 1/Orchard Road - cd1.mkv", unnumbered("Orchard Road", "Orchard Road - cd1")},
		{"Orchard Road/Orchard Road - disc2.mkv", unnumbered("Orchard Road", "Orchard Road - disc2")},
		{"Harbor Lights/Harbor Lights 2019.03.14.mkv", unnumbered("Harbor Lights", "Harbor Lights")},
		{"Harbor Lights/Harbor.Lights.2019.03.14.720p.mkv", unnumbered("Harbor Lights", "Harbor Lights")},
		{"Harbor Lights/Harbor Lights - 2019-03-14 - Guest Night.mkv", unnumbered("Harbor Lights", "Harbor Lights")},
		{"Harbor Lights/Harbor Lights 14.03.2019.mkv", unnumbered("Harbor Lights", "Harbor Lights 14.03")},
		{"Harbor Lights/Harbor Lights 2019 03 14.mkv", unnumbered("Harbor Lights", "Harbor Lights")},
		{"Harbor Lights/Harbor Lights (March 14, 2019).mkv", unnumbered("Harbor Lights", "Harbor Lights")},
		{"Harbor Lights/2019/Harbor Lights 2019-03-14.mkv", unnumbered("Harbor Lights", "Harbor Lights")},

		// Not Latin.
		{"Ведьмин Лес/Season 1/Ведьмин Лес - 03 - Туман.mkv", numbered("Ведьмин Лес", 1, 3, "Туман")},
		{"Ведьмин Лес/Ведьмин Лес 04.mkv", numbered("Ведьмин Лес", 1, 4, "")},
		{"灯台の町/灯台の町 - 04.mkv", numbered("灯台の町", 1, 4, "")},
		{"灯台の町/Specials/灯台の町 夏の夜.mkv", unnumbered("灯台の町", "灯台の町 夏の夜")},
		{
			"Les Phares du Nord/Saison 2/Les Phares du Nord - 06 - L'Écume.mkv",
			numbered("Les Phares du Nord", 2, 6, "L'Écume"),
		},
		{"Élan Vital/Élan Vital 07.mkv", numbered("Élan Vital", 1, 7, "")},
		{"Ψαράδες/Ψαράδες - 02 - Αυγή.mkv", numbered("Ψαράδες", 1, 2, "Αυγή")},

		// Provider ids and a site's name.
		{"Orchard Road [tvdbid-412345]/Season 1/Orchard Road - 02 - Fog.mkv", ParsedPath{
			Kind: "episode", Name: "Orchard Road", Season: 1, Episode: 2, EpisodeName: "Fog",
			ProviderIDs: map[string]string{"tvdb": "412345"},
		}},
		{"Orchard Road {tmdb-77123}/Orchard Road - 03.mkv", ParsedPath{
			Kind: "episode", Name: "Orchard Road", Season: 1, Episode: 3,
			ProviderIDs: map[string]string{"tmdb": "77123"},
		}},
		{"Orchard Road/www.KestrelLinks.org - Orchard Road - 04 - Dusk.mkv", numbered("Orchard Road", 1, 4, "Dusk")},
	})
}

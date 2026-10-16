// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"reflect"
	"testing"
)

// The corpus below is our own (D306 §1): every name is composed for this file in the shapes and
// with the difficulties other projects' suites test, and none is copied from one. The series and
// films are invented. A shape the parser does not read by design (stacked parts, dates, D306 §4
// and §5) asserts the safe reading: never a wrong season or episode.

// parsedCase is one path and what the parser should read from it.
type parsedCase struct {
	path string
	want ParsedPath
}

// checkParsed runs each case through parse and compares every field; a case that names no
// provider ids wants none.
func checkParsed(t *testing.T, name string, parse func(string) ParsedPath, cases []parsedCase) {
	t.Helper()
	for _, test := range cases {
		got := parse(test.path)
		if test.want.ProviderIDs == nil {
			test.want.ProviderIDs = map[string]string{}
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s(%q)\n got %+v\nwant %+v", name, test.path, got, test.want)
		}
	}
}

func TestParsePath_OurCorpusOfFilms(t *testing.T) {
	t.Parallel()
	checkParsed(t, "ParsePath", ParsePath, []parsedCase{
		// Separators: spaces, dots, underscores, doubled spaces, brackets.
		{
			"The Glass Cartographer (2014)/The Glass Cartographer (2014).mkv",
			ParsedPath{Kind: "movie", Name: "The Glass Cartographer", Year: 2014},
		},
		{
			"The.Glass.Cartographer.2014.1080p.WEB-DL.DDP5.1.H.264-QUILL.mkv",
			ParsedPath{Kind: "movie", Name: "The Glass Cartographer", Year: 2014},
		},
		{
			"the_glass_cartographer_2014_720p.mkv",
			ParsedPath{Kind: "movie", Name: "the glass cartographer", Year: 2014},
		},
		{
			"The Glass  Cartographer  (2014).mkv",
			ParsedPath{Kind: "movie", Name: "The Glass Cartographer", Year: 2014},
		},
		{
			"The Glass Cartographer [2014].mkv",
			ParsedPath{Kind: "movie", Name: "The Glass Cartographer", Year: 2014},
		},
		{
			"The Glass Cartographer [2014] [1080p].mkv",
			ParsedPath{Kind: "movie", Name: "The Glass Cartographer", Year: 2014},
		},
		{
			"The Glass Cartographer 2014.mkv",
			ParsedPath{Kind: "movie", Name: "The Glass Cartographer", Year: 2014},
		},
		{
			"The Glass Cartographer (2014) 1080p BluRay x265.mkv",
			ParsedPath{Kind: "movie", Name: "The Glass Cartographer", Year: 2014},
		},
		{
			"THE.GLASS.CARTOGRAPHER.2014.1080P.BLURAY.X264.MKV",
			ParsedPath{Kind: "movie", Name: "THE GLASS CARTOGRAPHER", Year: 2014},
		},
		{
			"the glass cartographer (2014).mp4",
			ParsedPath{Kind: "movie", Name: "the glass cartographer", Year: 2014},
		},
		{
			"Glass Cartographer/the.glass.cartographer.2014.avi",
			ParsedPath{Kind: "movie", Name: "the glass cartographer", Year: 2014},
		},
		{
			"Velvet.Static.2016.2160p.UHD.BluRay.x265.10bit.HDR.DTS-HD.MA.7.1-NOVA.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{"Velvet.Static.2016.DDP5.1.H.265-QUILL.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet Static (2016) 1920x1080.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet Static (2016) 10bit HEVC.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet_Static_(2016)_[1080p]_[x264].mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet.Static.REPACK.1080p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static"}},
		{"Velvet Static PROPER 720p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static"}},
		{"Velvet.Static.1080p.BluRay.x264.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static"}},
		{"Velvet Static.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static"}},
		{"Velvet Static (2016)/velvet.static.1080p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet Static (2016)/VS.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{
			"Films/Drama/Velvet Static (2016)/Velvet Static (2016).mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{"Copperfield Station (1952).mkv", ParsedPath{Kind: "movie", Name: "Copperfield Station", Year: 1952}},
		{
			"Copperfield Station (1952) [Remastered].mkv",
			ParsedPath{Kind: "movie", Name: "Copperfield Station", Year: 1952},
		},
		{
			"Copperfield.Station.1952.Remastered.720p.mkv",
			ParsedPath{Kind: "movie", Name: "Copperfield Station", Year: 1952},
		},
		{
			"Copperfield Station, The (1952).mkv",
			ParsedPath{Kind: "movie", Name: "Copperfield Station, The", Year: 1952},
		},
		{
			"Moonlit Ferry - A Harbor Tale (2008).mkv",
			ParsedPath{Kind: "movie", Name: "Moonlit Ferry - A Harbor Tale", Year: 2008},
		},
		{
			"Moonlit Ferry: A Harbor Tale (2008).mkv",
			ParsedPath{Kind: "movie", Name: "Moonlit Ferry: A Harbor Tale", Year: 2008},
		},
		{"Moss & Thistle (1977).mkv", ParsedPath{Kind: "movie", Name: "Moss & Thistle", Year: 1977}},
		{
			"Who Stole the Lighthouse? (1961).mkv",
			ParsedPath{Kind: "movie", Name: "Who Stole the Lighthouse?", Year: 1961},
		},
		{"Kestrel!.2003.mkv", ParsedPath{Kind: "movie", Name: "Kestrel!", Year: 2003}},

		// Years and numbers in titles.
		{"Station 2061 (2019).mkv", ParsedPath{Kind: "movie", Name: "Station 2061", Year: 2019}},
		{"Station.2061.2019.1080p.mkv", ParsedPath{Kind: "movie", Name: "Station 2061", Year: 2019}},
		{"1984 (1956).mkv", ParsedPath{Kind: "movie", Name: "1984", Year: 1956}},
		{"1999.2003.1080p.BluRay.mkv", ParsedPath{Kind: "movie", Name: "1999", Year: 2003}},
		{"1999 2003.mkv", ParsedPath{Kind: "movie", Name: "1999", Year: 2003}},
		{"1917.mkv", ParsedPath{Kind: "movie", Name: "1917"}},
		{"2026 (2026).mkv", ParsedPath{Kind: "movie", Name: "2026", Year: 2026}},
		{"Lantern 1999 (1999).mkv", ParsedPath{Kind: "movie", Name: "Lantern 1999", Year: 1999}},
		{"Summer of 1969 (2004).mkv", ParsedPath{Kind: "movie", Name: "Summer of 1969", Year: 2004}},
		{"The 39 Lanterns (1935).mkv", ParsedPath{Kind: "movie", Name: "The 39 Lanterns", Year: 1935}},
		{"The 100 Steps (2011).mkv", ParsedPath{Kind: "movie", Name: "The 100 Steps", Year: 2011}},
		{"Harbor's Nine (2001).mkv", ParsedPath{Kind: "movie", Name: "Harbor's Nine", Year: 2001}},
		{"Eleven Harbors (2001).mkv", ParsedPath{Kind: "movie", Name: "Eleven Harbors", Year: 2001}},
		{"6-1-6 Lullaby (2008).mkv", ParsedPath{Kind: "movie", Name: "6-1-6 Lullaby", Year: 2008}},
		{"24 Hours in Kestrel (2012).mkv", ParsedPath{Kind: "movie", Name: "24 Hours in Kestrel", Year: 2012}},
		{"Lantern 13 (1995).mkv", ParsedPath{Kind: "movie", Name: "Lantern 13", Year: 1995}},
		{"Seven Ferries (1960).mkv", ParsedPath{Kind: "movie", Name: "Seven Ferries", Year: 1960}},
		{"Velvet Static 2 (2019).mkv", ParsedPath{Kind: "movie", Name: "Velvet Static 2", Year: 2019}},
		{"Velvet.Static.2.2019.1080p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static 2", Year: 2019}},
		{"Velvet Static 3D (2020).mkv", ParsedPath{Kind: "movie", Name: "Velvet Static 3D", Year: 2020}},
		{"Kestrel 2x4 (2009).mkv", ParsedPath{Kind: "movie", Name: "Kestrel 2x4", Year: 2009}},
		{"The 3x3 Club (2011).mkv", ParsedPath{Kind: "movie", Name: "The 3x3 Club", Year: 2011}},
		{"Route 66 Lanterns (1999).mkv", ParsedPath{Kind: "movie", Name: "Route 66 Lanterns", Year: 1999}},
		{"Room 1080 (2015).mkv", ParsedPath{Kind: "movie", Name: "Room 1080", Year: 2015}},
		{
			"Velvet Static: Chapter 3 (2021).mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static: Chapter 3", Year: 2021},
		},
		{"Lanternfall Part 2 (2015).mkv", ParsedPath{Kind: "movie", Name: "Lanternfall Part 2", Year: 2015}},
		{"Lanternfall.Pt.II.2015.mkv", ParsedPath{Kind: "movie", Name: "Lanternfall Pt II", Year: 2015}},
		{"Lanternfall - Part Two (2016).mkv", ParsedPath{Kind: "movie", Name: "Lanternfall - Part Two", Year: 2016}},

		// Roman numerals.
		{"Kestrel Point II (1998).mkv", ParsedPath{Kind: "movie", Name: "Kestrel Point II", Year: 1998}},
		{
			"Kestrel Point III - The Return (2001).mkv",
			ParsedPath{Kind: "movie", Name: "Kestrel Point III - The Return", Year: 2001},
		},
		{"Kestrel.Point.IV.2004.720p.mkv", ParsedPath{Kind: "movie", Name: "Kestrel Point IV", Year: 2004}},
		{"Kestrel Point Part VI (2010).mkv", ParsedPath{Kind: "movie", Name: "Kestrel Point Part VI", Year: 2010}},

		// Versions and editions: the cut is not the name.
		{
			"Velvet Static (2016)/Velvet Static (2016) - 1080p.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) - 4K HDR.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) - Director's Cut.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) - Theatrical.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) - Director's Cut - 1080p.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) {edition-Director's Cut}.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) {edition-Extended Cut} {tmdb-48213}.mkv",
			ParsedPath{
				Kind: "movie", Name: "Velvet Static", Year: 2016,
				ProviderIDs: map[string]string{"tmdb": "48213"},
			},
		},
		{"Velvet.Static.2016.Extended.1080p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet.Static.2016.UNRATED.720p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet.Static.2016.Directors.Cut.1080p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet.Static.2016.IMAX.2160p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{
			"Velvet Static Extended Edition (2016).mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{"Velvet Static Unrated (2016).mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet Static Director's Cut (2016).mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"Velvet Static Theatrical Cut (2016).mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		// A cut's words name the cut only as a pair: these are titles.
		{"The Director's Lantern (2010).mkv", ParsedPath{Kind: "movie", Name: "The Director's Lantern", Year: 2010}},
		{"Theatrical Embers (2001).mkv", ParsedPath{Kind: "movie", Name: "Theatrical Embers", Year: 2001}},
		{"Velvet Static Remastered (2016).mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},

		// Provider ids, written or bare.
		{
			"Velvet Static (2016) [tt7654321].mkv",
			ParsedPath{
				Kind: "movie", Name: "Velvet Static", Year: 2016,
				ProviderIDs: map[string]string{"imdb": "tt7654321"},
			},
		},
		{
			"Velvet Static (2016) {tmdb-48213}.mkv",
			ParsedPath{
				Kind: "movie", Name: "Velvet Static", Year: 2016,
				ProviderIDs: map[string]string{"tmdb": "48213"},
			},
		},
		{
			"Velvet Static {tmdb-48213}.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", ProviderIDs: map[string]string{"tmdb": "48213"}},
		},
		{
			"Velvet Static (2016) {tmdb=48213}.mkv",
			ParsedPath{
				Kind: "movie", Name: "Velvet Static", Year: 2016,
				ProviderIDs: map[string]string{"tmdb": "48213"},
			},
		},
		{
			"Velvet Static (2016) {imdb-tt7654321}.mkv",
			ParsedPath{
				Kind: "movie", Name: "Velvet Static", Year: 2016,
				ProviderIDs: map[string]string{"imdb": "tt7654321"},
			},
		},
		{
			"Velvet Static (2016) [imdbid-tt7654321]/Velvet Static.mkv",
			ParsedPath{
				Kind: "movie", Name: "Velvet Static", Year: 2016,
				ProviderIDs: map[string]string{"imdb": "tt7654321"},
			},
		},
		{
			"Velvet Static (2016) [tmdbid-48213]/Velvet Static (2016).mkv",
			ParsedPath{
				Kind: "movie", Name: "Velvet Static", Year: 2016,
				ProviderIDs: map[string]string{"tmdb": "48213"},
			},
		},
		{"Velvet Static (2016) {tmdb-48213} [tt7654321].mkv", ParsedPath{
			Kind: "movie", Name: "Velvet Static", Year: 2016,
			ProviderIDs: map[string]string{"tmdb": "48213", "imdb": "tt7654321"},
		}},
		{
			"Velvet.Static.2016.[tt7654321].1080p.mkv",
			ParsedPath{
				Kind: "movie", Name: "Velvet Static", Year: 2016,
				ProviderIDs: map[string]string{"imdb": "tt7654321"},
			},
		},

		// A release group's tag before the name, a site's before the release.
		{"[QUILL] Velvet Static (2016) [1080p].mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"[QUILL] Velvet.Static.2016.1080p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"[2160p] Velvet Static (2016).mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{
			"[KITE-RIPS] The Glass Cartographer 2014 BDRip.mkv",
			ParsedPath{Kind: "movie", Name: "The Glass Cartographer", Year: 2014},
		},
		{"www.ReelHarbor.org - Velvet Static (2016).mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{"ReelHarbor.net - Velvet.Static.2016.720p.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},

		// Extras: a suffix types one, a folder holds one (D162).
		{
			"Velvet Static (2016)/Velvet Static (2016)-trailer.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "trailer"},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016)-featurette.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "featurette"},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016)-behindthescenes.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "behindthescenes"},
		},
		{
			"Velvet Static (2016)/Velvet Static-Behind The Scenes.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "behindthescenes"},
		},
		{
			"Velvet Static (2016)/Velvet Static - Deleted Scene.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "deletedscene"},
		},
		{
			"Velvet Static (2016)/Velvet Static-deleted.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "deleted"},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) - Trailer.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "trailer"},
		},
		{
			"Velvet Static (2016)/Velvet Static-teaser.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "teaser"},
		},
		{
			"Velvet Static (2016)/Velvet Static-interview.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "interview"},
		},
		{
			"Velvet Static (2016)/Velvet Static-scene.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "scene"},
		},
		{
			"Velvet Static (2016)/Velvet Static-short.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "short"},
		},
		{
			"Velvet Static (2016)/Velvet Static-clip.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "clip"},
		},
		{
			"Velvet Static (2016)/Velvet Static-TRAILERS.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "trailer"},
		},
		{
			"Velvet Static (2016)/Velvet Static-theme.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "theme"},
		},
		{
			"Velvet Static (2016)/Velvet Static-other.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "other"},
		},
		{
			"Velvet Static (2016)/Featurettes/Making the Static.mkv",
			ParsedPath{Kind: "movie", Name: "Making the Static"},
		},
		{
			"Velvet Static (2016)/Behind The Scenes/On Set Day 3.mkv",
			ParsedPath{Kind: "movie", Name: "On Set Day 3"},
		},
		{
			"Velvet Static (2016)/Deleted Scenes/Alternate Ending.mkv",
			ParsedPath{Kind: "movie", Name: "Alternate Ending"},
		},
		{
			"Velvet Static (2016)/Interviews/The Director Talks.mkv",
			ParsedPath{Kind: "movie", Name: "The Director Talks"},
		},
		{"Velvet Static (2016)/Trailers/Teaser 2.mkv", ParsedPath{Kind: "movie", Name: "Teaser 2"}},
		{"Velvet Static (2016)/Extras/Gag Reel.mkv", ParsedPath{Kind: "movie", Name: "Gag Reel"}},
		{
			"Velvet Static (2016)/Shorts/Static Prelude (2015).mkv",
			ParsedPath{Kind: "movie", Name: "Static Prelude", Year: 2015},
		},
		// A spaced word that only ends like a suffix is no extra.
		{
			"Kestrel Point - The Other Shore (2003).mkv",
			ParsedPath{Kind: "movie", Name: "Kestrel Point - The Other Shore", Year: 2003},
		},
		{"Ferry-Trailer Park (1994).mkv", ParsedPath{Kind: "movie", Name: "Ferry-Trailer Park", Year: 1994}},

		// Samples: named and typed, never a film of their own name.
		{
			"Velvet Static (2016)/Sample/Velvet.Static.2016.sample.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet.Static.2016-sample.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016, ExtraSuffix: "sample"},
		},
		{"Velvet Static (2016)/sample.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},

		// Stacked parts are out of the beta (D306 §5): a film still, never an episode.
		{
			"Velvet Static (2016)/Velvet Static (2016) - cd1.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) - cd2.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) - part1.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) - disc 2.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{
			"Velvet Static (2016)/Velvet Static (2016) pt2.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},
		{"Velvet.Static.2016.CD2.mkv", ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016}},
		{
			"Velvet Static (2016)/Velvet Static (2016) - 1 of 2.mkv",
			ParsedPath{Kind: "movie", Name: "Velvet Static", Year: 2016},
		},

		// Dates are out of scope (D306 §4): in a film library they never make an episode.
		{"Harbor Lights 2019.03.14.mkv", ParsedPath{Kind: "movie", Name: "Harbor Lights", Year: 2019}},
		{"Harbor Lights 2019-03-14.mkv", ParsedPath{Kind: "movie", Name: "Harbor Lights", Year: 2019}},
		{"Harbor.Lights.2019.03.14.Guest.Night.720p.mkv", ParsedPath{Kind: "movie", Name: "Harbor Lights", Year: 2019}},

		// Not Latin, and full-width (D49).
		{"Le Cœur des Phares (1987).mkv", ParsedPath{Kind: "movie", Name: "Le Cœur des Phares", Year: 1987}},
		{"Ça Ira (1999).mkv", ParsedPath{Kind: "movie", Name: "Ça Ira", Year: 1999}},
		{"Ölfeld (2004).mkv", ParsedPath{Kind: "movie", Name: "Ölfeld", Year: 2004}},
		{"Żurawie nad Wisłą (1972).mkv", ParsedPath{Kind: "movie", Name: "Żurawie nad Wisłą", Year: 1972}},
		{"Ψαράδες της Αυγής (2011).mkv", ParsedPath{Kind: "movie", Name: "Ψαράδες της Αυγής", Year: 2011}},
		{"灯台守 (2009).mkv", ParsedPath{Kind: "movie", Name: "灯台守", Year: 2009}},
		{"灯台守.2009.1080p.mkv", ParsedPath{Kind: "movie", Name: "灯台守", Year: 2009}},
		{"灯台守　(2009).mkv", ParsedPath{Kind: "movie", Name: "灯台守", Year: 2009}},
		{"灯台守（2009）.mkv", ParsedPath{Kind: "movie", Name: "灯台守", Year: 2009}},
		{"등대지기 (2015).mkv", ParsedPath{Kind: "movie", Name: "등대지기", Year: 2015}},
		{"Смотритель маяка (2019).mkv", ParsedPath{Kind: "movie", Name: "Смотритель маяка", Year: 2019}},
		{"Маяк.2006.1080p.mkv", ParsedPath{Kind: "movie", Name: "Маяк", Year: 2006}},
		{"منارة الشمال (2010).mkv", ParsedPath{Kind: "movie", Name: "منارة الشمال", Year: 2010}},
		{"प्रकाशस्तंभ (2012).mkv", ParsedPath{Kind: "movie", Name: "प्रकाशस्तंभ", Year: 2012}},
		{"ＶＥＬＶＥＴ ＳＴＡＴＩＣ (2016).mkv", ParsedPath{Kind: "movie", Name: "ＶＥＬＶＥＴ ＳＴＡＴＩＣ", Year: 2016}},
		{
			"Fièvre à la Plage (2001)/Fièvre à la Plage (2001) - 1080p.mkv",
			ParsedPath{Kind: "movie", Name: "Fièvre à la Plage", Year: 2001},
		},
	})
}

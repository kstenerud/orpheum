// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// ParsedPath is what a path says about the work in it: a hint, never an authority
// (identification.md). The probe wins on matters of fact, and a person's answer wins over both.
type ParsedPath struct {
	// Kind is "movie" or "episode"; empty when the path says neither.
	Kind string
	// Name is the title as the path writes it, cleaned of release tags and separators.
	Name string
	// Year is the release year the path gives, or 0.
	Year int
	// Season and Episode locate an episode; EpisodeEnd is the last episode of a multi-episode
	// file, or 0.
	Season, Episode, EpisodeEnd int
	// ProviderIDs are ids written into the path, "{tmdb-4801}" or "[imdbid-tt0123456]", by
	// provider: they skip the search (D172).
	ProviderIDs map[string]string
	// ExtraSuffix is a trailing "-trailer", "-featurette" and so on. It types an extra; it never
	// makes one (D162).
	ExtraSuffix string
	// EpisodeName is an episode's own name as its file writes it, for one no listing names: an
	// unnumbered special, or "Show - 01 - Its Name".
	EpisodeName string
	// IsUnnumbered is an episode of a series library that no marker or folder numbers: a special
	// (season 0), a film or a featurette inside a series' folders, placed by its path.
	IsUnnumbered bool
}

// The expressions. D49: no \w, \d, \s, \b or [[:alpha:]] — \w and \s are ASCII-only in Go and
// fail silently on "Fièvre" and "狂った一頁" — so \p{L}, \p{N} and \p{Z}. Where only ASCII
// digits are meant, because strconv.Atoi reads nothing else, they are spelled [0-9].
//
//nolint:gocritic // regexpSimplify's \d is the class D49 bans; [0-9] says the ASCII is meant
var (
	providerIDPattern = regexp.MustCompile(`[\[{](tmdb|tmdbid|imdb|imdbid|tvdb|tvdbid)[-=]([0-9A-Za-z]+)[\]}]`)
	episodePattern    = regexp.MustCompile(
		`(?i)(?:^|[^\p{L}\p{N}])s([0-9]{1,3})[ ._-]?e([0-9]{1,4})(?:[ ._-]?-?e([0-9]{1,4}))?`)
	seasonXEpisodePattern = regexp.MustCompile(`(?:^|[^\p{L}\p{N}])([0-9]{1,2})x([0-9]{2,3})(?:[^\p{L}\p{N}]|$)`)
	seasonFolderPattern   = regexp.MustCompile(
		`(?i)^(?:season|series|staffel|saison|temporada)[\p{Z} ._-]*([0-9]{1,3})$|^s([0-9]{1,3})$`)
	// A season pack's folder carries a season marker as a word inside its release name, which
	// seasonPackPattern finds, as in "Show.S02.COMPLETE.720p", "show-s04", "Show Season 1 Complete"
	// and "Show.1982.S01-S04". An episode marker, "S02E01", is not one.
	seasonPackPattern = regexp.MustCompile(`(?i)(?:^|[\p{Z} ._-])(?:s[0-9]{1,3}|` +
		`(?:season|series|staffel|saison|temporada)[\p{Z} ._-]*[0-9]{1,3})(?:[\p{Z} ._-]|$)`)
	yearPattern = regexp.MustCompile(`(?:^|[\p{Z} ._(\[-])((?:19|20)[0-9]{2})(?:[\p{Z} ._)\]-]|$)`)
	// A site's name a download put before the release name: "www.example.org - Show.S13E01".
	sitePrefixPattern = regexp.MustCompile(
		`(?i)^[\p{Z} ]*(?:www\.)?[\p{L}\p{N}-]+\.(?:org|com|net|to|info|io|cc|me|tv|ru|se|nz|ws)[\p{Z} ]*[-–][\p{Z} ]*`)
	// A folder or a file named only for an episode's number: "E01", "Ep 3", "Episode 12".
	episodeOnlyPattern = regexp.MustCompile(`(?i)^(?:e|ep|episode)[\p{Z} ._-]*([0-9]{1,4})$`)
	// An episode number a name carries without a season: "Show E05 Title", "Show Episode 5".
	loneEpisodePattern = regexp.MustCompile(
		`(?i)(?:^|[^\p{L}\p{N}])(?:e|ep|episode)[\p{Z} ._-]?([0-9]{1,4})(?:[^\p{L}\p{N}]|$)`)
	// "Show - 01 - Its Name": a number alone between dashes, the season left unsaid.
	dashNumberPattern = regexp.MustCompile(`[\p{Z} ._]-[\p{Z} ._]*([0-9]{1,3})[\p{Z} ._]*-[\p{Z} ._]`)
)

// isReleaseTag reports whether a word ends a title in a release name: what follows it is about
// the encoding, not the work.
func isReleaseTag(word string) bool {
	switch strings.ToLower(strings.Trim(word, "[]()-")) {
	case "2160p", "1080p", "1080i", "720p", "576p", "480p", "4k", "uhd", "bluray", "blu-ray",
		"bdrip", "brrip", "remux", "web-dl", "webdl", "webrip", "web", "hdtv", "dvdrip", "dvd",
		"x264", "x265", "h264", "h265", "hevc", "avc", "hdr", "hdr10", "dv", "10bit", "aac", "ac3",
		"dts", "truehd", "atmos", "proper", "repack", "extended", "unrated", "directors",
		"remastered", "imax":
		return true
	}
	return false
}

// extraSuffixes type an extra by its file name's last dash-separated part (D162).
func extraSuffixes() []string {
	return []string{
		extraTrailer, "teaser", extraClip, extraFeaturette, "behindthescenes", "deleted", "deletedscene",
		extraInterview, extraShort, extraScene, "sample", "theme", "other",
	}
}

// ParsePath reads a path relative to its library root. Episode markers in the file name win;
// then the name and year come from the file's stem, or from its folder when the stem says
// nothing a title would.
func ParsePath(relative string) ParsedPath {
	parsed := ParsedPath{ProviderIDs: map[string]string{}}
	for _, match := range providerIDPattern.FindAllStringSubmatch(relative, -1) {
		provider := strings.TrimSuffix(strings.ToLower(match[1]), "id")
		parsed.ProviderIDs[provider] = match[2]
	}
	folder, file := path.Split(relative)
	stem := sitePrefixPattern.ReplaceAllString(strings.TrimSuffix(file, path.Ext(file)), "")
	stem, parsed.ExtraSuffix = cutExtraSuffix(stem)

	if marker, found := findEpisode(stem); found {
		parsed.Kind = library.TitleKindEpisode
		parsed.Season, parsed.Episode, parsed.EpisodeEnd = marker.season, marker.episode, marker.lastEpisode
		// The folders name a series more reliably than the words before "S02E05", which may be
		// the episode's own name or "Folge" ("episode"); the file name is the fallback.
		parsed.Name, parsed.Year = seriesFromFolders(folder)
		if parsed.Name == "" {
			parsed.Name, parsed.Year = cleanTitle(stem[:marker.startIndex])
		}
		return parsed
	}
	parsed.Kind = KindMovie
	parsed.Name, parsed.Year = cleanTitle(stem)
	if parsed.Year == 0 || parsed.Name == "" {
		// "Metropolis (1995)/movie.mkv", "Metropolis (1995)/metropolis-1080p.mkv": the folder names the work.
		if name, year := cleanTitle(path.Base(strings.TrimSuffix(folder, "/"))); year != 0 {
			parsed.Name, parsed.Year = name, year
		}
	}
	return parsed
}

// episodeMarker is an S01E02, S01E02-E03 or 1x02 found in a stem, and where it starts.
type episodeMarker struct {
	season, episode, lastEpisode, startIndex int
}

func findEpisode(stem string) (episodeMarker, bool) {
	if match := episodePattern.FindStringSubmatchIndex(stem); match != nil {
		marker := episodeMarker{
			season: parseNonNegativeInt(stem[match[2]:match[3]]), episode: parseNonNegativeInt(stem[match[4]:match[5]]),
			startIndex: match[0],
		}
		if match[6] >= 0 {
			marker.lastEpisode = parseNonNegativeInt(stem[match[6]:match[7]])
		}
		return marker, true
	}
	if match := seasonXEpisodePattern.FindStringSubmatchIndex(stem); match != nil {
		season, episode := parseNonNegativeInt(stem[match[2]:match[3]]), parseNonNegativeInt(stem[match[4]:match[5]])
		return episodeMarker{season: season, episode: episode, startIndex: match[0]}, true
	}
	return episodeMarker{}, false
}

// seriesFromFolders names a series from the folders above an episode: "Show/Season 01/x.mkv" and
// "Show/Specials/x.mkv" are Show. A season pack's folder, "Show/Show.S02.COMPLETE.720p/x.mkv", names
// a season too, not a series of its own, so the folder above it names the series; with none above,
// what the pack's name says before its season marker does (the owner's NAS, 2026-10-07: each of Ted
// Lasso's seasons was a series of its own).
//
// A folder of one episode, "Show/Show.S01E01.1080p/x.mkv", or one named only for its number,
// "Show/Show.S01/E01/x.mkv", and a specials or extras folder are passed over the same way (the
// owner's NAS, 2026-10-13: each of Wing Vellmoor's episodes, and Corvessa's "E01" to "E12", were
// series of their own).
func seriesFromFolders(folder string) (name string, year int) {
	packPrefix := ""
	for _, part := range slices.Backward(strings.Split(strings.Trim(folder, "/"), "/")) {
		part = strings.TrimSpace(sitePrefixPattern.ReplaceAllString(part, ""))
		if part == "" || isSeasonFolder(part) || episodeOnlyPattern.MatchString(part) || isBonusFolder(part) {
			continue
		}
		if marker, found := findEpisode(part); found {
			if packPrefix == "" {
				packPrefix = part[:marker.startIndex]
			}
			continue
		}
		if marker := seasonPackPattern.FindStringIndex(part); marker != nil {
			if packPrefix == "" {
				packPrefix = part[:marker[0]]
			}
			continue
		}
		return cleanTitle(part)
	}
	return cleanTitle(packPrefix)
}

// isBonusFolder is a series' folder of specials or extras: "Specials", "Christmas SPECIALS",
// "Featurettes", "DVD Extras", "Special features" and D162's extras folders.
func isBonusFolder(name string) bool {
	if _, isExtras := extraTypeOfFolder(name); isExtras {
		return true
	}
	lower := strings.ToLower(name)
	for _, word := range []string{"special", "extras", "featurette", "bonus", "deleted scene", "behind the scenes"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// ParseEpisodePath reads a path in a series library, where every video is an episode of a series
// (the owner, 2026-10-13: "when something is marked as a series, it should be impossible for any of
// its contents to be classified as films"). A marker in the file name places it as ParsePath does;
// else "Show - 01 - Its Name", "E01" or "Episode 1" numbers it, in the season its folders name or
// the first; else, or in a specials or extras folder, it is a special of season 0, its name the
// file's, placed by its path: a featurette, or a film made of the series (Brackenmere's "The Promised
// Land"), which its series' listing of specials may name.
func ParseEpisodePath(relative string) ParsedPath {
	parsed := ParsePath(relative)
	if parsed.Kind == library.TitleKindEpisode {
		return parsed
	}
	folder, file := path.Split(relative)
	stem := sitePrefixPattern.ReplaceAllString(strings.TrimSuffix(file, path.Ext(file)), "")
	stem, _ = cutExtraSuffix(stem)
	parsed.Kind = library.TitleKindEpisode
	parsed.Name, parsed.Year = outermostSeriesFolder(folder)
	season, isBonus := seasonFromFolders(folder)
	before, episodeName := "", stem
	if match := dashNumberPattern.FindStringSubmatchIndex(stem); match != nil {
		parsed.Episode = parseNonNegativeInt(stem[match[2]:match[3]])
		before, episodeName = stem[:match[0]], stem[match[1]:]
	} else if match := loneEpisodePattern.FindStringSubmatchIndex(stem); match != nil {
		parsed.Episode = parseNonNegativeInt(stem[match[2]:match[3]])
		before = stem[:match[0]]
	} else if number, found := episodeFolderNumber(folder); found {
		parsed.Episode = number
	}
	if parsed.Name == "" {
		parsed.Name, parsed.Year = cleanTitle(before)
	}
	if parsed.Name == "" {
		parsed.Name, parsed.Year = cleanTitle(stem)
	}
	parsed.EpisodeName, _ = cleanTitle(episodeName)
	if parsed.EpisodeName == "" {
		parsed.EpisodeName = strings.TrimSpace(episodeName)
	}
	switch {
	case isBonus || parsed.Episode == 0:
		parsed.Season, parsed.Episode, parsed.IsUnnumbered = 0, 0, true
	case season >= 0:
		parsed.Season = season
	default:
		parsed.Season = 1
	}
	return parsed
}

// outermostSeriesFolder names the series of a file no marker places by the folder just under the
// library's root, where a series library keeps each series: a featurette or a film inside a series'
// folders is that series', not its own folder's ("Brackenmere/The Promised Land 2020/x.mkv" is Red
// Dwarf's). When that folder is itself an episode's or a season pack's, the folders say as for an
// episode.
func outermostSeriesFolder(folder string) (name string, year int) {
	parts := strings.Split(strings.Trim(folder, "/"), "/")
	top := strings.TrimSpace(sitePrefixPattern.ReplaceAllString(parts[0], ""))
	_, isEpisodeFolder := findEpisode(top)
	if top == "" || isEpisodeFolder || isSeasonFolder(top) || episodeOnlyPattern.MatchString(top) ||
		seasonPackPattern.MatchString(top) {
		return seriesFromFolders(folder)
	}
	return cleanTitle(top)
}

// seasonFromFolders is the season the nearest season folder above a file names, -1 for none, and
// whether a specials or extras folder holds it.
func seasonFromFolders(folder string) (season int, isBonus bool) {
	for _, part := range slices.Backward(strings.Split(strings.Trim(folder, "/"), "/")) {
		part = strings.TrimSpace(part)
		if isBonusFolder(part) {
			return 0, true
		}
		if match := seasonFolderPattern.FindStringSubmatch(part); match != nil {
			return parseNonNegativeInt(match[1] + match[2]), false
		}
		if marker := seasonPackPattern.FindString(part); marker != "" {
			digits := strings.TrimFunc(marker, func(r rune) bool { return r < '0' || r > '9' })
			return parseNonNegativeInt(digits), false
		}
	}
	return -1, false
}

// episodeFolderNumber is the number of the "E01" folder nearest above a file, if there is one.
func episodeFolderNumber(folder string) (int, bool) {
	for _, part := range slices.Backward(strings.Split(strings.Trim(folder, "/"), "/")) {
		if match := episodeOnlyPattern.FindStringSubmatch(strings.TrimSpace(part)); match != nil {
			return parseNonNegativeInt(match[1]), true
		}
	}
	return 0, false
}

// cleanTitle turns a release-style stem into a title and its year: separators become spaces,
// provider-id markers and bracketed groups go, and everything from the year or the first release
// tag on is cut.
func cleanTitle(stem string) (name string, year int) {
	text := providerIDPattern.ReplaceAllString(stem, " ")
	if !strings.ContainsFunc(text, unicode.IsSpace) {
		text = strings.NewReplacer(".", " ", "_", " ").Replace(text)
	}
	if matches := yearPattern.FindAllStringSubmatchIndex(text, -1); len(matches) > 0 {
		// The last year-looking number is the year; an earlier one is part of the name, as in
		// "7th Heaven (1968)" or "Glass Runner 2061 (2017)".
		last := matches[len(matches)-1]
		// A number that is the whole name is the name: "1917".
		if last[0] > 0 {
			year = parseNonNegativeInt(text[last[2]:last[3]])
			text = text[:last[0]]
		}
	}
	words := strings.Fields(text)
	for index, word := range words {
		// "Complete" ends a pack's name, "Show.COMPLETE.SERIES", but may begin a title.
		if isReleaseTag(word) || (index > 0 && strings.EqualFold(word, "complete")) {
			words = words[:index]
			break
		}
	}
	name = strings.Join(words, " ")
	name = strings.TrimFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == '(' || r == '[' || r == '.' || r == ','
	})
	return name, year
}

// cutExtraSuffix removes a trailing "-trailer"-style part from a stem, returning its type.
func cutExtraSuffix(stem string) (rest, suffix string) {
	dash := strings.LastIndex(stem, "-")
	if dash < 0 {
		return stem, ""
	}
	suffix = strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(stem[dash+1:]))
	for _, known := range extraSuffixes() {
		if suffix == known || suffix == known+"s" {
			return strings.TrimSpace(stem[:dash]), known
		}
	}
	return stem, ""
}

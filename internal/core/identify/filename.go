// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/textindex"
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
	episodePattern    = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])s([0-9]{1,3})[ ._-]?e([0-9]{1,4})`)
	// The rest of a multi-episode file's run, read from where the number before it ends, each
	// match its season if it says one and its episode: "E02", "-E02", " - S01E02" or ".S01E02"
	// after "S01E01", and "-02", "x02" or "-1x02" after either "S01E01" or "1x01". The function
	// lastOfRun says which continue the run.
	episodeRunPattern = regexp.MustCompile(
		`(?i)^(?:[\p{Z} ._-]*s([0-9]{1,3})[\p{Z} ._-]?e|[\p{Z} ._-]?-?e)([0-9]{1,4})`)
	numberRunPattern = regexp.MustCompile(`^(?:-(?:([0-9]{1,2})x)?|x)([0-9]{1,3})(?:[^\p{L}\p{N}]|x|$)`)
	// A season and an episode said as words apart: "s2 ep3", "S03 EP05", "SE7 - EP5", "Season 2
	// Episode 5". The episode's word is required, so "Se7en" is no marker.
	wordedEpisodePattern = regexp.MustCompile(
		`(?i)(?:^|[^\p{L}\p{N}])(?:s|se|season|series|staffel|saison|temporada)[\p{Z} ._-]*([0-9]{1,3})` +
			`[\p{Z} ._-]*(?:-[\p{Z} ._-]*)?(?:e|ep|episode|épisode|folge|episodio)` +
			`[\p{Z} ._-]*([0-9]{1,4})(?:[^\p{L}\p{N}]|$)`)
	// An IMDb id alone in brackets: "[tt0234567]".
	bareIMDbPattern = regexp.MustCompile(`\[(tt[0-9]{7,9})\]`)
	// A release group's tag before the name: "[KESTREL.EX] Movie Name".
	leadingGroupPattern = regexp.MustCompile(`^[\p{Z} ]*\[[^\]]*\][\p{Z} ._-]*`)
	// "1x02", and "1x02x03", whose run lastOfRun reads.
	seasonXEpisodePattern = regexp.MustCompile(`(?:^|[^\p{L}\p{N}])([0-9]{1,2})x([0-9]{2,3})(?:[^\p{L}\p{N}]|x|$)`)
	seasonFolderPattern   = regexp.MustCompile(
		`(?i)^(?:season|series|staffel|saison|temporada)[\p{Z} ._-]*([0-9]{1,3})$|^s([0-9]{1,3})$`)
	// A season pack's folder carries a season marker as a word inside its release name, which
	// seasonPackPattern finds, as in "Show.S02.COMPLETE.720p", "show-s04", "Show Season 1 Complete"
	// and "Show.1982.S01-S04". An episode marker, "S02E01", is not one.
	seasonPackPattern = regexp.MustCompile(`(?i)(?:^|[\p{Z} ._-])(?:s[0-9]{1,3}|` +
		`(?:season|series|staffel|saison|temporada)[\p{Z} ._-]*[0-9]{1,3})(?:[\p{Z} ._-]|$)`)
	// A year, in brackets too, full-width ones included: "灯台守（2009）".
	yearPattern = regexp.MustCompile(`(?:^|[\p{Z} ._(\[（［)\]）］-])((?:19|20)[0-9]{2})(?:[\p{Z} ._)\]）］-]|$)`)
	// A date, day first or year first: "14.03.2019", "2019-03-14". Daily shows are out of the
	// beta (D306 §4); a date is only kept from being read as an episode's number.
	datePattern = regexp.MustCompile(`(?:^|[^\p{L}\p{N}])(?:[0-9]{1,2}[./-][0-9]{1,2}[./-](?:19|20)[0-9]{2}|` +
		`(?:19|20)[0-9]{2}[./-][0-9]{1,2}[./-][0-9]{1,2})(?:[^\p{L}\p{N}]|$)`)
	// A site's name a download put before the release name: "www.example.org - Show.S13E01".
	sitePrefixPattern = regexp.MustCompile(
		`(?i)^[\p{Z} ]*(?:www\.)?[\p{L}\p{N}-]+\.(?:org|com|net|to|info|io|cc|me|tv|ru|se|nz|ws)[\p{Z} ]*[-–][\p{Z} ]*`)
	// A folder or a file named only for an episode's number: "E01", "Ep 3", "Episode 12".
	episodeOnlyPattern = regexp.MustCompile(`(?i)^(?:e|ep|episode)[\p{Z} ._-]*([0-9]{1,4})$`)
	// An episode number a name carries without a season: "Show E05 Title", "Show Episode 5".
	loneEpisodePattern = regexp.MustCompile(
		`(?i)(?:^|[^\p{L}\p{N}])(?:e|ep|episode)[\p{Z} ._-]?([0-9]{1,4})(?:[^\p{L}\p{N}]|$)`)
	// "Show - 1x4 - Its Name": a season and an episode of one digit, which in a series library alone
	// is taken as a marker, since a film's name may be "5x2".
	shortSeasonXEpisodePattern = regexp.MustCompile(
		`(?:^|[^\p{L}\p{N}])([0-9]{1,2})x([0-9]{1,3})(?:[^\p{L}\p{N}]+|$)`)
	// A miniseries' part: "Show part 2", "Show - Part 3 - Its Name", "Show BBC 1996 2 of 4".
	partPattern   = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])part[\p{Z} ._-]*([0-9]{1,2})(?:[^\p{L}\p{N}]+|$)`)
	partOfPattern = regexp.MustCompile(
		`(?i)(?:^|[^\p{L}\p{N}])([0-9]{1,2})[\p{Z} ._-]*of[\p{Z} ._-]*[0-9]{1,2}(?:[^\p{L}\p{N}]+|$)`)
	// A number after the series' name, as a word: "Show 02 Its Name", "Show_-_03_v2_[720p]"; a
	// release's version, "v2", may follow it. The function seriesNumberedEpisode checks what comes
	// before.
	numberWordPattern = regexp.MustCompile(
		`(?:^|[\p{Z} ._-])([0-9]{1,3})(?:[\p{Z} ._-]?v[0-9])?(?:[^\p{L}\p{N}]+|$)`)
	// A release's tag in brackets, before or after the name: "[Hoshizora]", "[720p-10bit]".
	bracketTagPattern = regexp.MustCompile(`\[[^\]]*\]`)
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
	if match := bareIMDbPattern.FindStringSubmatch(relative); match != nil && parsed.ProviderIDs["imdb"] == "" {
		parsed.ProviderIDs["imdb"] = match[1]
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
		marker.lastEpisode = lastOfRun(
			stem[match[5]:], marker.season, marker.episode, episodeRunPattern, numberRunPattern)
		return marker, true
	}
	if match := seasonXEpisodePattern.FindStringSubmatchIndex(stem); match != nil {
		marker := episodeMarker{
			season: parseNonNegativeInt(stem[match[2]:match[3]]), episode: parseNonNegativeInt(stem[match[4]:match[5]]),
			startIndex: match[0],
		}
		marker.lastEpisode = lastOfRun(stem[match[5]:], marker.season, marker.episode, numberRunPattern)
		return marker, true
	}
	if match := wordedEpisodePattern.FindStringSubmatchIndex(stem); match != nil {
		return episodeMarker{
			season:     parseNonNegativeInt(stem[match[2]:match[3]]),
			episode:    parseNonNegativeInt(stem[match[4]:match[5]]),
			startIndex: match[0],
		}, true
	}
	return episodeMarker{}, false
}

// lastOfRun is the last episode of a multi-episode file, read from rest, what follows its first
// episode's number, by the run expressions given; 0 for a file of one. A run stays in its
// marker's season and only climbs, so "S01E02-1080p", "S01E05-03" and "S01E05 S02E01" are one
// episode each.
func lastOfRun(rest string, season, episode int, runs ...*regexp.Regexp) int {
	last := 0
	for {
		match := firstOf(rest, runs...)
		if match == nil {
			return last
		}
		number := parseNonNegativeInt(rest[match[4]:match[5]])
		if (match[2] >= 0 && parseNonNegativeInt(rest[match[2]:match[3]]) != season) || number <= max(episode, last) {
			return last
		}
		last, rest = number, rest[match[5]:]
	}
}

// seriesFromFolders names a series from the folders above an episode: "Show/Season 01/x.mkv" and
// "Show/Specials/x.mkv" are Show. A season pack's folder, "Show/Show.S02.COMPLETE.720p/x.mkv", names
// a season too, not a series of its own, so the folder above it names the series; with none above,
// what the pack's name says before its season marker does (the owner's NAS, 2026-10-07: each of a
// series' seasons was a series of its own).
//
// A folder of one episode, "Show/Show.S01E01.1080p/x.mkv", or one named only for its number,
// "Show/Show.S01/E01/x.mkv", and a specials or extras folder are passed over the same way (the
// owner's NAS, 2026-10-13: each of a series' episodes, and another series' "E01" to "E12", were
// series of their own).
//
// A folder holding a season's is the series' own, though its name says specials: "Show.Complete.
// Series.Plus.Christmas.Specials/Series 1/x.mkv" is Show (the owner, 2026-10-28: "If a folder contains
// a bunch of subfolders that look like seasons, it must be the parent folder of a series"; staging's
// series 2405 was named "" and matched nothing). And a name never comes out empty while a specials
// folder above it has one.
func seriesFromFolders(folder string) (name string, year int) {
	packPrefix, outermostBonus := "", ""
	isAboveSeason := false
	for _, part := range slices.Backward(strings.Split(strings.Trim(folder, "/"), "/")) {
		part = strings.TrimSpace(sitePrefixPattern.ReplaceAllString(part, ""))
		if part != "" && (isSeasonFolder(part) || seasonPackPattern.MatchString(part)) {
			isAboveSeason = true
		}
		if part != "" && !isAboveSeason && isBonusFolder(part) {
			outermostBonus = part
			continue
		}
		if part == "" || isSeasonFolder(part) || episodeOnlyPattern.MatchString(part) {
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
	if name, year = cleanTitle(packPrefix); name == "" && outermostBonus != "" {
		return cleanTitle(outermostBonus)
	}
	return name, year
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
// file's, placed by its path: a featurette, or a film made of the series (Show's "The Promised
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
	marker := numberedInStem(stem, folder, parsed.Name)
	parsed.Episode = marker.episode
	before, episodeName := marker.before, marker.after
	if parsed.Name == "" {
		parsed.Name, parsed.Year = cleanTitle(before)
	}
	if parsed.Name == "" {
		parsed.Name, parsed.Year = cleanTitle(stem)
	}
	parsed.EpisodeName, _ = cleanTitle(episodeName)
	if parsed.EpisodeName == "" && !marker.isNamedOnlyIfClean {
		parsed.EpisodeName = strings.TrimSpace(episodeName)
	}
	switch {
	case isBonus || parsed.Episode == 0:
		parsed.Season, parsed.Episode, parsed.IsUnnumbered = 0, 0, true
	case marker.season >= 0:
		parsed.Season = marker.season
	case season >= 0:
		parsed.Season = season
	default:
		parsed.Season = 1
	}
	return parsed
}

// stemNumber is an episode's number as a series library's file name gives it with no S01E02
// marker: the season too where it says one (-1 where not), what comes before the number and what
// after, and whether what follows is the episode's name only if it cleans to one, as release tags
// alone, "[720p-10bit]", name nothing.
type stemNumber struct {
	season, episode    int
	before, after      string
	isNamedOnlyIfClean bool
}

// numberedInStem reads an episode's number from a stem, in order: "1x4", "Show - 01 - Its Name", a
// miniseries' "2 of 4" or "part 2", a number after the series' name, "E05" or "Episode 5", and else
// a folder named for the number. None found, its episode is 0.
func numberedInStem(stem, folder, seriesName string) stemNumber {
	if match := shortSeasonXEpisodePattern.FindStringSubmatchIndex(stem); match != nil {
		return stemNumber{
			season:  parseNonNegativeInt(stem[match[2]:match[3]]),
			episode: parseNonNegativeInt(stem[match[4]:match[5]]),
			before:  stem[:match[0]], after: stem[match[1]:],
		}
	}
	if match := dashNumberPattern.FindStringSubmatchIndex(stem); match != nil {
		return stemNumber{
			season: -1, episode: parseNonNegativeInt(stem[match[2]:match[3]]),
			before: stem[:match[0]], after: stem[match[1]:],
		}
	}
	if match := firstOf(stem, partOfPattern, partPattern); match != nil {
		return stemNumber{
			season: -1, episode: parseNonNegativeInt(stem[match[2]:match[3]]),
			before: stem[:match[0]], after: stem[match[1]:], isNamedOnlyIfClean: true,
		}
	}
	if episode, rest, found := seriesNumberedEpisode(stem, seriesName); found {
		return stemNumber{season: -1, episode: episode, after: rest, isNamedOnlyIfClean: true}
	}
	if match := loneEpisodePattern.FindStringSubmatchIndex(stem); match != nil {
		episode := parseNonNegativeInt(stem[match[2]:match[3]])
		return stemNumber{season: -1, episode: episode, before: stem[:match[0]], after: stem}
	}
	number, _ := episodeFolderNumber(folder)
	return stemNumber{season: -1, episode: number, after: stem}
}

// firstOf is the first match of the first expression that matches text, or nil.
func firstOf(text string, expressions ...*regexp.Regexp) []int {
	for _, expression := range expressions {
		if match := expression.FindStringSubmatchIndex(text); match != nil {
			return match
		}
	}
	return nil
}

// seriesNumberedEpisode reads an episode's number written as a word after the series' name, as in
// "Show 02 Palm of the Hand" or "[Group]_Kaito_Kumo_-_Tsuki_no_Onna_-_03_v2", and what
// follows it. What comes before the number must end with the series' name, as textindex.SameWork
// folds it, so a number inside an episode's own name is not taken for one, and a date's numbers
// are none ("Show 14.03.2019"). The separator after one number may be the one before the next,
// as in "Station 9 05", so the numbers are read by everyMatch.
func seriesNumberedEpisode(stem, seriesName string) (episode int, rest string, found bool) {
	series := textindex.SameWork(seriesName)
	if series == "" {
		return 0, "", false
	}
	text := strings.TrimSpace(strings.ReplaceAll(bracketTagPattern.ReplaceAllString(stem, " "), "_", " "))
	text = datePattern.ReplaceAllString(text, " ")
	for _, match := range everyMatch(numberWordPattern, text) {
		if strings.HasSuffix(textindex.SameWork(text[:match[0]]), series) {
			return parseNonNegativeInt(text[match[2]:match[3]]), text[match[1]:], true
		}
	}
	return 0, "", false
}

// outermostSeriesFolder names the series of a file no marker places by the folder just under the
// library's root, where a series library keeps each series: a featurette or a film inside a series'
// folders is that series', not its own folder's ("Show/The Promised Land 2020/x.mkv" is Show's).
// When that folder is itself an episode's or a season pack's, the folders say as for an
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

// wordsBeforeTags are a name's words before its first release tag, one joined to a word by a dash
// too: "Movie Name-AC3-5.1-DVDRip" is Movie Name.
func wordsBeforeTags(words []string) []string {
	for index, word := range words {
		if parts := strings.Split(word, "-"); len(parts) > 1 && !isReleaseTag(word) {
			for part := 1; part < len(parts); part++ {
				if isReleaseTag(parts[part]) {
					return append(words[:index:index], strings.Join(parts[:part], "-"))
				}
			}
		}
		// "Complete" ends a pack's name, "Show.COMPLETE.SERIES", but may begin a title; so does a cut's
		// name, "Name Director's Cut", but not "The Director's Wife".
		if isReleaseTag(word) || (index > 0 && strings.EqualFold(word, "complete")) ||
			(index > 0 && index+1 < len(words) && isCutName(word, words[index+1])) {
			return words[:index]
		}
	}
	return words
}

// withoutOpenBracket is text cut where a bracket opens that does not close: what is left of a group
// whose year cut it, as in "Movie Name (Western Comedy" or "Show Name [ATG".
func withoutOpenBracket(text string) string {
	open := -1
	depth := 0
	for index, r := range text {
		switch r {
		case '(', '[', '（', '［':
			if depth == 0 {
				open = index
			}
			depth++
		case ')', ']', '）', '］':
			if depth > 0 {
				depth--
			}
		}
	}
	if depth > 0 && open > 0 {
		return text[:open]
	}
	return text
}

// isCutName is whether two words name a film's cut: "Director's Cut", "Theatrical Edition".
func isCutName(word, next string) bool {
	switch strings.ToLower(word) {
	case "director's", "director’s", "theatrical":
		return strings.EqualFold(next, "cut") || strings.EqualFold(next, "edition")
	}
	return false
}

// cleanTitle turns a release-style stem into a title and its year: separators become spaces,
// provider-id markers and bracketed groups go, and everything from the year or the first release
// tag on is cut.
func cleanTitle(stem string) (name string, year int) {
	// An id goes without a trace, so "Show.2016.[tt7654321].1080p" stays a dotted name.
	text := bareIMDbPattern.ReplaceAllString(providerIDPattern.ReplaceAllString(stem, ""), "")
	text = leadingGroupPattern.ReplaceAllString(text, "")
	if !strings.ContainsFunc(text, unicode.IsSpace) {
		text = strings.NewReplacer(".", " ", "_", " ").Replace(text)
	}
	if matches := everyMatch(yearPattern, text); len(matches) > 0 {
		// The last year-looking number is the year; an earlier one is part of the name, as in
		// "7th Heaven (1968)" or "Station 2061 (2022)".
		last := matches[len(matches)-1]
		// A number that is the whole name is the name: "1917".
		if last[0] > 0 {
			year = parseNonNegativeInt(text[last[2]:last[3]])
			text = text[:last[0]]
		}
	}
	// What is left of a bracket the year was inside goes, "Name (Comedy" and "Name [ATG", and so does
	// a bracketed tag after the name, "Name [BDRip-1080p][RiP By X]".
	text = bracketTagPattern.ReplaceAllString(withoutOpenBracket(text), " ")
	name = strings.Join(wordsBeforeTags(strings.Fields(text)), " ")
	name = strings.TrimFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == '(' || r == '[' || r == '.' || r == ','
	})
	return name, year
}

// everyMatch finds every match of an expression whose first group is a number between separators,
// as a year or an episode's number is: one search after another, each from the end of the last
// number's digits, since the separator after one number is the one before the next ("1917 2019", a
// film named for a year), which FindAll would have consumed.
func everyMatch(expression *regexp.Regexp, text string) [][]int {
	var found [][]int
	for from := 0; from < len(text); {
		match := expression.FindStringSubmatchIndex(text[from:])
		if match == nil {
			break
		}
		for index := range match {
			match[index] += from
		}
		found = append(found, match)
		from = match[3]
	}
	return found
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

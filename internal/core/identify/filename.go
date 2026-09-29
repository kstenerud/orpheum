// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"path"
	"regexp"
	"slices"
	"strconv"
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
	yearPattern = regexp.MustCompile(`(?:^|[\p{Z} ._(\[-])((?:19|20)[0-9]{2})(?:[\p{Z} ._)\]-]|$)`)
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
	stem := strings.TrimSuffix(file, path.Ext(file))
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
			season: parseIntOrZero(stem[match[2]:match[3]]), episode: parseIntOrZero(stem[match[4]:match[5]]),
			startIndex: match[0],
		}
		if match[6] >= 0 {
			marker.lastEpisode = parseIntOrZero(stem[match[6]:match[7]])
		}
		return marker, true
	}
	if match := seasonXEpisodePattern.FindStringSubmatchIndex(stem); match != nil {
		season, episode := parseIntOrZero(stem[match[2]:match[3]]), parseIntOrZero(stem[match[4]:match[5]])
		return episodeMarker{season: season, episode: episode, startIndex: match[0]}, true
	}
	return episodeMarker{}, false
}

// seriesFromFolders names a series from the folders above an episode: "Show/Season 01/x.mkv"
// is Show.
func seriesFromFolders(folder string) (name string, year int) {
	for _, part := range slices.Backward(strings.Split(strings.Trim(folder, "/"), "/")) {
		if part == "" || seasonFolderPattern.MatchString(strings.TrimSpace(part)) {
			continue
		}
		return cleanTitle(part)
	}
	return "", 0
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
			year = parseIntOrZero(text[last[2]:last[3]])
			text = text[:last[0]]
		}
	}
	words := strings.Fields(text)
	for index, word := range words {
		if isReleaseTag(word) {
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

func parseIntOrZero(digits string) int {
	value, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return value
}

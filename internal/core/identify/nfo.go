// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// NFO is what a Kodi-family .nfo file says (identification.md, Reading the sidecars). Its
// locked fields are a person's corrections and become overrides; the rest is the title's source
// where the library was told to use NFO files (D182, D183). Watch counts in it belong to nobody
// and are not read.
type NFO struct {
	// Kind is the root element: "movie", "episodedetails", "tvshow" or "season".
	Kind          string
	Title         string
	OriginalTitle string
	SortTitle     string
	Plot          string
	Tagline       string
	Year          int
	RuntimeMs     int64
	Season        int
	Episode       int
	// ProviderIDs are the ids it names, by provider: "tmdb", "imdb", "tvdb".
	ProviderIDs map[string]string
	// Locked are the fields a person locked, in our field names ("name", "overview", ...);
	// lockdata locks every field the file sets.
	Locked []string
}

// ErrNotNFO is a body that is not an NFO document.
var ErrNotNFO = errors.New("not an NFO document")

// nfoDocument is the XML the reader accepts: Kodi's, Emby's and Jellyfin's dialects share these
// elements.
type nfoDocument struct {
	XMLName       xml.Name
	Title         string        `xml:"title"`
	OriginalTitle string        `xml:"originaltitle"`
	SortTitle     string        `xml:"sorttitle"`
	Plot          string        `xml:"plot"`
	Outline       string        `xml:"outline"`
	Tagline       string        `xml:"tagline"`
	Year          string        `xml:"year"`
	Premiered     string        `xml:"premiered"`
	Aired         string        `xml:"aired"`
	Runtime       string        `xml:"runtime"`
	Season        string        `xml:"season"`
	Episode       string        `xml:"episode"`
	UniqueIDs     []nfoUniqueID `xml:"uniqueid"`
	TMDBID        string        `xml:"tmdbid"`
	IMDBID        string        `xml:"imdbid"`
	TVDBID        string        `xml:"tvdbid"`
	ID            string        `xml:"id"`
	LockData      string        `xml:"lockdata"`
	LockedFields  string        `xml:"lockedfields"`
}

type nfoUniqueID struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// lockedFieldNames maps the lockedfields vocabulary (Jellyfin's MetadataField) to ours.
func lockedFieldNames() map[string]string {
	return map[string]string{"name": fieldName, "overview": fieldOverview, "runtime": fieldRuntimeMs}
}

// ParseNFO reads an NFO. Kodi lets a file be XML followed by a scraper URL, so anything after
// the root element is ignored.
func ParseNFO(body []byte) (NFO, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.Strict = false
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	var document nfoDocument
	if err := decoder.Decode(&document); err != nil { //nolint:musttag // XMLName takes the root element, untagged
		return NFO{}, fmt.Errorf("%w: %w", ErrNotNFO, err)
	}
	kind := strings.ToLower(document.XMLName.Local)
	switch kind {
	case kindMovie, "episodedetails", "tvshow", "season":
	default:
		return NFO{}, fmt.Errorf("%w: the root element is <%s>", ErrNotNFO, document.XMLName.Local)
	}
	nfo := NFO{
		Kind: kind, Title: clean(document.Title), OriginalTitle: clean(document.OriginalTitle),
		SortTitle: clean(document.SortTitle), Plot: textindex.Prose(firstOf(document.Plot, document.Outline)),
		Tagline: clean(document.Tagline), Year: yearOf(document), RuntimeMs: runtimeOf(document.Runtime),
		Season: number(document.Season), Episode: number(document.Episode),
		ProviderIDs: providerIDsOf(document),
	}
	nfo.Locked = lockedOf(document, nfo)
	return nfo, nil
}

func yearOf(document nfoDocument) int {
	if year := number(document.Year); year > 0 {
		return year
	}
	for _, date := range []string{document.Premiered, document.Aired} {
		if year := number(strings.SplitN(strings.TrimSpace(date), "-", 2)[0]); year > 0 {
			return year
		}
	}
	return 0
}

// runtimeOf reads Kodi's runtime, in minutes.
func runtimeOf(text string) int64 {
	minutes := number(strings.TrimSuffix(strings.TrimSpace(text), " min"))
	return int64(minutes) * 60_000
}

func providerIDsOf(document nfoDocument) map[string]string {
	ids := map[string]string{}
	for _, unique := range document.UniqueIDs {
		if provider := strings.ToLower(strings.TrimSpace(unique.Type)); provider != "" && clean(unique.Value) != "" {
			ids[provider] = clean(unique.Value)
		}
	}
	for provider, value := range map[string]string{
		sourceTMDB: document.TMDBID, sourceIMDb: document.IMDBID, sourceTVDB: document.TVDBID,
	} {
		if _, found := ids[provider]; !found && clean(value) != "" {
			ids[provider] = clean(value)
		}
	}
	// Kodi's bare <id> is IMDb's when it looks like one.
	if id := clean(document.ID); strings.HasPrefix(id, "tt") && ids[sourceIMDb] == "" {
		ids[sourceIMDb] = id
	}
	return ids
}

// lockedOf lists the fields a person locked: every field the file sets when lockdata is true,
// else those lockedfields names.
func lockedOf(document nfoDocument, nfo NFO) []string {
	if strings.EqualFold(strings.TrimSpace(document.LockData), "true") {
		var locked []string
		for field, set := range map[string]bool{
			fieldName: nfo.Title != "", fieldOverview: nfo.Plot != "", fieldRuntimeMs: nfo.RuntimeMs > 0,
			fieldYear: nfo.Year > 0, fieldTagline: nfo.Tagline != "",
		} {
			if set {
				locked = append(locked, field)
			}
		}
		return sortedStrings(locked)
	}
	var locked []string
	names := lockedFieldNames()
	for part := range strings.SplitSeq(document.LockedFields, "|") {
		if field, known := names[strings.ToLower(strings.TrimSpace(part))]; known {
			locked = append(locked, field)
		}
	}
	return sortedStrings(locked)
}

// clean is an NFO field as it enters the system (principles §22): an NFO is anyone's text.
func clean(text string) string { return textindex.Line(text) }

func firstOf(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func number(text string) int {
	value, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func sortedStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	slices.Sort(values)
	return values
}

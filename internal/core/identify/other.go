// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Where an other video's recorded date came from (other_video.recorded_from; D349 §4).
const (
	recordedFromTag  = "tag"  // its container's recorded-time tag
	recordedFromName = "name" // a date in its file's or a folder's name
	recordedFromFile = "file" // the file's own modified time
	recordedFromUser = "user" // a user's correction, the override recorded_at
)

var (
	// A date, year first, as cameras and people name videos: "2019-07-14", "2019_07_14 Beach",
	// "VID_20190714_103102", "PXL_20230102_123456789", with its time where one follows.
	recordedDatePattern = regexp.MustCompile(`(?:^|[^\p{N}])((?:19|20)[0-9]{2})[-_. ]?(0[1-9]|1[0-2])[-_. ]?` +
		`(0[1-9]|[12][0-9]|3[01])` +
		`(?:[-_. Tt]?([01][0-9]|2[0-3])[-_.:h]?([0-5][0-9])[-_.:m]?([0-5][0-9]))?(?:[^\p{N}]|$)`)
	// A month, year first: "2019-07", "2019_07 Holiday".
	recordedMonthPattern = regexp.MustCompile(`(?:^|[^\p{N}])((?:19|20)[0-9]{2})[-_. ](0[1-9]|1[0-2])(?:[^\p{N}]|$)`)
)

// otherFacts are what an other video's file says beyond its name: when it was recorded and where
// that came from, and the file's name.
type otherFacts struct {
	recordedAtMs int64
	recordedFrom string
	fileName     string
}

// datedSpan is a date a name holds, and where in the name it stands.
type datedSpan struct {
	date       time.Time
	start, end int
}

// nameDate is the date a name holds, with where in the name it stands; false when it holds none.
// A full date wins over a month, whose day is its first.
func nameDate(name string) (datedSpan, bool) {
	if match := recordedDatePattern.FindStringSubmatchIndex(name); match != nil {
		parts := make([]int, 0, 6)
		for group := 1; group <= 6; group++ {
			number := 0
			if match[2*group] >= 0 {
				number, _ = strconv.Atoi(name[match[2*group]:match[2*group+1]]) //nolint:errcheck // digits only
			}
			parts = append(parts, number)
		}
		date := time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], 0, time.UTC)
		if date.Day() != parts[2] {
			return datedSpan{}, false // 2019-02-31 is no day, nor a month and a stray number
		}
		return datedSpan{date: date, start: match[2], end: lastGroupEnd(match)}, true
	}
	if match := recordedMonthPattern.FindStringSubmatchIndex(name); match != nil {
		year, _ := strconv.Atoi(name[match[2]:match[3]])  //nolint:errcheck // digits only
		month, _ := strconv.Atoi(name[match[4]:match[5]]) //nolint:errcheck // digits only
		date := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		return datedSpan{date: date, start: match[2], end: match[5]}, true
	}
	return datedSpan{}, false
}

// lastGroupEnd is where the last group a match found ends.
func lastGroupEnd(match []int) int {
	end := 0
	for group := 1; 2*group+1 < len(match); group++ {
		end = max(end, match[2*group+1])
	}
	return end
}

// otherName is the name an other video's file gives it, and the date in the name, when it holds one
// (D349 §3, §4): the file's name without its extension, a date in it left out and its underscores and
// dots read as spaces. A name that is only a camera's mark and a date ("VID_20190714_103102") stays
// whole, as it says nothing else.
func otherName(stem string) (name string, date time.Time, hasDate bool) {
	whole := tidyOtherName(stem)
	dated, hasDate := nameDate(stem)
	if !hasDate {
		return whole, time.Time{}, false
	}
	rest := tidyOtherName(stem[:dated.start] + " " + stem[dated.end:])
	if isOnlyAMark(rest) {
		return whole, dated.date, true
	}
	return rest, dated.date, true
}

// tidyOtherName reads underscores and dots as spaces, runs of space as one, and drops the dashes
// and spaces at either end.
func tidyOtherName(text string) string {
	text = strings.Join(strings.Fields(strings.NewReplacer("_", " ", ".", " ").Replace(text)), " ")
	return strings.Trim(text, " -–—")
}

// isOnlyAMark is whether what is left of a name is no name: nothing, or a camera's mark in capitals
// ("VID", "IMG", "PXL", "MVI 0042").
func isOnlyAMark(text string) bool {
	letters := 0
	for _, character := range text {
		if unicode.IsLower(character) {
			return false
		}
		if unicode.IsLetter(character) {
			letters++
		}
	}
	return letters <= 5
}

// isOtherLibrary is whether the library holds videos that are neither films nor series (D349 §3).
func (filer *libraryFiler) isOtherLibrary() bool {
	return filer.library.Kind == library.LibraryKindOther
}

// identifyOther files every video of an other library as a title of its own, named by its file or
// the NFO file beside it and never matched; an image beside it is its picture (D349 §3).
func (filer *libraryFiler) identifyOther(ctx context.Context) error {
	for _, file := range filer.files {
		if !isVideo(file) {
			continue
		}
		relativePath, ok := filer.relativePath(file)
		if !ok {
			continue
		}
		if err := filer.fileOther(ctx, file, relativePath); err != nil {
			return err
		}
	}
	filer.mapLocalArtwork()
	if err := filer.resolveAndFile(ctx); err != nil {
		return err
	}
	filer.reportLocalImages()
	return filer.retireEmptied(ctx)
}

// fileOther files one video of an other library: in the title it is already in, which a rename or a
// move does not change, as its name never identified it; else in a title of its own.
func (filer *libraryFiler) fileOther(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, relativePath string,
) error {
	spec := filer.otherSpec(file, relativePath)
	var titleID int64
	if file.VersionID != nil {
		current, err := filer.versionTitle(ctx, *file.VersionID)
		if err != nil {
			return fmt.Errorf("read %s's version: %w", file.Path, err)
		}
		titleID = current.WorkID
	} else {
		var err error
		if titleID, err = filer.findOrAddTitle(ctx, spec); err != nil {
			return err
		}
	}
	// A folder holds many videos: an image named for the folder is none of theirs.
	return filer.place(ctx, file, titleID, spec)
}

// otherSpec is what an other video's file, its probe and the NFO file beside it say: its name, its
// date, its file's name. Identified from these alone, and no provider's id is kept, so nothing finds
// it at a provider or in another library by one (D349 §10).
func (filer *libraryFiler) otherSpec(file sqlcgen.ListIdentifyFilesRow, relativePath string) titleSpec {
	fileName := path.Base(relativePath)
	name, _, _ := otherName(strings.TrimSuffix(fileName, path.Ext(fileName)))
	recordedAtMs, recordedFrom := filer.recordedOf(file, relativePath)
	spec := titleSpec{
		kind: library.TitleKindOther, key: library.TitleKindOther + "|" + file.Path, name: name,
		runtimeMs: file.DurationMs, source: sourceNone, identification: identificationIdentified,
		other: otherFacts{recordedAtMs: recordedAtMs, recordedFrom: recordedFrom, fileName: fileName},
	}
	if nfo, found := filer.nfoFor(file); found && nfo.Kind == KindMovie {
		spec.fromNFO(nfo)
		spec.ids = nil
	}
	return spec
}

// recordedOf is when a video was recorded and where that came from: its container's tag, else a
// date in its file's name or its nearest folder's that holds one, else the file's own time (D349 §4).
func (filer *libraryFiler) recordedOf(file sqlcgen.ListIdentifyFilesRow, relativePath string) (int64, string) {
	if at := filer.recordedByFile[file.ID]; at > 0 {
		return at, recordedFromTag
	}
	for part := relativePath; part != "." && part != "/" && part != ""; part = path.Dir(part) {
		name := path.Base(part)
		if part == relativePath {
			name = strings.TrimSuffix(name, path.Ext(name))
		}
		if dated, found := nameDate(name); found {
			return dated.date.UnixMilli(), recordedFromName
		}
	}
	return ptr.ValueOrZero(file.ModifiedAt), recordedFromFile
}

// loadRecordedTimes reads when each of an other library's files says it was recorded.
func (filer *libraryFiler) loadRecordedTimes(ctx context.Context, libraryID int64) error {
	filer.recordedByFile = map[int64]int64{}
	if !filer.isOtherLibrary() {
		return nil
	}
	times, err := filer.queries.ListLibraryRecordedTimes(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("read when its files were recorded: %w", err)
	}
	for _, recorded := range times {
		filer.recordedByFile[recorded.FileID] = recorded.RecordedAtMs
	}
	return nil
}

// resolveOtherVideo writes an other video's date, a person's correction over its file's, and its
// file's name with that name's sort key in the pass's locale, when they changed.
func (filer *libraryFiler) resolveOtherVideo(
	ctx context.Context, titleID int64, facts otherFacts, corrected string,
) error {
	values := sqlcgen.UpsertOtherVideoParams{
		WorkID: titleID, RecordedAt: facts.recordedAtMs, RecordedFrom: facts.recordedFrom,
		FileName: facts.fileName, FileNameKey: filer.sorter.KeyExact(facts.fileName),
	}
	if at, err := strconv.ParseInt(corrected, 10, 64); err == nil && at > 0 {
		values.RecordedAt, values.RecordedFrom = at, recordedFromUser
	}
	if held, isHeld := filer.snap.otherVideos[titleID]; isHeld && isSameOtherVideo(held, values) {
		return nil
	}
	if err := filer.queries.UpsertOtherVideo(ctx, values); err != nil {
		return fmt.Errorf("date other video %d: %w", titleID, err)
	}
	if filer.snap.isLoaded {
		filer.snap.otherVideos[titleID] = values
	}
	return nil
}

// isSameOtherVideo is whether two of an other video's rows hold the same values.
func isSameOtherVideo(first, second sqlcgen.UpsertOtherVideoParams) bool {
	return first.WorkID == second.WorkID && first.RecordedAt == second.RecordedAt &&
		first.RecordedFrom == second.RecordedFrom && first.FileName == second.FileName &&
		string(first.FileNameKey) == string(second.FileNameKey)
}

// loadOtherVideos reads what the pass last derived of the library's other videos.
func (snap *snapshot) loadOtherVideos(ctx context.Context, queries *sqlcgen.Queries, libraryID int64) error {
	snap.otherVideos = map[int64]sqlcgen.UpsertOtherVideoParams{}
	rows, err := queries.ListLibraryOtherVideos(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its other videos: %w", err)
	}
	for _, row := range rows {
		snap.otherVideos[row.WorkID] = sqlcgen.UpsertOtherVideoParams(row)
	}
	return nil
}

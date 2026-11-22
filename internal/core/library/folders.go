// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// FolderQuery asks for one folder of a library of any kind (D539 §7): its path below the library's
// roots, "" for their top, the order its videos come in, and the page of it: its folders then its
// videos, from Offset, at most Limit of them, every one when Limit is 0 (D432 §3).
type FolderQuery struct {
	LibraryID int64
	Path      string
	Sort      string // SortRecorded, SortName, SortFileName or SortAdded
	Order     string // OrderAscending, OrderDescending, or OrderDefault for the sort's own (D366 §2)
	Reach     auth.Reach
	UserID    int64
	Offset    int64
	Limit     int64
}

// Folder is one folder of a library, or the page of it asked for: the folders in it, and its own
// videos in the order asked for. An other library's videos are its titles, Entries, each one's file
// in Videos at the same index (D349 §5); any other library's are its video files, Videos alone, with
// no Entries (D539 §7).
type Folder struct {
	Path       string
	Subfolders []Subfolder
	Entries    []Entry
	Videos     []FolderVideo
}

// Subfolder is a folder in a folder: its name, its path below the roots, and how many videos it
// holds, in the folders inside it too.
type Subfolder struct {
	Name, Path string
	VideoCount int64
}

// FolderVideo is a video file directly in a library's folder (D539 §7): its file, its name, the live
// title it is filed as, nil for none, its length where its probe knows it, 0 otherwise, and where
// the person's playback of it starts (ResumeAtMs), 0 at its start.
type FolderVideo struct {
	FileID     int64
	Name       string
	Title      *FolderVideoTitle
	DurationMs int64
	ResumeAtMs int64
}

// FolderVideoTitle is the title a folder's video is filed as: its id, its kind (TitleKindMovie,
// TitleKindEpisode, TitleKindExtra or TitleKindOther), its name, its year where it has one, and its
// identification state, unidentified for a film whose match was not found.
type FolderVideoTitle struct {
	ID                  int64
	Kind, Name          string
	Year                *int64
	IdentificationState string
}

// placedVideo is an other video and where its file is below the library's roots.
type placedVideo struct {
	row      sqlcgen.ListOtherVideosOfLibraryRow
	relative string
}

// placedFile is a library's video file and where it is below the library's roots.
type placedFile struct {
	row      sqlcgen.ListVideoFilesOfLibraryRow
	relative string
}

// Folder is one folder of a library the caller reaches, as the scanner saw its files: a library with
// several roots shows their folders together (D163; D349 §5). An other library's folders count and
// list its titles; any other library's count and list its video files, matched or not (D539 §7).
func (browser *Browser) Folder(ctx context.Context, query FolderQuery) (Folder, error) {
	if !query.Reach.Reaches(&query.LibraryID) {
		return Folder{}, fmt.Errorf("library %d: %w", query.LibraryID, ErrNoLibrary)
	}
	kind, roots, err := browser.libraryRoots(ctx, query.LibraryID)
	if err != nil {
		return Folder{}, err
	}
	files, err := browser.placedFiles(ctx, query.LibraryID, query.UserID, roots)
	if err != nil {
		return Folder{}, err
	}
	videos, err := browser.folderVideos(ctx, files)
	if err != nil {
		return Folder{}, err
	}
	if kind == LibraryKindOther {
		return browser.otherFolder(ctx, query, roots, videos)
	}
	folder := Folder{Path: cleanFolder(query.Path), Entries: []Entry{}}
	var here []placedFile
	folder.Subfolders, here = splitFolder(folder.Path, files, func(file placedFile) string { return file.relative })
	sort := filesSort(query.Sort)
	if sortErr := browser.sortFiles(ctx, here, sort); sortErr != nil {
		return Folder{}, sortErr
	}
	if (ListQuery{Sort: sort, Order: query.Order}).isReversed() {
		slices.Reverse(here) // the other way from the sort's own (D366 §2)
	}
	folder.Subfolders, here = folderPage(folder.Subfolders, here, query.Offset, query.Limit)
	folder.Videos = make([]FolderVideo, 0, len(here))
	for _, file := range here {
		folder.Videos = append(folder.Videos, videos[file.row.FileID])
	}
	return folder, nil
}

// otherFolder is the folder of an other library query asks for: the folders in it, counting its
// titles, and its own titles in the order asked for, each with its file from videos, the library's
// by file (D349 §5).
func (browser *Browser) otherFolder(
	ctx context.Context, query FolderQuery, roots []sqlcgen.ListLibraryRootsRow, videos map[int64]FolderVideo,
) (Folder, error) {
	placed, err := browser.placedVideos(ctx, query.LibraryID, roots)
	if err != nil {
		return Folder{}, err
	}
	folder := Folder{Path: cleanFolder(query.Path)}
	var here []placedVideo
	folder.Subfolders, here = splitFolder(folder.Path, placed, func(video placedVideo) string { return video.relative })
	if sortErr := browser.sortPlaced(ctx, here, query.Sort); sortErr != nil {
		return Folder{}, sortErr
	}
	if (ListQuery{Sort: query.Sort, Order: query.Order}).isReversed() {
		// The other way from the sort's own: Z to A, or oldest first (D366 §2).
		slices.Reverse(here)
	}
	folder.Subfolders, here = folderPage(folder.Subfolders, here, query.Offset, query.Limit)
	page := Page{Entries: make([]Entry, 0, len(here))}
	folder.Videos = make([]FolderVideo, 0, len(here))
	for _, video := range here {
		row := video.row
		facts := &OtherFacts{RecordedAtMs: row.RecordedAt, RecordedFrom: row.RecordedFrom, FileName: row.FileName}
		page.Entries = append(page.Entries, otherEntry(row.WorkID, &row.Name, ptr.ValueOrZero(row.IdentificationState),
			row.AddedAt, facts))
		file, isRead := videos[row.FileID]
		if !isRead { // filed, but its place not read: the title's own
			file = FolderVideo{FileID: row.FileID, Name: path.Base(video.relative), Title: &FolderVideoTitle{
				ID: row.WorkID, Kind: TitleKindOther, Name: row.Name,
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
			}}
		}
		folder.Videos = append(folder.Videos, file)
	}
	if page, err = browser.finishPage(ctx, query.Reach, page); err != nil {
		return Folder{}, err
	}
	if page, err = browser.withProgress(ctx, query.Reach, query.UserID, page); err != nil {
		return Folder{}, err
	}
	folder.Entries = page.Entries
	return folder, nil
}

// splitFolder is the folder at folderPath's share of videos, each below the roots at relativeOf it:
// the folders in it, by name, each with how many of the videos it holds, in the folders inside it
// too, and its own videos, in their order.
func splitFolder[Video any](
	folderPath string, videos []Video, relativeOf func(Video) string,
) ([]Subfolder, []Video) {
	counts := map[string]int64{}
	var here []Video
	for _, video := range videos {
		directory := cleanFolder(path.Dir(relativeOf(video)))
		if directory == folderPath {
			here = append(here, video)
			continue
		}
		if child, isInside := childOf(folderPath, directory); isInside {
			counts[child]++
		}
	}
	subfolders := []Subfolder{}
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		subfolders = append(subfolders,
			Subfolder{Name: name, Path: path.Join(folderPath, name), VideoCount: counts[name]})
	}
	return subfolders, here
}

// folderPage is the part of a folder a page holds: its folders first, then its videos, from offset,
// at most limit of them, every one from offset when limit is 0.
func folderPage[Video any](subfolders []Subfolder, videos []Video, offset, limit int64) ([]Subfolder, []Video) {
	split, end := int64(len(subfolders)), int64(len(subfolders)+len(videos))
	if limit > 0 {
		end = min(end, offset+limit)
	}
	start := min(max(offset, 0), end)
	return subfolders[min(start, split):min(end, split)], videos[max(start, split)-split : max(end, split)-split]
}

// libraryRoots are a library's kind and its folders.
func (browser *Browser) libraryRoots(
	ctx context.Context, libraryID int64,
) (string, []sqlcgen.ListLibraryRootsRow, error) {
	queries := browser.database.Read()
	held, err := store.OrNotFound(queries.GetLibraryForIdentify(ctx, libraryID))
	if errors.Is(err, store.ErrNotFound) {
		return "", nil, fmt.Errorf("library %d: %w", libraryID, ErrNoLibrary)
	}
	if err != nil {
		return "", nil, fmt.Errorf("read library %d: %w", libraryID, err)
	}
	roots, err := queries.ListLibraryRoots(ctx, libraryID)
	if err != nil {
		return "", nil, fmt.Errorf("list library %d's roots: %w", libraryID, err)
	}
	return held.Kind, roots, nil
}

// placedVideos are an other library's live videos, each with its file's path below its root.
func (browser *Browser) placedVideos(
	ctx context.Context, libraryID int64, roots []sqlcgen.ListLibraryRootsRow,
) ([]placedVideo, error) {
	rows, err := browser.database.Read().ListOtherVideosOfLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list library %d's videos: %w", libraryID, err)
	}
	placed := make([]placedVideo, 0, len(rows))
	for _, row := range rows {
		placed = append(placed, placedVideo{row: row, relative: belowRoots(row.Path, roots)})
	}
	return placed, nil
}

// placedFiles are a library's video files, each with its path below its root and userID's position
// in it.
func (browser *Browser) placedFiles(
	ctx context.Context, libraryID, userID int64, roots []sqlcgen.ListLibraryRootsRow,
) ([]placedFile, error) {
	rows, err := browser.database.Read().ListVideoFilesOfLibrary(ctx,
		sqlcgen.ListVideoFilesOfLibraryParams{LibraryID: libraryID, UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("list library %d's video files: %w", libraryID, err)
	}
	placed := make([]placedFile, 0, len(rows))
	for _, row := range rows {
		placed = append(placed, placedFile{row: row, relative: belowRoots(row.Path, roots)})
	}
	return placed, nil
}

// folderVideos are files as a folder lists them, by file: each with its title, its length and where
// its person's playback of it resumes.
func (browser *Browser) folderVideos(ctx context.Context, files []placedFile) (map[int64]FolderVideo, error) {
	videos := make(map[int64]FolderVideo, len(files))
	var thresholds *ResumeThresholds
	for _, file := range files {
		row := file.row
		video := FolderVideo{FileID: row.FileID, Name: path.Base(file.relative), DurationMs: row.DurationMs}
		if row.ItemID != 0 {
			video.Title = &FolderVideoTitle{
				ID: row.ItemID, Kind: row.ItemKind, Name: row.ItemName, Year: row.Year,
				IdentificationState: row.IdentificationState,
			}
		}
		if row.PositionMs > 0 {
			if thresholds == nil {
				read, err := ReadResumeThresholds(ctx, browser.settings)
				if err != nil {
					return nil, err
				}
				thresholds = &read
			}
			video.ResumeAtMs = thresholds.ResumeAtMs(row.PositionMs, row.PositionReadAt, row.MarkedAt, row.DurationMs)
		}
		videos[row.FileID] = video
	}
	return videos, nil
}

// belowRoots is a file's path below the longest of the roots that holds it, or its name alone when
// none does.
func belowRoots(filePath string, roots []sqlcgen.ListLibraryRootsRow) string {
	relative := path.Base(filePath)
	longest := -1
	for _, root := range roots {
		prefix := strings.TrimSuffix(PathText([]byte(root.RootPath)), "/") + "/"
		if strings.HasPrefix(filePath, prefix) && len(prefix) > longest {
			relative, longest = strings.TrimPrefix(filePath, prefix), len(prefix)
		}
	}
	return relative
}

// cleanFolder is a folder's path as the folders list it: no leading, trailing or doubled slash, and
// "" for the top.
func cleanFolder(folder string) string {
	cleaned := strings.Trim(path.Clean("/"+folder), "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}

// childOf is the folder directly in parent on the way to directory, when directory is inside it.
func childOf(parent, directory string) (string, bool) {
	rest := directory
	if parent != "" {
		var isInside bool
		if rest, isInside = strings.CutPrefix(directory, parent+"/"); !isInside {
			return "", false
		}
	}
	if rest == "" {
		return "", false
	}
	child, _, _ := strings.Cut(rest, "/")
	return child, true
}

// sortPlaced orders a folder's videos as asked: newest recorded first, by name or file name in the
// sortLocale, or newest added first.
func (browser *Browser) sortPlaced(ctx context.Context, videos []placedVideo, sort string) error {
	var sorter *textindex.Sorter
	if sort == SortName {
		var err error
		if sorter, err = browser.sorter(ctx); err != nil {
			return err
		}
	}
	slices.SortStableFunc(videos, func(first, second placedVideo) int {
		one, two := first.row, second.row
		switch sort {
		case SortName:
			return cmp.Or(bytes.Compare(sorter.Key(one.Name), sorter.Key(two.Name)),
				cmp.Compare(one.WorkID, two.WorkID))
		case SortFileName:
			return cmp.Or(bytes.Compare(one.FileNameKey, two.FileNameKey), cmp.Compare(one.WorkID, two.WorkID))
		case SortAdded:
			return cmp.Or(cmp.Compare(two.FoundAt, one.FoundAt), cmp.Compare(two.FoundFileModifiedAt,
				one.FoundFileModifiedAt), cmp.Compare(two.WorkID, one.WorkID))
		}
		return cmp.Or(cmp.Compare(two.RecordedAt, one.RecordedAt),
			cmp.Compare(two.WorkID, one.WorkID))
	})
	return nil
}

// filesSort is the sort a folder of video files is in for sort: by file name for SortRecorded, which a
// file of a film or a series library has no date of, else sort itself.
func filesSort(sort string) string {
	if sort == SortRecorded {
		return SortFileName
	}
	return sort
}

// sortFiles orders a folder's video files as asked, by filesSort's sort: by their titles' names in the
// sortLocale, a file filed as no title by its own name; by file name; or newest found first.
func (browser *Browser) sortFiles(ctx context.Context, files []placedFile, sort string) error {
	sorter, err := browser.sorter(ctx)
	if err != nil {
		return err
	}
	keys := make(map[int64][]byte, len(files))
	for _, file := range files {
		name := path.Base(file.relative)
		if sort == SortName && file.row.ItemName != "" {
			keys[file.row.FileID] = sorter.Key(file.row.ItemName)
		} else {
			keys[file.row.FileID] = sorter.KeyExact(name)
		}
	}
	slices.SortStableFunc(files, func(first, second placedFile) int {
		one, two := first.row, second.row
		if sort == SortAdded {
			return cmp.Or(cmp.Compare(two.FirstSeenAt, one.FirstSeenAt), cmp.Compare(two.FileID, one.FileID))
		}
		return cmp.Or(bytes.Compare(keys[one.FileID], keys[two.FileID]), cmp.Compare(one.FileID, two.FileID))
	})
	return nil
}

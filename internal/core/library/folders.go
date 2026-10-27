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
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNoFolders is a library whose folders are not browsed: any but an other library (D349 §5).
var ErrNoFolders = errors.New("only an other library's folders are browsed")

// FolderQuery asks for one folder of an other library: its path below the library's roots, "" for
// their top, and the order its videos come in.
type FolderQuery struct {
	LibraryID int64
	Path      string
	Sort      string // SortRecorded, SortName, SortFileName or SortAdded
	Order     string // OrderAscending, OrderDescending, or OrderDefault for the sort's own (D366 §2)
	Reach     auth.Reach
	UserID    int64
}

// Folder is one folder of an other library: the folders in it, and its own videos in the order
// asked for (D349 §5).
type Folder struct {
	Path       string
	Subfolders []Subfolder
	Entries    []Entry
}

// Subfolder is a folder in a folder: its name, its path below the roots, and how many videos it
// holds, in the folders inside it too.
type Subfolder struct {
	Name, Path string
	VideoCount int64
}

// placedVideo is an other video and where its file is below the library's roots.
type placedVideo struct {
	row      sqlcgen.ListOtherVideosOfLibraryRow
	relative string
}

// Folder is one folder of an other library the caller reaches, as the scanner saw its files: a
// library with several roots shows their folders together (D163; D349 §5).
func (browser *Browser) Folder(ctx context.Context, query FolderQuery) (Folder, error) {
	if !query.Reach.Reaches(&query.LibraryID) {
		return Folder{}, fmt.Errorf("library %d: %w", query.LibraryID, ErrNoLibrary)
	}
	placed, err := browser.placedVideos(ctx, query.LibraryID)
	if err != nil {
		return Folder{}, err
	}
	folder := Folder{Path: cleanFolder(query.Path), Subfolders: []Subfolder{}}
	counts := map[string]int64{}
	var here []placedVideo
	for _, video := range placed {
		directory := cleanFolder(path.Dir(video.relative))
		if directory == folder.Path {
			here = append(here, video)
			continue
		}
		if child, isInside := childOf(folder.Path, directory); isInside {
			counts[child]++
		}
	}
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		folder.Subfolders = append(folder.Subfolders, Subfolder{
			Name: name, Path: path.Join(folder.Path, name), VideoCount: counts[name],
		})
	}
	if sortErr := browser.sortPlaced(ctx, here, query.Sort); sortErr != nil {
		return Folder{}, sortErr
	}
	if (ListQuery{Sort: query.Sort, Order: query.Order}).isReversed() {
		// The other way from the sort's own: Z to A, or oldest first (D366 §2).
		slices.Reverse(here)
	}
	page := Page{Entries: make([]Entry, 0, len(here))}
	for _, video := range here {
		row := video.row
		facts := &OtherFacts{RecordedAtMs: row.RecordedAt, RecordedFrom: row.RecordedFrom, FileName: row.FileName}
		page.Entries = append(page.Entries, otherEntry(row.TitleID, &row.Name, row.IdentificationState,
			row.AddedAt, facts))
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

// placedVideos are an other library's live videos, each with its file's path below its root.
func (browser *Browser) placedVideos(ctx context.Context, libraryID int64) ([]placedVideo, error) {
	queries := browser.database.Read()
	held, err := store.OrNotFound(queries.GetLibraryForIdentify(ctx, libraryID))
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("library %d: %w", libraryID, ErrNoLibrary)
	}
	if err != nil {
		return nil, fmt.Errorf("read library %d: %w", libraryID, err)
	}
	if held.Kind != LibraryKindOther {
		return nil, fmt.Errorf("library %d: %w", libraryID, ErrNoFolders)
	}
	roots, err := queries.ListLibraryRoots(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list library %d's roots: %w", libraryID, err)
	}
	rows, err := queries.ListOtherVideosOfLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list library %d's videos: %w", libraryID, err)
	}
	placed := make([]placedVideo, 0, len(rows))
	for _, row := range rows {
		placed = append(placed, placedVideo{row: row, relative: belowRoots(row.Path, roots)})
	}
	return placed, nil
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
// sort_locale, or newest added first.
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
				cmp.Compare(one.TitleID, two.TitleID))
		case SortFileName:
			return cmp.Or(bytes.Compare(one.FileNameKey, two.FileNameKey), cmp.Compare(one.TitleID, two.TitleID))
		case SortAdded:
			return cmp.Or(cmp.Compare(two.FoundAt, one.FoundAt), cmp.Compare(two.FoundFileModifiedAt,
				one.FoundFileModifiedAt), cmp.Compare(two.TitleID, one.TitleID))
		}
		return cmp.Or(cmp.Compare(two.RecordedAt, one.RecordedAt),
			cmp.Compare(two.TitleID, one.TitleID))
	})
	return nil
}

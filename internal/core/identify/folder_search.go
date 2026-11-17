// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// searchTitle is searchWithSpellings for a title's query, and, for a film whose name finds no work
// of it, for the name of the folder holding its files where that folder names another (D513 §4): a
// release names its file in its own words, as an anime film's "Gekijouban" or a fansub's
// episode-like name, and its folder the work (staging's titles/170 and titles/184). It returns the
// query the candidates answer and the candidates: the folder's, then the name's.
func (matcher *Matcher) searchTitle(
	ctx context.Context, provider Provider, itemID int64, query Query,
) (Query, []Candidate, error) {
	searched, candidates, err := searchWithSpellings(ctx, provider, query)
	if err != nil || query.Kind != KindMovie || hasWorkNamed(searched.Name, candidates) {
		return searched, candidates, err
	}
	name, year, err := matcher.folderOfFiles(ctx, itemID, query.Name)
	if err != nil || name == "" {
		return searched, candidates, err
	}
	byFolder, found, err := searchWithSpellings(ctx, provider,
		Query{Kind: query.Kind, Name: name, Year: cmp.Or(query.Year, year)})
	if err != nil {
		return query, nil, err
	}
	if !hasWorkNamed(byFolder.Name, found) {
		return searched, candidates, nil
	}
	return byFolder, withCandidates(found, candidates), nil
}

// folderOfFiles is the name and year, as cleanTitle reads them, of the folder holding the first of
// a title's live files that is in a folder below its library's root and whose folder names another
// work than name, by textindex.SameWork; "" where none is so, or the title is filed in no library.
func (matcher *Matcher) folderOfFiles(ctx context.Context, itemID int64, name string) (string, int, error) {
	reads := matcher.database.Read()
	title, err := reads.GetWorkForMatch(ctx, itemID)
	if err != nil {
		return "", 0, fmt.Errorf("read title %d: %w", itemID, err)
	}
	if title.FilingLibraryID == nil {
		return "", 0, nil
	}
	rootRows, err := reads.ListLibraryRoots(ctx, *title.FilingLibraryID)
	if err != nil {
		return "", 0, fmt.Errorf("list library %d's roots: %w", *title.FilingLibraryID, err)
	}
	roots := make([]string, 0, len(rootRows))
	for _, root := range rootRows {
		roots = append(roots, root.RootPath)
	}
	files, err := reads.ListLiveFilesOfWork(ctx, itemID)
	if err != nil {
		return "", 0, fmt.Errorf("list title %d's files: %w", itemID, err)
	}
	for _, file := range files {
		relative, isBelow := belowRoots(file.Path, roots)
		if folder := path.Dir(relative); isBelow && folder != "." {
			if folderName, year := cleanTitle(path.Base(folder)); folderName != "" &&
				textindex.SameWork(folderName) != textindex.SameWork(name) {
				return folderName, year, nil
			}
		}
	}
	return "", 0, nil
}

// belowRoots is a file's path below the first of a library's roots that holds it, and whether one
// does; the path itself where none does.
func belowRoots(filePath string, roots []string) (string, bool) {
	for _, root := range roots {
		if rest, isUnder := strings.CutPrefix(filePath, strings.TrimSuffix(root, "/")+"/"); isUnder {
			return rest, true
		}
	}
	return filePath, false
}

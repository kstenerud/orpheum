// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The doctor's look at the files found at more than one place (D363 §8, §9): an admin sees every place
// of a file, and the items whose places' NFO files disagree, whose match the NFO beside the place first
// found settles.

// placesListedAtMost bounds how many places the doctor reads of the files found at more than one: a
// justified constant, a bound on one admin's page, which says when it reached it.
const placesListedAtMost = 1000

// FileAtPlaces is a file found at more than one place, with each, its first first.
type FileAtPlaces struct {
	FileID int64
	Paths  []string
}

// FilesAtSeveralPlaces are the live files found at more than one place, by id, and whether there are
// more than were read (placesListedAtMost).
func (admin *Admin) FilesAtSeveralPlaces(ctx context.Context) ([]FileAtPlaces, bool, error) {
	rows, err := admin.database.Read().ListFilesAtSeveralPlaces(ctx, sqlcgen.ListFilesAtSeveralPlacesParams{
		AfterID: 0, MaxRows: placesListedAtMost,
	})
	if err != nil {
		return nil, false, fmt.Errorf("list the files found at more than one place: %w", err)
	}
	var files []FileAtPlaces
	for _, row := range rows {
		if len(files) == 0 || files[len(files)-1].FileID != row.FileID {
			files = append(files, FileAtPlaces{FileID: row.FileID})
		}
		last := &files[len(files)-1]
		last.Paths = append(last.Paths, row.Path)
	}
	isMore := len(rows) == placesListedAtMost
	if isMore && len(files) > 1 {
		files = files[:len(files)-1] // its places were cut off
	}
	return files, isMore, nil
}

// DisagreeingNFOs is an item whose file's places have NFO files that say different things: the one
// beside the place first found speaks for it (D363 §9).
type DisagreeingNFOs struct {
	FileID  int64
	TitleID int64
	Name    string
	// Paths are the places whose NFO files disagree, the first found first.
	Paths []string
}

// ItemsWhoseNFOsDisagree are the items whose files, found at more than one place, have NFO files beside
// their places that are not byte for byte the same.
func (admin *Admin) ItemsWhoseNFOsDisagree(ctx context.Context) ([]DisagreeingNFOs, error) {
	queries := admin.database.Read()
	rows, err := queries.ListNFOsOfFilesAtSeveralPlaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the NFO files beside the places of files found at more than one: %w", err)
	}
	var disagreeing []DisagreeingNFOs
	for start := 0; start < len(rows); {
		end := start + 1
		isSame := true
		for end < len(rows) && rows[end].FileID == rows[start].FileID {
			isSame = isSame && bytes.Equal(rows[end].Body, rows[start].Body)
			end++
		}
		if !isSame {
			found := DisagreeingNFOs{FileID: rows[start].FileID}
			for _, row := range rows[start:end] {
				found.Paths = append(found.Paths, row.Path)
			}
			title, titleErr := queries.GetFilesWork(ctx, found.FileID)
			if titleErr != nil {
				return nil, fmt.Errorf("read what file %d is: %w", found.FileID, titleErr)
			}
			found.TitleID, found.Name = ptr.ValueOrZero(title.WorkID), ptr.ValueOrZero(title.Name)
			disagreeing = append(disagreeing, found)
		}
		start = end
	}
	return disagreeing, nil
}

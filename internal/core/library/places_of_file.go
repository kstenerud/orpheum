// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A file is found at one or more places, a node and a path (D363 §2): its row is its first place, and
// each confirmed copy of it at another place a row filed FilingCopy that names it. What is known of
// the file — its filings, tracks, probe and what anyone watched of it — is on its row; what lies beside
// each place is that place's (§3).

// removePlace tombstones a place that is gone: a copy's row; or a file's first place, which, while a
// copy of it lives, the earliest found of those takes over: the file's row moves to the copy's place,
// keeping its id, its history, its filings and its tracks, what lay beside the copy is attached to the
// file, and the copy's row is tombstoned. A file with no copy left is gone. It reports whether the file
// itself went, which a caller cancelling its work needs.
func removePlace(
	ctx context.Context, queries *sqlcgen.Queries, logger *slog.Logger, placeID, nowMs int64,
) (isFileGone bool, err error) {
	copies, err := queries.ListLiveCopiesOf(ctx, placeID)
	if err != nil {
		return false, fmt.Errorf("list the copies of file %d: %w", placeID, err)
	}
	if len(copies) == 0 {
		// A copy, or a file at no other place.
		if err = queries.TombstoneFile(ctx, sqlcgen.TombstoneFileParams{RemovedAt: &nowMs, ID: placeID}); err != nil {
			return false, fmt.Errorf("tombstone file %d: %w", placeID, err)
		}
		return true, nil
	}
	heir := copies[0]
	// The copy's row goes first, so the one live row at its path is the file's from now on.
	if err = queries.TombstoneFile(ctx, sqlcgen.TombstoneFileParams{RemovedAt: &nowMs, ID: heir.ID}); err != nil {
		return false, fmt.Errorf("retire copy %d: %w", heir.ID, err)
	}
	err = queries.MoveFileToItsCopysPlace(ctx, sqlcgen.MoveFileToItsCopysPlaceParams{
		NodeID: heir.NodeID, Path: heir.Path, ReportedPathRaw: heir.ReportedPathRaw,
		ModifiedAt: heir.ModifiedAt, FilesystemDevice: heir.FilesystemDevice, Inode: heir.Inode,
		LastSeenAt: heir.LastSeenAt, IsUnconfirmed: heir.IsUnconfirmed, ID: placeID,
	})
	if err == nil {
		err = queries.ReattachSidecars(ctx, sqlcgen.ReattachSidecarsParams{ToPlace: &placeID, FromPlace: &heir.ID})
	}
	if err != nil {
		return false, fmt.Errorf("move file %d to its copy %d's place: %w", placeID, heir.ID, err)
	}
	logger.InfoContext(ctx, "a file's first place went; it is at its copy's place from now on", "fileId", placeID,
		"copyId", heir.ID, "nodeId", heir.NodeID, "path", heir.Path)
	return false, nil
}

// removePlaces removes the places named, which a scan no longer sees: the copies among them first, so a
// file whose first place and copy both went is gone, and one whose copy lives elsewhere is there.
func removePlaces(
	ctx context.Context, queries *sqlcgen.Queries, logger *slog.Logger, placeIDs []int64, nowMs int64,
) error {
	if len(placeIDs) == 0 {
		return nil
	}
	encoded, err := json.Marshal(placeIDs)
	if err != nil {
		return fmt.Errorf("encode %d place ids: %w", len(placeIDs), err)
	}
	rows, err := queries.ListLivePlacesOfFiles(ctx, string(encoded))
	if err != nil {
		return fmt.Errorf("list %d places: %w", len(placeIDs), err)
	}
	isCopy := map[int64]bool{}
	for _, row := range rows {
		isCopy[row.ID] = row.IsCopy == 1
	}
	for _, copiesFirst := range []bool{true, false} {
		for _, placeID := range placeIDs {
			if isCopy[placeID] != copiesFirst {
				continue
			}
			if _, removeErr := removePlace(ctx, queries, logger, placeID, nowMs); removeErr != nil {
				return removeErr
			}
		}
	}
	return nil
}

// removeFilesAndPlaces removes every live place of each file named, their copies and then their first,
// and returns the places removed: a file an admin says is gone is gone wherever it was found.
func removeFilesAndPlaces(
	ctx context.Context, queries *sqlcgen.Queries, logger *slog.Logger, fileIDs []int64, nowMs int64,
) ([]int64, error) {
	encoded, err := json.Marshal(fileIDs)
	if err != nil {
		return nil, fmt.Errorf("encode %d file ids: %w", len(fileIDs), err)
	}
	places, err := queries.ListLivePlacesOfFiles(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("list the places of %d files: %w", len(fileIDs), err)
	}
	removed := make([]int64, 0, len(places))
	for _, isCopies := range []bool{true, false} {
		for _, place := range places {
			if (place.IsCopy == 1) != isCopies {
				continue
			}
			if _, err := removePlace(ctx, queries, logger, place.ID, nowMs); err != nil {
				return nil, err
			}
			removed = append(removed, place.ID)
		}
	}
	return removed, nil
}

// VersionRetirer retires a version a row found to be another file left, into that file's title of the
// same kind (intoTitleID), and the title it was in when no live version is left: identify's
// RetireVersionInto, which writes versions and titles. It runs in the caller's transaction.
type VersionRetirer func(ctx context.Context, queries *sqlcgen.Queries, versionID, intoTitleID, nowMs int64) error

// libraryKinds are the kinds of library, each of which files a file in a title of its own (D363 §5).
func libraryKinds() []string { return []string{LibraryKindMovie, LibraryKindSeries, LibraryKindOther} }

// retireFilingsInto gives fileID what each kind of library filed rowID as, rowID being found to be
// fileID: an unread row whose read found a file stored before (D304 §5), or a copy a whole read
// confirmed (D363 §2). A kind that files fileID too keeps fileID's title, the row's version retired
// into it by retire; a kind that files fileID in none files it as it filed the row. The row files
// nothing after.
func retireFilingsInto(
	ctx context.Context, queries *sqlcgen.Queries, retire VersionRetirer, rowID, fileID, nowMs int64,
) error {
	rowVersions, err := queries.ListFileVersions(ctx, rowID)
	if err != nil {
		return fmt.Errorf("read file %d's versions: %w", rowID, err)
	}
	fileVersions, err := queries.ListFileVersions(ctx, fileID)
	if err != nil {
		return fmt.Errorf("read file %d's versions: %w", fileID, err)
	}
	titleOfKind := map[string]int64{}
	for _, version := range fileVersions {
		titleOfKind[version.LibraryKind] = version.WorkID
	}
	for _, kind := range libraryKinds() {
		err = queries.MoveFilingToFile(ctx,
			sqlcgen.MoveFilingToFileParams{ToFile: fileID, FromFile: rowID, LibraryKind: kind})
		if err != nil {
			return fmt.Errorf("give file %d's %s filing to file %d: %w", rowID, kind, fileID, err)
		}
	}
	if err := queries.DeleteFilingsOfFile(ctx, rowID); err != nil {
		return fmt.Errorf("drop file %d's filings: %w", rowID, err)
	}
	for _, version := range rowVersions {
		intoTitleID, isFiledToo := titleOfKind[version.LibraryKind]
		if !isFiledToo || version.VersionID == nil || retire == nil {
			continue // moved with its filing, or nothing to retire it
		}
		if err := retire(ctx, queries, *version.VersionID, intoTitleID, nowMs); err != nil {
			return err
		}
	}
	return nil
}

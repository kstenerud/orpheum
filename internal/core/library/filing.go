// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// This package writes the file table and file_filing, and nothing else does (schema.md, "Who writes
// each table"). Identification decides how each kind of library files a file and which version it is
// in; it records that through these, in its own transaction.

// FileAs records how a kind of library's identification filed a file and the reason for it (D163,
// D363 §5): listed, unidentified or an extra. An empty reason is recorded as none.
func FileAs(ctx context.Context, queries *sqlcgen.Queries, fileID int64, libraryKind, filing, reason string) error {
	err := queries.UpsertFileFiling(ctx, sqlcgen.UpsertFileFilingParams{
		FileID: fileID, LibraryKind: libraryKind, Filing: filing, FilingReason: ptr.NilIfZero(reason),
	})
	if err != nil {
		return fmt.Errorf("record file %d as %s of a %s library: %w", fileID, filing, libraryKind, err)
	}
	return nil
}

// PutInVersion puts a file in a version, the one its kind of library files it in, and records its path
// as the one it was identified from, so a later rename or move is seen as one (D208 §1, D363 §5).
func PutInVersion(
	ctx context.Context, queries *sqlcgen.Queries, fileID int64, libraryKind string, versionID int64,
) error {
	err := queries.SetFileVersionAndIdentifiedPath(ctx, sqlcgen.SetFileVersionAndIdentifiedPathParams{
		LibraryKind: libraryKind, VersionID: &versionID, FileID: fileID,
	})
	if err != nil {
		return fmt.Errorf("put file %d in version %d of a %s library: %w", fileID, versionID, libraryKind, err)
	}
	return nil
}

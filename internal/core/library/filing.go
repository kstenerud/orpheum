// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// This package writes the file table and nothing else does (schema.md, "Who writes each table").
// Identification decides how a file is filed and which version it is in; it records that through
// these, in its own transaction.

// FileAs records a file's filing and the reason for it (D163): listed, unidentified or an extra,
// as identification decided. An empty reason is recorded as none.
func FileAs(ctx context.Context, queries *sqlcgen.Queries, fileID int64, filing, reason string) error {
	err := queries.SetFileFiling(ctx, sqlcgen.SetFileFilingParams{
		Filing: filing, FilingReason: ptr.NilIfZero(reason), ID: fileID,
	})
	if err != nil {
		return fmt.Errorf("record file %d as %s: %w", fileID, filing, err)
	}
	return nil
}

// PutInVersion puts a file in a version and records its path as the one it was identified from,
// so a later rename or move is seen as one (D208 §1).
func PutInVersion(ctx context.Context, queries *sqlcgen.Queries, fileID, versionID int64) error {
	err := queries.SetFileVersionAndIdentifiedPath(ctx,
		sqlcgen.SetFileVersionAndIdentifiedPathParams{VersionID: &versionID, ID: fileID})
	if err != nil {
		return fmt.Errorf("put file %d in version %d: %w", fileID, versionID, err)
	}
	return nil
}

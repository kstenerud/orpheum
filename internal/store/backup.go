// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// BackupInto writes a consistent copy of the database to path with VACUUM INTO: one file, no
// WAL sidecars, safe beside a live writer (D77; backup-restore.md measured a plain copy of a WAL
// database producing a file with no tables). The target must not exist yet. The copy carries sealed
// ciphertext only; secret.key is never in the database (D194 §4).
func (database *Database) BackupInto(ctx context.Context, path string) error {
	if strings.Contains(path, "?") {
		return fmt.Errorf("back up into %q: a path may not contain '?'", path)
	}
	if _, err := database.writer.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("back up into %q: %w", path, err)
	}
	return nil
}

// BackupDescription describes a backup or snapshot read without modifying it.
type BackupDescription struct {
	ApplicationID int64
	State         SchemaState
}

// InspectBackup reads a backup's application_id and schema versions. It checks the file exists
// first: the driver creates a missing file, so a typo in the path would otherwise "restore" an
// empty database over the live one (backup-restore.md, from Navidrome).
func InspectBackup(ctx context.Context, path string) (inspected BackupDescription, err error) {
	if strings.Contains(path, "?") {
		return inspected, fmt.Errorf("backup %q: a path may not contain '?'", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return inspected, fmt.Errorf("backup %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return inspected, fmt.Errorf("backup %q is not a regular file", path)
	}
	pool, err := sql.Open("sqlite", readOnlyURI(path)+"&_pragma=query_only(1)")
	if err != nil {
		return inspected, fmt.Errorf("open backup %q: %w", path, err)
	}
	defer func() { err = errors.Join(err, wrapIf("close backup", pool.Close())) }()
	err = pool.QueryRowContext(ctx, "PRAGMA application_id").Scan(&inspected.ApplicationID)
	if err != nil {
		return inspected, fmt.Errorf("backup %q: read application_id: %w", path, err)
	}
	if inspected.ApplicationID != ApplicationID {
		return inspected, fmt.Errorf("backup %q: %w (found %d)", path, ErrNotOurs, inspected.ApplicationID)
	}
	inspected.State, err = readState(ctx, pool)
	if err != nil {
		return inspected, fmt.Errorf("backup %q: %w", path, err)
	}
	return inspected, nil
}

// CopyBackup writes a fresh copy of the backup at source into target with VACUUM INTO, after
// InspectBackup has vouched for it. The target must not exist yet.
func CopyBackup(ctx context.Context, source, target string) (err error) {
	if strings.Contains(target, "?") {
		return fmt.Errorf("copy backup into %q: a path may not contain '?'", target)
	}
	if _, err = InspectBackup(ctx, source); err != nil {
		return err
	}
	pool, err := sql.Open("sqlite", readOnlyURI(source))
	if err != nil {
		return fmt.Errorf("open backup %q: %w", source, err)
	}
	defer func() { err = errors.Join(err, wrapIf("close backup", pool.Close())) }()
	if _, err = pool.ExecContext(ctx, "VACUUM INTO ?", target); err != nil {
		return fmt.Errorf("copy backup %q into %q: %w", source, target, err)
	}
	return nil
}

// readOnlyURI opens path read-only as a SQLite URI, escaped so that '#' or '%' in a directory
// name cannot change which file opens.
func readOnlyURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String() + "?mode=ro"
}

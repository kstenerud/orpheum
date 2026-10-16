// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/errs"
)

// BackupInto writes a consistent copy of the database to path with VACUUM INTO: one file, no
// WAL sidecars, safe beside a live writer (D77; backup-restore.md measured a plain copy of a WAL
// database producing a file with no tables). It reads a snapshot on the maintenance connection, so
// writes go on while it copies (D303 §7). The copy is then checked whole (integrity_check), and one
// that fails is refused, ErrDamagedCopy, for the caller to remove: a backup that would not restore
// is no backup (D303 §7, M6).
// The target must not exist yet. The copy carries sealed ciphertext only; secret.key is never in
// the database (D194 §4).
func (database *Database) BackupInto(ctx context.Context, path string) error {
	if strings.Contains(path, "?") {
		return fmt.Errorf("back up into %q: a path may not contain '?'", path)
	}
	if _, err := database.maintenance.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("back up into %q: %w", path, err)
	}
	return checkCopy(ctx, path)
}

// ErrDamagedCopy is a backup whose copy fails SQLite's integrity check.
var ErrDamagedCopy = errors.New("the copy fails its integrity check")

// checkCopy runs integrity_check over the copy at path, which must answer "ok" alone. The copy is
// opened query_only, not mode=ro: on a read-only file SQLite's integrity_check skips CHECK
// constraints and answers "ok" (idiosyncrasies.md).
func checkCopy(ctx context.Context, path string) (err error) {
	pool, err := sql.Open("sqlite", path+"?_pragma=query_only(1)")
	if err != nil {
		return fmt.Errorf("open the copy %q: %w", path, err)
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the copy", pool.Close())) }()
	rows, err := pool.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("check the copy %q: %w", path, err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // a read; rows.Err is checked
	var found []string
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			return fmt.Errorf("check the copy %q: %w", path, err)
		}
		found = append(found, line)
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("check the copy %q: %w", path, err)
	}
	if len(found) != 1 || found[0] != "ok" {
		return fmt.Errorf("back up into %q: %w: %s", path, ErrDamagedCopy, strings.Join(found, "; "))
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
	defer func() { err = errors.Join(err, errs.WrapIf("close backup", pool.Close())) }()
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
	defer func() { err = errors.Join(err, errs.WrapIf("close backup", pool.Close())) }()
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

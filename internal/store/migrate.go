// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SchemaState is what a database file says about its schema: the version it is at, and the
// oldest schema whose code can still use it (migrations.md, Bookkeeping).
type SchemaState struct {
	Version           int
	CompatibleVersion int
}

// MigrateOptions are what the runner needs from its caller.
type MigrateOptions struct {
	// SnapshotDirectory receives pre-migrate-vN.sqlite before the first pending step, and keeps
	// it after success (D181).
	SnapshotDirectory string
	// BeforeStep, if set, is called before each pending step with its position and the number
	// pending, so /health can say "migrating: step 3 of 7" before the step starts.
	BeforeStep func(step, total int)
	// SpaceFactor is the free space the preflight needs, as a multiple of the database's size:
	// the snapshot plus a rebuild's worst case. It comes from the config file (D200).
	SpaceFactor int
}

// NewerDatabaseError refuses a database that a newer release upgraded past what this binary can
// use, and names the snapshot taken before that upgrade (migrations.md, A newer database).
type NewerDatabaseError struct {
	State         SchemaState
	BinaryVersion int
	Snapshot      string
}

func (refusal *NewerDatabaseError) Error() string {
	return fmt.Sprintf("this database was upgraded to schema %d by a newer release and cannot be used by this one, "+
		"which understands schema %d and older (the file needs schema %d or newer). The snapshot %s, taken "+
		"before that upgrade, can be restored with `orpheum restore`; you will lose what happened since",
		refusal.State.Version, refusal.BinaryVersion, refusal.State.CompatibleVersion, refusal.Snapshot)
}

// StepError is a step that failed and rolled back. The database stays at the version before it.
type StepError struct {
	Step     Step
	Snapshot string
	Err      error
}

func (failure *StepError) Error() string {
	snapshot := "no snapshot was needed"
	if failure.Snapshot != "" {
		snapshot = "the snapshot taken before migrating is " + failure.Snapshot
	}
	return fmt.Sprintf("migration step %s failed and was rolled back; %s: %v", failure.Step.Name, snapshot, failure.Err)
}

func (failure *StepError) Unwrap() error {
	return failure.Err
}

// State reads the schema version and compatible version. A file with no schema is at version 0.
func (database *Database) State(ctx context.Context) (SchemaState, error) {
	return readState(ctx, database.readers)
}

// Pending returns the steps this binary would run on the database, none if it is current.
func (database *Database) Pending(ctx context.Context) ([]Step, error) {
	steps, err := Steps()
	if err != nil {
		return nil, err
	}
	state, err := database.State(ctx)
	if err != nil {
		return nil, err
	}
	return pendingSteps(steps, state), nil
}

// Migrate brings the database to this binary's schema, one committed step at a time, so an
// interruption resumes at the last completed step. A database a newer release left unusable is
// refused with a NewerDatabaseError.
func (database *Database) Migrate(ctx context.Context, options MigrateOptions) error {
	steps, err := Steps()
	if err != nil {
		return err
	}
	return database.migrate(ctx, steps, options)
}

func (database *Database) migrate(ctx context.Context, steps []Step, options MigrateOptions) error {
	state, err := database.State(ctx)
	if err != nil {
		return err
	}
	err = refuseNewer(state, len(steps), options.SnapshotDirectory)
	if err != nil {
		return err
	}
	if checkErr := database.checkAppliedSteps(ctx, steps, state.Version); checkErr != nil {
		return checkErr
	}
	pending := pendingSteps(steps, state)
	if len(pending) == 0 {
		return database.recordAppliedSteps(ctx, steps, state.Version)
	}
	snapshot, err := database.snapshotBeforeMigrating(ctx, state.Version, options)
	if err != nil {
		return err
	}
	for position, step := range pending {
		if options.BeforeStep != nil {
			options.BeforeStep(position+1, len(pending))
		}
		err = database.runStep(ctx, step)
		if err != nil {
			return &StepError{Step: step, Snapshot: snapshot, Err: err}
		}
	}
	return database.recordAppliedSteps(ctx, steps, len(steps))
}

func pendingSteps(steps []Step, state SchemaState) []Step {
	if state.Version >= len(steps) {
		return nil
	}
	return steps[state.Version:]
}

// UsableBy reports whether a binary that understands schemas up to binaryVersion may use a file
// in this state: an older or equal file always (it is migrated), a newer one only if the release
// that upgraded it declared it still compatible (migrations.md, A newer database).
func (state SchemaState) UsableBy(binaryVersion int) bool {
	return state.Version <= binaryVersion || state.CompatibleVersion <= binaryVersion
}

// refuseNewer runs a newer file only if it declared itself usable by this binary's schema.
func refuseNewer(state SchemaState, binaryVersion int, snapshotDirectory string) error {
	if state.UsableBy(binaryVersion) {
		return nil
	}
	return &NewerDatabaseError{
		State:         state,
		BinaryVersion: binaryVersion,
		Snapshot:      filepath.Join(snapshotDirectory, snapshotName(binaryVersion)),
	}
}

func snapshotName(fromVersion int) string {
	return fmt.Sprintf("pre-migrate-v%d.sqlite", fromVersion)
}

// snapshotBeforeMigrating writes pre-migrate-vN.sqlite with VACUUM INTO, one per from-version so
// a restart loop cannot fill the disk, and returns its path. A file with no schema has nothing
// to keep, so it gets none.
func (database *Database) snapshotBeforeMigrating(
	ctx context.Context, fromVersion int, options MigrateOptions,
) (string, error) {
	if fromVersion == 0 {
		return "", nil
	}
	directory := options.SnapshotDirectory
	if directory == "" {
		return "", errors.New("migrate: no snapshot directory to write the pre-migration snapshot into")
	}
	path := filepath.Join(directory, snapshotName(fromVersion))
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return path, nil
	case !errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("check for snapshot %s: %w", path, err)
	}
	err = database.requireSpaceToMigrate(ctx, directory, options.SpaceFactor)
	if err != nil {
		return "", err
	}
	err = database.BackupInto(ctx, path)
	if err != nil {
		return "", fmt.Errorf("snapshot before migrating: %w", err)
	}
	return path, nil
}

// requireSpaceToMigrate refuses, with the numbers, when the snapshot plus a rebuild's worst case
// would not fit: factor times the database, where a factor of 3 is the snapshot plus twice the
// whole database, which bounds twice its largest table.
func (database *Database) requireSpaceToMigrate(ctx context.Context, directory string, factor int) error {
	if factor < 1 {
		return fmt.Errorf("the migration space factor is %d; it must be at least 1", factor)
	}
	size, err := database.sizeBytes(ctx)
	if err != nil {
		return err
	}
	needed := uint64(factor) * size
	for _, where := range []string{directory, filepath.Dir(database.path)} {
		available, measureErr := availableBytes(where)
		if measureErr != nil {
			return fmt.Errorf("measure free space in %s: %w", where, measureErr)
		}
		if available < needed {
			return fmt.Errorf("not enough free space to migrate safely: %s has %d bytes free, and the snapshot plus "+
				"a rebuild may need %d (%d times the %d-byte database); nothing was changed",
				where, available, needed, factor, size)
		}
	}
	return nil
}

func (database *Database) sizeBytes(ctx context.Context) (uint64, error) {
	var pages, pageSize int64
	if err := database.readers.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return 0, fmt.Errorf("read page_count: %w", err)
	}
	if err := database.readers.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("read page_size: %w", err)
	}
	if pages < 0 || pageSize < 0 {
		return 0, fmt.Errorf("database reports %d pages of %d bytes", pages, pageSize)
	}
	return uint64(pages) * uint64(pageSize), nil
}

// runStep applies one step on a pinned connection (migrations.md, The runner is ours): foreign
// keys off outside the transaction, BEGIN IMMEDIATE, the step, the runner's own checks, the
// version, COMMIT, foreign keys back on. Anything that fails rolls the step back whole.
func (database *Database) runStep(ctx context.Context, step Step) (err error) {
	connection, err := database.writer.Conn(ctx)
	if err != nil {
		return fmt.Errorf("pin a connection: %w", err)
	}
	defer func() { err = errors.Join(err, wrapIf("release the connection", connection.Close())) }()
	err = setForeignKeys(ctx, connection, false)
	if err != nil {
		return err
	}
	// The restore uses a fresh context: foreign keys must come back on even when ctx has ended.
	defer func() {
		err = errors.Join(err, setForeignKeys(context.WithoutCancel(ctx), connection, true))
	}()
	_, err = connection.ExecContext(ctx, "BEGIN IMMEDIATE")
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	err = applyStep(ctx, connection, step)
	if err == nil {
		_, err = connection.ExecContext(ctx, "COMMIT")
		err = wrapIf("commit", err)
	}
	if err != nil {
		_, rollbackErr := connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		return errors.Join(err, wrapIf("roll back", rollbackErr))
	}
	return nil
}

// applyStep runs inside the step's transaction.
func applyStep(ctx context.Context, connection *sql.Conn, step Step) error {
	sequencesBefore, err := readSequences(ctx, connection)
	if err != nil {
		return err
	}
	_, err = connection.ExecContext(ctx, step.SQL)
	if err != nil {
		return fmt.Errorf("run %s: %w", step.Name, err)
	}
	err = checkStep(ctx, connection, sequencesBefore)
	if err != nil {
		return err
	}
	return recordVersion(ctx, connection, step)
}

func recordVersion(ctx context.Context, connection *sql.Conn, step Step) error {
	if step.Version == 1 {
		if _, err := connection.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d", ApplicationID)); err != nil {
			return fmt.Errorf("set application_id: %w", err)
		}
	}
	if step.RaisesCompatible {
		const raise = "INSERT INTO schema_meta (id, compatible_version) VALUES (1, ?) " +
			"ON CONFLICT (id) DO UPDATE SET compatible_version = excluded.compatible_version"
		if _, err := connection.ExecContext(ctx, raise, step.Version); err != nil {
			return fmt.Errorf("raise compatible_version to %d: %w", step.Version, err)
		}
	}
	if _, err := connection.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", step.Version)); err != nil {
		return fmt.Errorf("set user_version to %d: %w", step.Version, err)
	}
	return nil
}

func setForeignKeys(ctx context.Context, connection *sql.Conn, enabled bool) error {
	statement, want := "PRAGMA foreign_keys = OFF", 0
	if enabled {
		statement, want = "PRAGMA foreign_keys = ON", 1
	}
	if _, err := connection.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("%s: %w", statement, err)
	}
	var got int
	if err := connection.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&got); err != nil {
		return fmt.Errorf("read foreign_keys back: %w", err)
	}
	if got != want {
		return fmt.Errorf("%s did not take: foreign_keys is %d", statement, got)
	}
	return nil
}

// readState reads user_version and schema_meta's compatible_version.
func readState(ctx context.Context, pool *sql.DB) (SchemaState, error) {
	var state SchemaState
	if err := pool.QueryRowContext(ctx, "PRAGMA user_version").Scan(&state.Version); err != nil {
		return state, fmt.Errorf("read user_version: %w", err)
	}
	if state.Version == 0 {
		return state, nil
	}
	err := pool.QueryRowContext(ctx, "SELECT compatible_version FROM schema_meta WHERE id = 1").
		Scan(&state.CompatibleVersion)
	if err != nil {
		return state, fmt.Errorf("read compatible_version: %w", err)
	}
	return state, nil
}

// wrapIf wraps err with what was being done, or returns nil.
func wrapIf(doing string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", doing, err)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	// The pure-Go SQLite driver registers itself as "sqlite" (D31).
	_ "modernc.org/sqlite"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/waiting"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// busyTimeoutMs is how long a connection waits for SQLite's lock before failing: checkpoints,
// VACUUM INTO, a migration or a backup run beside a live server (standards/sql.md §8).
const busyTimeoutMs = 15000

// The pragmas every connection gets, in the pure-Go driver's DSN form, so the pool applies them
// to each connection it opens rather than to whichever one ran an Exec (standards/sql.md §7).
const (
	connectionPragmas = "_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(15000)&_pragma=temp_store(MEMORY)"
	writerOptions = "&_txlock=immediate"
	readerOptions = "&_pragma=query_only(1)"
)

// synchronousNormal is PRAGMA synchronous's value for NORMAL.
const synchronousNormal = 1

// tempStoreMemory is PRAGMA temp_store's value for MEMORY. SQLite's temporary files would
// otherwise go wherever TMPDIR points, which is configuration nobody gave us (D204).
const tempStoreMemory = 2

// ErrNotFound is returned by One when a query that returns one row found none.
var ErrNotFound = errors.New("not found")

// Database is one SQLite file with exactly one write connection and a pool of read-only
// connections (standards/sql.md §8). Reads use Read; every write goes through Write.
type Database struct {
	path    string
	writer  *sql.DB
	readers *sql.DB
	// maintenance is one connection that may write but is not the writer: VACUUM INTO, which a
	// query_only reader refuses, reads a snapshot through it while writes go on (D303, the
	// independent audit's S7).
	maintenance *sql.DB
	// gate hands the writer to one write at a time, people's before background work.
	gate writerGate
	// slowWrites says each write that waited for the writer or held it past SlowWriteAfter; none
	// when its options named no clock or no listener.
	slowWrites slowWrites
}

// OpenOptions are the choices Open needs from the config file: they are known before the
// database opens, so they cannot be settings (D200). None has a default here.
type OpenOptions struct {
	// MinimumReaders is the smallest reader pool; a machine with more CPUs gets one per CPU.
	MinimumReaders int
	// Clock and OnSlowWrite, both set, are told each write that waited for the one writer, or held
	// it, longer than SlowWriteAfter: a request that writes waits behind it, and the log names it
	// (the owner's nas.example, 2026-10-15: /me took 12 s and removing a device 27 s). Either nil
	// times nothing.
	Clock       clock.Clock
	OnSlowWrite func(SlowWrite)
}

// SlowWriteAfter is how long a write may wait for the writer, or hold it, before it is reported.
const SlowWriteAfter = time.Second

// SlowWrite is a write that waited for the writer, or held it, past SlowWriteAfter: the database
// file's name, the function outside the store that wrote, the one that held the writer when it
// asked (when it waited long; D303, DL17), and both times.
type SlowWrite struct {
	File   string
	Caller string
	Behind string
	Waited time.Duration
	Held   time.Duration
}

// slowWrites is a database's timing of its writes, from its options.
type slowWrites struct {
	file   string
	clock  clock.Clock
	report func(SlowWrite)
}

// isOn is whether writes are timed.
func (writes slowWrites) isOn() bool { return writes.clock != nil && writes.report != nil }

// callerOutsideTheStore is the first function that called into the store, for a slow write's report.
func callerOutsideTheStore() string {
	frames := make([]uintptr, 16)
	found := runtime.CallersFrames(frames[:runtime.Callers(3, frames)])
	for {
		frame, more := found.Next()
		if !strings.Contains(frame.Function, "/internal/store.") || !more {
			return frame.Function
		}
	}
}

// open opens a database file that must carry applicationID or be empty.
func open(ctx context.Context, path string, options OpenOptions, applicationID int64) (*Database, error) {
	if options.MinimumReaders < 1 {
		return nil, fmt.Errorf("open database %q: the reader pool minimum is %d; it must be at least 1",
			path, options.MinimumReaders)
	}
	if strings.Contains(path, "?") {
		// The driver splits the DSN at '?', so such a path would open a different file
		// (backup-restore.md, from Navidrome).
		return nil, fmt.Errorf("open database %q: a path may not contain '?'", path)
	}
	writer, err := sql.Open("sqlite", path+"?"+connectionPragmas+writerOptions)
	if err != nil {
		return nil, fmt.Errorf("open database %q for writing: %w", path, err)
	}
	writer.SetMaxOpenConns(1)
	readers, err := sql.Open("sqlite", path+"?"+connectionPragmas+readerOptions)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open database %q for reading: %w", path, err), writer.Close())
	}
	readers.SetMaxOpenConns(max(options.MinimumReaders, runtime.NumCPU()))
	maintenance, err := sql.Open("sqlite", path+"?"+connectionPragmas)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open database %q for maintenance: %w", path, err), readers.Close(),
			writer.Close())
	}
	maintenance.SetMaxOpenConns(1)
	database := &Database{
		path: path, writer: writer, readers: readers, maintenance: maintenance,
		slowWrites: slowWrites{file: filepath.Base(path), clock: options.Clock, report: options.OnSlowWrite},
	}
	if err := database.verify(ctx, applicationID); err != nil {
		return nil, errors.Join(err, database.Close())
	}
	return database, nil
}

// Path is the database file's path.
func (database *Database) Path() string {
	return database.path
}

// Close closes the reader pool, the writer and the maintenance connection.
func (database *Database) Close() error {
	return errors.Join(database.readers.Close(), database.maintenance.Close(), database.writer.Close())
}

// Read returns the generated queries over the read-only pool. SQLite itself refuses a write
// through it (query_only).
func (database *Database) Read() *sqlcgen.Queries {
	return sqlcgen.New(database.readers)
}

// ReadTx runs read in one read transaction on the reader pool: every query in it sees the database
// as one commit left it, whatever commits meanwhile (D303, M2). A page or a decision built from
// more than one query reads through it; Read's queries each see the latest commit.
func (database *Database) ReadTx(ctx context.Context, read func(queries *sqlcgen.Queries) error) error {
	return readTransaction(ctx, database.readers, func(transaction *sql.Tx) error {
		return read(sqlcgen.New(transaction))
	})
}

// readTransaction runs read in one read-only transaction on pool, which ends when read returns.
func readTransaction(ctx context.Context, pool *sql.DB, read func(transaction *sql.Tx) error) error {
	transaction, err := pool.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin read transaction: %w", err)
	}
	if err := read(transaction); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("end read transaction: %w", err)
	}
	return nil
}

// Write runs change in one write transaction, which begins IMMEDIATE on the single writer and
// commits only if change returns nil.
func (database *Database) Write(ctx context.Context, change func(queries *sqlcgen.Queries) error) error {
	return database.transact(ctx, func(transaction *sql.Tx) error { return change(sqlcgen.New(transaction)) })
}

// transact runs change in one IMMEDIATE transaction on the single writer, which it takes through
// the writer gate: a person's write before background work (D303, DL11).
func (database *Database) transact(ctx context.Context, change func(transaction *sql.Tx) error) error {
	if !database.slowWrites.isOn() {
		if err := database.gate.acquire(ctx, waiting.IsAPerson(ctx), ""); err != nil {
			return err
		}
		defer database.gate.release()
		return database.transactHolding(ctx, change)
	}
	writes := database.slowWrites
	caller, behind := callerOutsideTheStore(), database.gate.heldBy()
	asked := writes.clock.Now()
	if err := database.gate.acquire(ctx, waiting.IsAPerson(ctx), caller); err != nil {
		return err
	}
	begun := writes.clock.Now()
	defer func() {
		database.gate.release()
		done := writes.clock.Now()
		waited, held := begun.Sub(asked), done.Sub(begun)
		if waited <= SlowWriteAfter {
			behind = ""
		}
		if waited > SlowWriteAfter || held > SlowWriteAfter {
			writes.report(SlowWrite{File: writes.file, Caller: caller, Behind: behind, Waited: waited, Held: held})
		}
	}()
	return database.transactHolding(ctx, change)
}

// transactHolding is transact once the gate has handed this write the writer.
func (database *Database) transactHolding(ctx context.Context, change func(transaction *sql.Tx) error) error {
	transaction, err := database.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write transaction: %w", err)
	}
	if err := change(transaction); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit write transaction: %w", err)
	}
	return nil
}

// OrNotFound turns a generated query's "no rows" into ErrNotFound, so callers outside the store can
// test for absence without importing database/sql.
func OrNotFound[Row any](row Row, err error) (Row, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

// verify reads back every pragma on every pool and checks the application_id.
func (database *Database) verify(ctx context.Context, wantApplicationID int64) error {
	for _, pool := range []*sql.DB{database.writer, database.readers, database.maintenance} {
		if err := verifyPragmas(ctx, pool); err != nil {
			return fmt.Errorf("database %q: %w", database.path, err)
		}
	}
	isEmpty, applicationID, err := database.emptinessAndApplicationID(ctx)
	if err != nil {
		return err
	}
	if applicationID != wantApplicationID && !isEmpty {
		return fmt.Errorf("database %q: %w (found %d)", database.path, ErrNotOurs, applicationID)
	}
	return nil
}

// verifyPragmas refuses a connection whose pragmas are not the ones the DSN asked for.
func verifyPragmas(ctx context.Context, pool *sql.DB) error {
	var journalMode string
	var synchronous, foreignKeys, busyTimeout, tempStore int
	checks := []struct {
		pragma string
		value  any
	}{
		{"PRAGMA journal_mode", &journalMode},
		{"PRAGMA synchronous", &synchronous},
		{"PRAGMA foreign_keys", &foreignKeys},
		{"PRAGMA busy_timeout", &busyTimeout},
		{"PRAGMA temp_store", &tempStore},
	}
	for _, check := range checks {
		if err := pool.QueryRowContext(ctx, check.pragma).Scan(check.value); err != nil {
			return fmt.Errorf("read %s: %w", check.pragma, err)
		}
	}
	switch {
	case journalMode != "wal":
		return fmt.Errorf("journal_mode is %q, want wal", journalMode)
	case synchronous != synchronousNormal:
		return fmt.Errorf("synchronous is %d, want %d (NORMAL)", synchronous, synchronousNormal)
	case foreignKeys != 1:
		return fmt.Errorf("foreign_keys is %d, want 1", foreignKeys)
	case busyTimeout != busyTimeoutMs:
		return fmt.Errorf("busy_timeout is %d, want %d", busyTimeout, busyTimeoutMs)
	case tempStore != tempStoreMemory:
		return fmt.Errorf("temp_store is %d, want %d (MEMORY)", tempStore, tempStoreMemory)
	}
	return nil
}

// emptinessAndApplicationID reports whether the file has no schema at all, and its application_id.
func (database *Database) emptinessAndApplicationID(
	ctx context.Context,
) (isEmpty bool, applicationID int64, err error) {
	var schemaObjectCount int64
	const countObjects = "SELECT count(*) FROM sqlite_schema"
	if err := database.readers.QueryRowContext(ctx, countObjects).Scan(&schemaObjectCount); err != nil {
		return false, 0, fmt.Errorf("database %q: count schema objects: %w", database.path, err)
	}
	if err := database.readers.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID); err != nil {
		return false, 0, fmt.Errorf("database %q: read application_id: %w", database.path, err)
	}
	return schemaObjectCount == 0, applicationID, nil
}

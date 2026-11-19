// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// tallying is the generated queries' connection, counting each statement into its context's
// querytally.Tally each time it runs.
type tallying struct {
	inner interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
		PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
		QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
		QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	}
	// kept is a write transaction's statements that return no rows, each prepared once; nil on a
	// read, where each statement is prepared as it runs.
	kept *keptStatements
}

// ExecContext counts and runs a statement that returns no rows.
func (counting tallying) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if tally := querytally.Of(ctx); tally != nil {
		tally.Count(query)
	}
	run := counting.inner.ExecContext
	if counting.kept != nil {
		run = counting.kept.exec
	}
	result, err := run(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w", err) // the generated code's caller says what it was doing
	}
	return result, nil
}

// PrepareContext prepares a statement; it is counted when run.
func (counting tallying) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	statement, err := counting.inner.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w", err) // the generated code's caller says what it was doing
	}
	return statement, nil
}

// QueryContext counts and runs a statement that returns rows.
func (counting tallying) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if tally := querytally.Of(ctx); tally != nil {
		tally.Count(query)
	}
	rows, err := counting.inner.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w", err) // the generated code's caller says what it was doing
	}
	return rows, nil
}

// QueryRowContext counts and runs a statement that returns one row.
func (counting tallying) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if tally := querytally.Of(ctx); tally != nil {
		tally.Count(query)
	}
	return counting.inner.QueryRowContext(ctx, query, args...)
}

// keptStatements is one write transaction's statements that return no rows, each prepared the
// first time the transaction runs its text and run from there after. SQLite compiles a statement
// each time it is prepared, the programs of every trigger it fires included, and for a write to a
// table with many triggers that compile costs more than the run (test-weight.md §34, §36).
//
// Statements that return rows are not kept. The driver resets a kept statement before it runs it
// again, and a Rows of an earlier run may still be open, read by a caller that runs the same query
// inside its loop: the reset would end those rows under the reader. An Exec's statement has run
// to its end and been reset before ExecContext returns, so nothing still reads it.
type keptStatements struct {
	transaction *sql.Tx
	// onKeep is told each statement as it is prepared and kept; nil tells no one.
	onKeep func(query string, statement *sql.Stmt)
	// mutex guards byQuery, for a change that runs its queries from more than one goroutine.
	mutex   sync.Mutex
	byQuery map[string]*sql.Stmt
}

// exec runs query's kept statement, preparing and keeping it the first time.
func (kept *keptStatements) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	statement, err := kept.statement(ctx, query) //nolint:sqlclosecheck // kept for the write; closeAll closes it
	if err != nil {
		return nil, err
	}
	result, err := statement.ExecContext(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("%w", err) // the generated code's caller says what it was doing
	}
	return result, nil
}

// statement is query's kept statement, prepared on the transaction the first time it is asked for.
func (kept *keptStatements) statement(ctx context.Context, query string) (*sql.Stmt, error) {
	kept.mutex.Lock()
	defer kept.mutex.Unlock()
	if statement, isKept := kept.byQuery[query]; isKept {
		return statement, nil
	}
	statement, err := kept.transaction.PrepareContext(ctx, query) //nolint:sqlclosecheck // closeAll closes it
	if err != nil {
		return nil, fmt.Errorf("%w", err) // the generated code's caller says what it was doing
	}
	kept.byQuery[query] = statement
	if kept.onKeep != nil {
		kept.onKeep(query, statement)
	}
	return statement, nil
}

// closeAll closes every kept statement, before the transaction commits or rolls back.
func (kept *keptStatements) closeAll() error {
	kept.mutex.Lock()
	defer kept.mutex.Unlock()
	failures := make([]error, 0, len(kept.byQuery))
	for _, statement := range kept.byQuery {
		failures = append(failures, statement.Close())
	}
	clear(kept.byQuery)
	if err := errors.Join(failures...); err != nil {
		return fmt.Errorf("close the write's kept statements: %w", err)
	}
	return nil
}

// keepingStatements runs change on a connection over transaction that keeps each statement
// returning no rows prepared until change ends, and closes them as it ends, however it ends:
// returning, failing or panicking.
func (database *Database) keepingStatements(transaction *sql.Tx, change func(connection tallying) error) (err error) {
	kept := &keptStatements{transaction: transaction, onKeep: database.onKeep, byQuery: map[string]*sql.Stmt{}}
	defer func() { err = errors.Join(err, kept.closeAll()) }()
	return change(tallying{inner: transaction, kept: kept})
}

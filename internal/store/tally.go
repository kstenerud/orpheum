// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// tallying is the generated queries' connection, counting each statement into its context's
// querytally.Tally.
type tallying struct {
	inner interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
		PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
		QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
		QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	}
}

// ExecContext counts and runs a statement that returns no rows.
func (counting tallying) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if tally := querytally.Of(ctx); tally != nil {
		tally.Count(query)
	}
	result, err := counting.inner.ExecContext(ctx, query, args...)
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

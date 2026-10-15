// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"

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
	return counting.inner.ExecContext(ctx, query, args...) //nolint:wrapcheck // the generated code's caller wraps
}

// PrepareContext prepares a statement; it is counted when run.
func (counting tallying) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return counting.inner.PrepareContext(ctx, query) //nolint:wrapcheck // the generated code's caller wraps
}

// QueryContext counts and runs a statement that returns rows.
func (counting tallying) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if tally := querytally.Of(ctx); tally != nil {
		tally.Count(query)
	}
	return counting.inner.QueryContext(ctx, query, args...) //nolint:wrapcheck // the generated code's caller wraps
}

// QueryRowContext counts and runs a statement that returns one row.
func (counting tallying) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if tally := querytally.Of(ctx); tally != nil {
		tally.Count(query)
	}
	return counting.inner.QueryRowContext(ctx, query, args...)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	_ "embed" // the invariants
	"errors"
	"fmt"
	"regexp"
	"strings"
)

//go:embed invariants.sql
var invariantsSQL string

// invariantHeading starts each invariant in invariants.sql and names it.
var invariantHeading = regexp.MustCompile(`(?m)^-- invariant: (.+)$`)

// violationRowsShown is how many of a violation's rows it names: enough to find them, not a dump.
const violationRowsShown = 10

// Violation is an invariant that does not hold: its name, and the first of the rows where it fails.
type Violation struct {
	Invariant string
	Rows      []string
}

// invariant is one of invariants.sql's queries.
type invariant struct {
	name  string
	query string
}

// invariants are invariants.sql's queries, in its order.
func invariants() []invariant {
	marks := invariantHeading.FindAllStringSubmatchIndex(invariantsSQL, -1)
	found := make([]invariant, 0, len(marks))
	for index, mark := range marks {
		end := len(invariantsSQL)
		if index+1 < len(marks) {
			end = marks[index+1][0]
		}
		found = append(found, invariant{
			name: invariantsSQL[mark[2]:mark[3]], query: strings.TrimSpace(invariantsSQL[mark[1]:end]),
		})
	}
	return found
}

// CheckInvariants says which of persistent.db's invariants do not hold (D303, M7): every query in
// invariants.sql, read in one snapshot; SQLite's foreign-key check; and each full-text index's
// agreement with the text it indexes, which FTS5 checks only by a write, taken through the writer.
func (database *Database) CheckInvariants(ctx context.Context) ([]Violation, error) {
	if database.isClosed.Load() {
		return nil, ErrClosed
	}
	var violations []Violation
	err := readTransaction(ctx, database.readers, func(transaction *sql.Tx) error {
		checks := append(invariants(), invariant{
			name:  "every foreign key names a row that exists",
			query: `SELECT "table" || ' row ' || rowid || ' -> ' || parent FROM pragma_foreign_key_check`,
		})
		for _, check := range checks {
			rows, err := firstColumns(ctx, transaction, check.query)
			if err != nil {
				return fmt.Errorf("check that %s: %w", check.name, err)
			}
			if len(rows) > 0 {
				violations = append(violations, Violation{Invariant: check.name, Rows: rows})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, index := range []struct{ name, check string }{
		{"item_fts", "INSERT INTO item_fts(item_fts, rank) VALUES ('integrity-check', 1)"},
		{"item_fts_tri", "INSERT INTO item_fts_tri(item_fts_tri, rank) VALUES ('integrity-check', 1)"},
	} {
		err := database.transact(ctx, func(transaction *sql.Tx) error {
			_, checkErr := transaction.ExecContext(ctx, index.check)
			return checkErr //nolint:wrapcheck // the violation below names the index
		})
		if err != nil {
			violations = append(violations, Violation{
				Invariant: index.name + " agrees with the text it indexes", Rows: []string{err.Error()},
			})
		}
	}
	return violations, nil
}

// firstColumns are the first column of the first violationRowsShown rows query returns, as text.
func firstColumns(ctx context.Context, transaction *sql.Tx, query string) (found []string, err error) {
	rows, err := transaction.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}
	for rows.Next() && len(found) < violationRowsShown {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		found = append(found, fmt.Sprint(values[0]))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return found, nil
}

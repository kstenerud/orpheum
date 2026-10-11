// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/errs"
)

// checkStep makes the checks a SQL file cannot make of itself: no dangling foreign key, no
// AUTOINCREMENT sequence moved backwards (an id reissued), every view still compiles.
func checkStep(ctx context.Context, connection *sql.Conn, sequencesBefore map[string]int64) error {
	err := checkForeignKeys(ctx, connection)
	if err != nil {
		return err
	}
	sequencesAfter, err := readSequences(ctx, connection)
	if err != nil {
		return err
	}
	for table, before := range sequencesBefore {
		// A table the step dropped or renamed away has no row now and can reissue nothing; a
		// rebuild that renames its copy into place is compared under the original name.
		if after, exists := sequencesAfter[table]; exists && after < before {
			return fmt.Errorf("the id sequence of %s went down from %d to %d; ids are never reissued (D173)",
				table, before, after)
		}
	}
	return checkViewsCompile(ctx, connection)
}

func checkForeignKeys(ctx context.Context, connection *sql.Conn) (err error) {
	rows, err := connection.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close foreign_key_check", rows.Close())) }()
	if rows.Next() {
		var table, parent string
		var rowID sql.NullInt64
		var constraint int64
		err = rows.Scan(&table, &rowID, &parent, &constraint)
		if err != nil {
			return fmt.Errorf("read foreign_key_check: %w", err)
		}
		return fmt.Errorf("foreign_key_check: a row of %s (rowid %d) references a missing row of %s",
			table, rowID.Int64, parent)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	return nil
}

// readSequences returns sqlite_sequence as a map; it is empty before any AUTOINCREMENT table.
func readSequences(ctx context.Context, connection *sql.Conn) (sequences map[string]int64, err error) {
	sequences = map[string]int64{}
	var sqliteSequenceCount int64
	const findTable = "SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'sqlite_sequence'"
	err = connection.QueryRowContext(ctx, findTable).Scan(&sqliteSequenceCount)
	if err != nil {
		return nil, fmt.Errorf("look for sqlite_sequence: %w", err)
	}
	if sqliteSequenceCount == 0 {
		return sequences, nil
	}
	rows, err := connection.QueryContext(ctx, "SELECT name, seq FROM sqlite_sequence")
	if err != nil {
		return nil, fmt.Errorf("read sqlite_sequence: %w", err)
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close sqlite_sequence", rows.Close())) }()
	for rows.Next() {
		var table string
		var sequence int64
		err = rows.Scan(&table, &sequence)
		if err != nil {
			return nil, fmt.Errorf("read sqlite_sequence: %w", err)
		}
		sequences[table] = sequence
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sqlite_sequence: %w", err)
	}
	return sequences, nil
}

// checkViewsCompile asks for each view's columns, which makes SQLite compile its definition; a
// view over a dropped or renamed table fails here rather than at the first read.
func checkViewsCompile(ctx context.Context, connection *sql.Conn) error {
	views, err := viewNames(ctx, connection)
	if err != nil {
		return err
	}
	for _, view := range views {
		var columns int64
		err = connection.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info(?)", view).Scan(&columns)
		if err != nil {
			return fmt.Errorf("view %s no longer compiles: %w", view, err)
		}
		if columns == 0 {
			return fmt.Errorf("view %s no longer compiles: it has no columns", view)
		}
	}
	return nil
}

func viewNames(ctx context.Context, connection *sql.Conn) (views []string, err error) {
	rows, err := connection.QueryContext(ctx, "SELECT name FROM sqlite_schema WHERE type = 'view' ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list views: %w", err)
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the view list", rows.Close())) }()
	for rows.Next() {
		var view string
		err = rows.Scan(&view)
		if err != nil {
			return nil, fmt.Errorf("list views: %w", err)
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list views: %w", err)
	}
	return views, nil
}

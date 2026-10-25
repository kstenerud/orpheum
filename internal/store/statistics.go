// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"bufio"
	"context"
	"database/sql"
	_ "embed" // the committed statistics
	"fmt"
	"slices"
	"strings"
)

// persistentStatistics are persistent.db's planner statistics, what ANALYZE finds of the reference
// library (performance-targets.md), one "table<TAB>index<TAB>stat" line each, sorted: committed so
// the plans tests read are the plans every server runs, whatever its own library holds, and so
// nothing ever runs ANALYZE on a household's server (D303 §3, DL15). TestReferenceScale writes it
// again with -update-statistics.
//
//go:embed statistics/persistent.tsv
var persistentStatistics string

// statisticsRow is one row of sqlite_stat1.
type statisticsRow struct {
	table, index, stat string
}

// parseStatistics reads a statistics file's rows.
func parseStatistics(text string) ([]statisticsRow, error) {
	var rows []statisticsRow
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("a statistics line has %d fields, want 3: %q", len(fields), line)
		}
		rows = append(rows, statisticsRow{table: fields[0], index: fields[1], stat: fields[2]})
	}
	return rows, scanner.Err() //nolint:wrapcheck // a string's scanner fails only on a too-long line
}

// loadStatistics makes sqlite_stat1 the committed statistics, when it is not already, and has every
// connection plan by them: SQLite reloads its statistics on ANALYZE sqlite_schema, on the connection
// that runs it, and on each connection when it opens, so the idle readers are closed (lang_analyze
// §2.2, "Fixed results of ANALYZE"). Samples (sqlite_stat4) are dropped: they describe one
// library's values.
func (database *Database) loadStatistics(ctx context.Context, text string) error {
	want, err := parseStatistics(text)
	if err != nil {
		return err
	}
	have, err := database.storedStatistics(ctx)
	if err != nil {
		return err
	}
	if slices.Equal(have, want) {
		return nil
	}
	err = database.transact(ctx, func(transaction *sql.Tx) error { return replaceStatistics(ctx, transaction, want) })
	if err != nil {
		return fmt.Errorf("load the planner's statistics: %w", err)
	}
	for _, pool := range []*sql.DB{database.readers, database.maintenance} {
		pool.SetMaxIdleConns(0) // the idle connections close, and the next open loads the statistics
		pool.SetMaxIdleConns(pool.Stats().MaxOpenConnections)
	}
	return nil
}

// replaceStatistics makes sqlite_stat1 rows, drops the samples, and reloads them on this connection.
func replaceStatistics(ctx context.Context, transaction *sql.Tx, rows []statisticsRow) error {
	emptied := []string{"ANALYZE sqlite_schema", "DELETE FROM sqlite_stat1", "DELETE FROM sqlite_stat4"}
	for _, statement := range emptied {
		if _, execErr := transaction.ExecContext(ctx, statement); execErr != nil {
			return fmt.Errorf("%s: %w", statement, execErr)
		}
	}
	for _, row := range rows {
		if _, execErr := transaction.ExecContext(ctx, "INSERT INTO sqlite_stat1 (tbl, idx, stat) VALUES (?, ?, ?)",
			row.table, row.index, row.stat); execErr != nil {
			return fmt.Errorf("keep the statistics of %s: %w", row.index, execErr)
		}
	}
	if _, execErr := transaction.ExecContext(ctx, "ANALYZE sqlite_schema"); execErr != nil {
		return fmt.Errorf("reload the statistics: %w", execErr)
	}
	return nil
}

// storedStatistics are sqlite_stat1's rows, sorted as a statistics file is; none before any.
func (database *Database) storedStatistics(ctx context.Context) ([]statisticsRow, error) {
	var exists int
	err := database.readers.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_schema WHERE name = 'sqlite_stat1'").Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("look for the statistics: %w", err)
	}
	if exists == 0 {
		return nil, nil
	}
	rows, err := database.readers.QueryContext(ctx, "SELECT tbl, idx, stat FROM sqlite_stat1 ORDER BY tbl, idx")
	if err != nil {
		return nil, fmt.Errorf("read the statistics: %w", err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // a read; rows.Err is checked
	var stored []statisticsRow
	for rows.Next() {
		var row statisticsRow
		var index sql.NullString
		if err := rows.Scan(&row.table, &index, &row.stat); err != nil {
			return nil, fmt.Errorf("read the statistics: %w", err)
		}
		row.index = index.String
		stored = append(stored, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the statistics: %w", err)
	}
	return stored, nil
}

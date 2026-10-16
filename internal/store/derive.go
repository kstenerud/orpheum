// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// WriteUnlessUnchanged runs derive first in a read transaction, where SQLite refuses any write
// (query_only), and only when it tried to write runs it again in a write transaction: a derivation
// that finds everything already as it derives it never takes the writer (D303, DL9, DL11). The
// derivation must compare before it writes, so that an unchanged run writes nothing, and must keep nothing of
// a run but what it returns, since it may run twice. It says whether the write ran.
func (database *Database) WriteUnlessUnchanged(
	ctx context.Context, derive func(queries *sqlcgen.Queries) error,
) (bool, error) {
	wouldWrite, err := database.WouldWrite(ctx, derive)
	if !wouldWrite {
		return false, err
	}
	return true, database.Write(ctx, derive)
}

// WouldWrite runs derive in a read transaction, where SQLite refuses any write (query_only), and
// says whether it tried to write: a derivation that finds everything as it derives it never takes
// the writer, and one that would write can then write as it chooses, in batches (D304 §2). Its run
// stops at the first write it tries, so it must keep nothing of the run.
func (database *Database) WouldWrite(ctx context.Context, derive func(queries *sqlcgen.Queries) error) (bool, error) {
	err := readTransaction(ctx, database.readers, func(transaction *sql.Tx) error {
		return derive(sqlcgen.New(tallying{transaction}))
	})
	if isRefusedWrite(err) {
		return true, nil
	}
	return false, err
}

// isRefusedWrite is whether err is SQLite refusing a write on a query_only connection.
func isRefusedWrite(err error) bool {
	failure, isSQLite := errors.AsType[*sqlite.Error](err)
	return isSQLite && failure.Code()&0xff == sqlite3.SQLITE_READONLY
}

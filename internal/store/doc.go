// SPDX-License-Identifier: GPL-3.0-or-later

// Package store is the database: the one Open() for library.db and cache.db with its pragmas read
// back at startup, the single writer connection and the reader pool, our migration runner, the
// sqlc-generated queries in sqlcgen, and the one hand-written filter builder for search. It knows
// tables, not features.
//
// # Invariants
//
//   - It is the only package that imports database/sql or the SQLite driver. Enforced by the
//     depguard rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - It imports nothing from internal/core or internal/node. Enforced by the depguard rule store-
//     is-below-core.
//   - SQL is never built by string concatenation. Enforced by gosec (G201, G202).
//   - Every query names its columns; no SELECT * or RETURNING *. Enforced by
//     TestArch_QueriesNameEveryColumn.
//   - Rows are closed and their errors checked. Enforced by sqlclosecheck and rowserrcheck.
//   - Generated code in sqlcgen is never edited by hand. Enforced by make check-generated and the
//     pre-edit hook.
//
// # Read before changing
//
//   - D6: sqlc, no ORM.
//   - D31: SQLite, pure-Go driver, WAL, pragmas read back (as refined by D193).
//   - D181: the migration policy and our small runner.
//   - D185: the schema as SQL that runs.
//   - D173: tombstones, not deletes; live_* views.
//   - docs/contributors/design/plans/schema.md, docs/contributors/design/plans/migrations.md.
//   - docs/contributors/standards/sql.md.
//
// # Where to change
//
//   - migrations/: append-only numbered SQL files.
//   - queries/: one .sql file per concern; run make generate.
//   - The Open function, the writer, the runner and the search filter builder live in this
//     package's own files.
package store

// SPDX-License-Identifier: AGPL-3.0-or-later

// Package storetest opens real, migrated databases, and cache.db, for other packages' tests. It
// is imported only by tests; production code opens the database once, in cmd/orpheum.
//
// # Invariants
//
//   - Every database it opens is a real file under the test's temporary directory, so a test sees
//     the production schema: a copy of one persistent.db the real runner migrated once per test
//     process, which the runner is then asked to migrate again and finds current. Review-only.
//   - It goes through internal/store like everyone else, except ExecRaw, which opens a file with
//     database/sql to build a state the store never produces (a newer release's file, a foreign
//     one). It may, being under internal/store: the depguard rule sql-only-in-store and
//     TestArch_OnlyStoreImportsDatabaseSQL allow database/sql nowhere else.
//   - A database OpenMigrated opens fails the test whose request that only reads wrote to it,
//     unless readerWritesAllowed lists the writer with why (D303, DL10). Enforced by
//     TestOpenMigrated_FailsATestWhoseReadingRequestWrote.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - docs/contributors/standards/testing.md §12: tests write only under t.TempDir().
//   - D181: the runner the database is migrated with.
//
// # Where to change
//
//   - storetest.go: OpenMigrated and the migrated template it copies, OpenCache, the options
//     tests open and migrate with (OpenOptions, MigrateOptions), ExecRaw, and helpers that seed
//     rows every caller needs (InsertUser).
package storetest

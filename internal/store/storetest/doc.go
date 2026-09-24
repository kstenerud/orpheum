// SPDX-License-Identifier: GPL-3.0-or-later

// Package storetest opens real, migrated databases for other packages' tests. It is imported only
// by tests; production code opens the database once, in cmd/orpheum.
//
// # Invariants
//
//   - Every database it opens is a real file under the test's temporary directory, migrated by
//     the real runner, so a test sees the production schema. Review-only.
//   - It imports no database/sql; it goes through internal/store like everyone else. Enforced by
//     the depguard rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - docs/contributors/standards/testing.md §12: tests write only under t.TempDir().
//   - D181: the runner the database is migrated with.
//
// # Where to change
//
//   - storetest.go: Open, and helpers that seed rows every caller needs (a user).
package storetest

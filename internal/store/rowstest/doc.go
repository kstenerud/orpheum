// SPDX-License-Identifier: AGPL-3.0-or-later

// Package rowstest builds the catalog's rows for internal/store's own tests: each builder takes a
// spec whose zero fields default, so a test names only what it cares about, and returns the SQL
// that inserts the row, which the test runs with its own execRaw or storetest.ExecRaw. It is the
// store's counterpart of internal/core/library/librarytest, which builds the same tables through
// the store's queries and so cannot serve a test inside package store (an import cycle) nor any
// test under internal/store (the depguard rule store-is-below-core). It is imported only by tests.
//
// # Invariants
//
//   - It imports nothing from internal/store, so package store's own tests can use it with no
//     import cycle. Enforced by the compiler: those tests import it.
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Every id is the test's to choose, or the database's when it is zero; nothing else need be
//     named. Review-only.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - internal/core/library/librarytest: the builders' shape, which these follow.
//
// # Where to change
//
//   - rows.go: Node, Library (and its root), Item, Title, Series, Ordering, Position, Version,
//     File (with its place and filing), Place, Probe and Stream, and Join, for a test's own builder
//     of several rows; a schema rename to one of their columns is made once, here, for the store's
//     tests.
package rowstest

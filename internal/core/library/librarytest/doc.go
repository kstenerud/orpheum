// SPDX-License-Identifier: GPL-3.0-or-later

// Package librarytest gives the tests that feed a library scans the fixture they share: an
// enrolled node with one library over one root, written through the store's queries, and the
// numbered envelope a node sends a scan in. It is imported only by tests.
//
// The same fixture is written two other ways on purpose. API tests that arrange a whole catalog
// with fixed ids write it as raw SQL beside the rest of that catalog (storetest.ExecRaw), and
// cmd/orpheum's tests make it through nodes.Enrol and library.Create, the path serve's database
// is made by. This package is the level between: the library's and identification's own tests.
//
// # Invariants
//
//   - Imports nothing from internal/core/library, so library's in-package tests can use it with
//     no import cycle. Review-only.
//   - Only tests import it. Review-only: no gate checks it yet, as TestArch_TestutilIsImportedOnlyByTests
//     does for internal/testutil.
//   - Imports nothing from internal/node, never os/exec. Enforced by the depguard rule
//     core-boundary.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - internal/nodeproto: the scan records and their report sequence.
//
// # Where to change
//
//   - fixture.go: NodeWithLibrary, and Reports, which numbers a scan's records.
package librarytest

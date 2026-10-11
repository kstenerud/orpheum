// SPDX-License-Identifier: AGPL-3.0-or-later

// Package librarytest gives tests the fixture they share: an enrolled node, a library over a root
// on it, written through the store's queries, and the numbered envelope a node sends a scan in. It
// is imported only by tests. A test that arranges a whole catalog makes its node and library here
// and writes the rest beside them; cmd/orpheum's tests make them through nodes.Enrol and
// library.Create instead, the path serve's database is made by.
//
// # Invariants
//
//   - Imports nothing from internal/core/library, so library's in-package tests can use it with
//     no import cycle. Review-only.
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
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
//   - fixture.go: Node, LibraryOn and NodeWithLibrary, and Scan, which frames a scan's records.
package librarytest

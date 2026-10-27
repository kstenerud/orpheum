// SPDX-License-Identifier: AGPL-3.0-or-later

// Package librarytest gives tests the fixture they share: an enrolled node, a library over a root
// on it, the catalog rows a scan would file under it (item, title, version, file, file_probe,
// stream), written through the store's queries, and the numbered envelope a node sends a scan in.
// It is imported only by tests. cmd/orpheum's tests make a node and a library through
// nodes.Enrol and library.Create instead, the path serve's database is made by.
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
//   - catalog.go: Item, Title, Version, File, Probe and Stream, the builders for the catalog's own
//     tables; a schema rename to one of their columns is made once, here.
//   - watching.go: Watch, a person's position in a file as a play report leaves it, the file's
//     titles watched once it is finished (D370).
package librarytest

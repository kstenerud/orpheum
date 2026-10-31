// SPDX-License-Identifier: AGPL-3.0-or-later

// Package trusttest gives other packages' tests a discovery's root key, made afresh in software, and
// what it signs: its root key set's statement, address documents and release records. It is imported
// only by tests, the paid service's among them.
//
// # Invariants
//
//   - It signs through trust.Sign, the code the paid service's online key signs with, so a test sees
//     what a core sees. Review.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//
// # Where to change
//
//   - trusttest.go: Root and what it signs.
package trusttest

// SPDX-License-Identifier: AGPL-3.0-or-later

// Package trusttest gives other packages' tests root chains made afresh in software (D393): a discovery's
// or a publisher's genesis, its everyday and recovery keys, hardware keys with and without their PIN,
// and what those keys sign, address documents and release records among them. It is imported only by
// tests, the paid service's among them.
//
// # Invariants
//
//   - It signs through trust.Sign, in the SSH signature format the owner's `ssh-keygen -Y sign` writes,
//     so a test sees what a core sees. Review.
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

// SPDX-License-Identifier: GPL-3.0-or-later

// Package nodetest gives the node's tests what they share: a media root opened for a test, and the
// payload hash of a file's contents, as a command names the file. It is imported only by tests.
//
// # Invariants
//
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - D100 and D179: a command names a file by its payload hash.
//
// # Where to change
//
//   - nodetest.go.
package nodetest

// SPDX-License-Identifier: AGPL-3.0-or-later

// Package containertest builds small container files for tests elsewhere in the node: a Matroska
// file whose picture's keyframes are in its cues (D259 §2). It is imported only by tests.
//
// # Invariants
//
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - D259 §2: a film's keyframes from its file's own index.
//
// # Where to change
//
//   - matroska.go.
package containertest

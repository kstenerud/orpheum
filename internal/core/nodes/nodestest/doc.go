// SPDX-License-Identifier: AGPL-3.0-or-later

// Package nodestest gives the tests of packages whose jobs a node does Resolver, which hands a test
// the answer its node's word resolves a job with. It is imported only by tests.
//
// # Invariants
//
//   - A job a test started by its handler alone (Start) is answered to the test, with the job as
//     the test gave it; any other is resolved by the fallback, a real runner. Review-only.
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from internal/node, never os/exec. Enforced by the depguard rule
//     core-boundary.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - internal/core/nodes/doc.go: how a job given to a node is resolved (D234 §7).
//
// # Where to change
//
//   - resolver.go: Resolver.
package nodestest

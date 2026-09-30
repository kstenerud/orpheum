// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobstest gives the tests of packages that queue work Queue, the job queue as a list of
// what was asked of it. It is imported only by tests.
//
// # Invariants
//
//   - Queue.Enqueue lists no second job of a kind for a target while one is listed and says it
//     added none, as the real queue merges a repeat request into the open job
//     (TestEnqueue_QueuesATargetOnceWhileItIsOpen); it does not merge priorities or arguments. A
//     test that wants the job taken off the list sets the list to nil. Review-only.
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from internal/node, never os/exec. Enforced by the depguard rule
//     core-boundary.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - internal/core/jobs/doc.go: what the real queue promises.
//
// # Where to change
//
//   - queue.go: Queue.
package jobstest

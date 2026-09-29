// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobstest gives the tests of packages that queue work Queue, the job queue as a list of
// what was asked of it. It is imported only by tests.
//
// # Invariants
//
//   - Queue.Enqueue refuses a second job of a kind for a target while one is listed, as the
//     real queue does for an open job (TestEnqueue_QueuesATargetOnceWhileItIsOpen); a test that
//     wants the job taken off the list sets the list to nil. Review-only.
//   - Only tests import it. Review-only: no gate checks it yet, as TestArch_TestutilIsImportedOnlyByTests
//     does for internal/testutil.
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

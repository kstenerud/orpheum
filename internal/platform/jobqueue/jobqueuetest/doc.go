// SPDX-License-Identifier: AGPL-3.0-or-later

// Package jobqueuetest gives tests of internal/platform/jobqueue, and of the programs built on it,
// Memory, a job storage held in memory, and Random, a random source whose jitter a test knows. It
// is imported only by tests.
//
// # Invariants
//
//   - Memory keeps the promises of jobqueue.Storage as internal/store's storage keeps them on
//     cache.db: the same claim order, attempt counting, and a cancelled job that stays cancelled.
//     The jobqueue tests run against Memory (TestClaim_TakesTheHighestPriorityThenTheEarliestThenTheOldest
//     and the rest); internal/store's tests check the same promises on cache.db
//     (TestJobStorage_ClaimsInOrderAndCountsTheAttempt,
//     TestJobStorage_ShowsThePlanTheOpenJobsAndMergesOnlyIntoAQueuedOne,
//     TestJobStorage_RecordsAWaitWithItsFlagsAndNeverUndoesACancel,
//     TestJobStorage_RequeuesWithTheAttemptCountedAndKnowsATargetByItsColumn). Review-only beyond
//     that.
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from the module but internal/platform. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - internal/platform/jobqueue/doc.go: what the storage promises.
//
// # Where to change
//
//   - memory.go: Memory. random.go: Random and its named sources.
package jobqueuetest

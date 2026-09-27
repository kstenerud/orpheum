// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobs is the one work queue: fetches, prepares, scans and backups are rows in cache.db's
// job table. One scheduler picks the highest-priority queued job whose runnable_at has passed and
// whose runner has a free slot. Node jobs become commands; their progress arrives as job records.
// After a crash, running jobs return to queued and restart from zero.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Every switch over a job kind or state names every value. Enforced by exhaustive.
//   - Time comes from a clock.Clock, so scheduling is testable. Enforced by the forbidigo rule
//     tagged [clock].
//   - A subject has at most one open job of a kind. TestEnqueue_QueuesASubjectOnceWhileItIsOpen.
//   - The highest priority runs first. TestRunDue_RunsTheHighestPriorityFirst.
//   - Several runners of one kind run its jobs at once, each job once.
//     TestRun_SeveralRunnersOfOneKindRunItsJobsAtOnceEachOnce.
//   - A queued job wakes a runner of its own kind at once; no other kind's runner takes its
//     wake-up, so it never waits for the next tick. TestRun_AQueuedJobWakesARunnerOfItsOwnKindAtOnce.
//   - A failure retries with doubling back-off up to the retry limit, then fails.
//     TestRunDue_RetriesAFailureWithDoublingBackoffThenFails.
//   - A provider's Retry-After, or a missing key, holds the kind without costing attempts (D170).
//     TestRunDue_ARetryAfterWaitsWithoutCostingAnAttemptAndHoldsTheKind,
//     TestRunDue_ABlockedKindWaitsTheBackoffWithoutCostingAttempts.
//   - After a crash, running jobs return to queued. TestRequeueRunning_ReturnsWhatACrashLeftRunning.
//   - A cancelled queued job never runs; a running one's handler is told through its context,
//     whose cause is ErrCancelled; either stays cancelled whatever its handler answers after.
//     TestCancel_AQueuedJobNeverRunsAndARunningOnesHandlerIsToldAndItStaysCancelled.
//   - A kind may retry on its own schedule instead (D213 §5), and a job may wait until a time
//     before it first runs. TestRunDue_AKindWithItsOwnScheduleRetriesOnItThenFails,
//     TestEnqueue_AJobWaitsUntilItsNotBefore.
//
// # Read before changing
//
//   - D193 §2: scheduling, one encode per node.
//   - D118: the scan is incremental, prioritised and steerable.
//   - D170: provider rate limits and back-off.
//   - docs/contributors/design/plans/schema.md, cache.db: the job table, one queue.
//   - docs/contributors/design/plans/cut-a.md §5.3.
//
// # Where to change
//
//   - queue.go: queuing, and the answers a handler gives. runner.go: running one kind, one job at
//     a time per runner (a kind may have several, as prepares do), and recording each outcome.
//     view.go: the job view, progress and cancel. Each kind's handler and runner count are
//     cmd/orpheum's (metadata.go); scan and backup jobs come in M4 (D223 §6).
package jobs

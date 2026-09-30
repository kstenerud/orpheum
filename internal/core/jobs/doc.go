// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobs is Orpheum's use of the generic queue in internal/platform/jobqueue: its kinds
// (fetches, artwork, prepares, subtitles, loudness; scans and backups come in M4), their targets,
// the answers its handlers give, and the job view. The rows are cache.db's job table, kept by
// internal/store's JobStorage. One scheduler per kind picks the highest-priority queued job whose
// runnable_at has passed; node jobs become commands, each answered on its own path to the node
// service. After a stop, running jobs return to queued and restart from zero.
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
//   - A job names exactly one target. TestEnqueue_QueuesATargetOnceWhileItIsOpen reads it
//     through the queue; ErrNotOneTarget refuses the rest.
//   - A target has at most one open job of a kind, and a repeat request merges into it
//     (internal/platform/jobqueue says how). TestEnqueue_QueuesATargetOnceWhileItIsOpen;
//     a fetch's arguments merge as identify.FetchArgsMerge says, wired in cmd/orpheum:
//     TestScenario_S71_APersonsRefreshBehindARollingOneRunsAtTheirPriorityAndAsksForTheImagesAgain.
//   - The highest priority runs first. TestRunDue_RunsTheHighestPriorityFirst.
//   - Several runners of one kind run its jobs at once, each job once.
//     TestRun_SeveralRunnersOfOneKindRunItsJobsAtOnceEachOnce.
//   - A queued job wakes a runner of its own kind at once; no other kind's runner takes its
//     wake-up, so it never waits for the next tick. TestRun_AQueuedJobWakesARunnerOfItsOwnKindAtOnce.
//   - A failure retries with doubling back-off up to job_attempt_limit, then fails.
//     TestRunDue_RetriesAFailureWithDoublingBackoffThenFails. The doubling stops at a day, so a
//     job that keeps failing retries daily until the limit, and never overflows whatever the
//     limit allows. TestRunDue_TheBackoffStopsAtADayThenRetriesDailyUntilTheLimit,
//     TestRunDue_TheBackoffNeverShrinksHoweverManyAttemptsTheLimitAllows. Each wait is jittered
//     by up to a tenth (D231 §10); these tests run unjittered.
//   - A provider's Retry-After, or a missing key, holds the kind without costing attempts (D170).
//     TestRunDue_ARetryAfterWaitsWithoutCostingAnAttemptAndHoldsTheKind,
//     TestRunDue_ABlockedKindWaitsTheBackoffWithoutCostingAttempts.
//   - A tool that stalled (MarkStalled) costs no attempt, and the next attempt is told the last
//     one's tool failed. TestRunDue_AStallCostsNoAttemptAndTheNextAttemptKnowsTheLastOneStalled.
//   - After a stop, running jobs return to queued, their attempt counted; one that stops the
//     server every time fails at its limit, and a handler's panic fails only its own job, the
//     second time. TestRequeueRunning_ReturnsWhatACrashLeftRunning,
//     TestScenario_S86_AJobThatBringsTheServerDownEveryTimeFailsAtItsLimitAndAPanicFailsOnlyItsJob.
//   - A cancelled queued job never runs; a running one's handler is told through its context,
//     whose cause is ErrCancelled; either stays cancelled whatever its handler answers after.
//     TestCancel_AQueuedJobNeverRunsAndARunningOnesHandlerIsToldAndItStaysCancelled.
//   - A kind may retry on its own schedule instead (D213 §5), and a job may wait until a time
//     before it first runs. TestRunDue_AKindWithItsOwnScheduleRetriesOnItThenFails,
//     TestEnqueue_AJobWaitsUntilItsRunnableAt.
//
// # Read before changing
//
//   - internal/platform/jobqueue/doc.go: what the queue itself promises.
//   - D231, D233: the queue's rebuild; docs/contributors/design/plans/jobs-rebuild.md.
//   - D193 §2: scheduling, one encode per node.
//   - D118: the scan is incremental, prioritised and steerable.
//   - D170: provider rate limits and back-off.
//   - docs/contributors/design/plans/schema.md, cache.db: the job table, one queue.
//   - docs/contributors/design/plans/cut-a.md §5.3.
//
// # Where to change
//
//   - queue.go: the kinds, targets, requests, the queue's parts and the answers a handler gives.
//     runner.go: a kind's runner, handing each claimed job to its handler as a Job. view.go: the
//     job view, progress and cancel. Each kind's handler, runner count and argument merge are
//     cmd/orpheum's (metadata.go); scan and backup jobs come in M4 (D223 §6).
package jobs

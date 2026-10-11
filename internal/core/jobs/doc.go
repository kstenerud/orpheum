// SPDX-License-Identifier: AGPL-3.0-or-later

// Package jobs is Orpheum's use of the generic queue in internal/platform/jobqueue: its kinds
// (fetches, artwork, prepares, subtitles, loudness, a scan's listing and examining, backups), their
// targets,
// the answers its handlers give, the sweeps that derive their work, and the job view. The rows are
// cache.db's job table, kept by internal/store's JobStorage, a cache of the work persistent.db
// implies (D231 §3): each kind's work is derived again by its sweeps, or asked for again by the
// next play. One scheduler per kind picks the highest-priority queued job whose runnable_at has
// passed; node jobs are given to their node as commands, and the node's word on each comes on one
// path (D234 §2, §7). After a
// stop, running jobs return to queued and restart from zero.
//
// # Invariants
//
//   - A job a person asks for to play is wanted for play_priority_kept_seconds from the last ask; past it,
//     it falls to background priority, and one given to a node says so, for its node to be told
//     (D234 §3, S87). TestDemoteUnwanted_APlaysJobFallsToBackgroundOnceNoOneWaits.
//   - The job view shows a job given to a node as running, with where its node last said it is:
//     waiting, running or paused (D240 §6).
//     TestList_AJobGivenToANodeRunsThereAndSaysWhereItsNodeSaysItIs.
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Every switch over a job kind or state names every value. Enforced by exhaustive.
//   - Time comes from a clock.Clock, so scheduling is testable. Enforced by the forbidigo rule
//     tagged [clock].
//   - A job names exactly one target, but a backup, which names none (D243 §11).
//     TestEnqueue_QueuesATargetOnceWhileItIsOpen reads it
//     through the queue; ErrNotOneTarget refuses the rest.
//   - A target has at most one open job of a kind, and a repeat request merges into it
//     (internal/platform/jobqueue says how). TestEnqueue_QueuesATargetOnceWhileItIsOpen;
//     a fetch's arguments merge as identify.FetchArgsMerge says, wired in cmd/orpheum:
//     TestScenario_S71_APersonsRefreshBehindARollingOneRunsAtTheirPriorityAndAsksForTheImagesAgain.
//     A repeat request raises a queued job, wakes one waiting to retry, looks at once at a parked
//     one's destination, and follows a running one once if the run does not cover it.
//     TestScenario_S71_ARepeatRequestRaisesWakesAndMerges.
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
//   - A job whose destination is down parks on it at no cost of an attempt; no job kept with a
//     parked destination is claimed; it resumes when the destination is unparked; and the job
//     view says what it waits for, why, since when and when it is looked at next (D231 §5, §6,
//     §12). TestRunDue_AJobParkedOnItsDestinationWaitsOnCacheDBAtNoCostAndTheViewSaysWhy; the
//     queue's own promises are internal/platform/jobqueue's.
//   - The destinations are named here, one vocabulary: the outside hosts, each node, each root or
//     mount a scan found missing, a node's prepared directory and the core's storage.
//     TestDestinations_ANameSaysWhatItIsAndReadsAsAPersonSaysIt.
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
//   - Every kind's work is derived by a sweep from persistent.db, or asked for by a play, and each
//     kind whose item can fail records the failure as a fact there (D231 §3, §4, §10); the server
//     registers them (cmd/orpheum).
//     TestRunners_EveryKindsWorkIsDerivedByASweepOrAskedForByAPlayAndItsOwnFailureIsAFact. So
//     cache.db can be discarded at any moment, losing only timing: the matches owed, a person's
//     refresh and a recheck are derived again, and a failure of the item's own is not retried.
//     TestScenario_S78_DiscardingCacheDBLosesOnlyTiming (cmd/orpheum).
//   - A retire cancels the queued, parked and running work of what it took out, by target or by
//     the destination it waits on, and forgets those destinations (D231 §7, §9).
//     TestCancelRetired_CancelsTheWorkOfWhatWasRetiredByTargetOrDestinationAndForgetsTheDestinations.
//   - Nothing reads job history as a fact: only the queue, its storage and the job view read the
//     job rows. TestArch_NothingButTheQueueAndTheJobViewReadsJobHistory.
//
// # Read before changing
//
//   - internal/platform/jobqueue/doc.go: what the queue itself promises.
//   - D231, D233: the queue's rebuild; docs/contributors/design/plans/jobs-rebuild.md.
//   - D193 §2: scheduling, one encode per node.
//   - D118: the scan is incremental, prioritised and steerable.
//   - D170: provider rate limits and back-off.
//   - D231 §5, §6, §12: destinations, parking, and what the job view says of parked work.
//   - docs/contributors/design/plans/schema.md, cache.db: the job table, one queue.
//   - docs/contributors/design/plans/cut-a.md §5.3.
//
// # Where to change
//
//   - queue.go: the kinds, targets, requests, the queue's parts and the answers a handler gives.
//     destination.go: the destinations, parking on them and their state. sweep.go: the sweeps,
//     and cancelling what is retired.
//     runner.go: a kind's runner, handing each claimed job to its handler as a Job. view.go: the
//     job view, progress and cancel. readiness.go: how far work a person waits for is. Each
//     kind's handler, runner count and argument merge are cmd/orpheum's (metadata.go).
package jobs

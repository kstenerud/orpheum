// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobqueue is a job queue that knows no program's kinds, targets or destinations (D231
// §2): a kind is a name, a target a comparable value it only compares, and its storage an
// interface, which internal/store implements on cache.db so that only store touches SQLite. It
// claims by priority, merges a repeat request into the job already open for its kind and target,
// retries with jittered back-off, snoozes without spending an attempt, cancels, returns what a
// stop left running, refuses a job past its attempt limit, and recovers from a panic in a
// handler. internal/core/jobs is Orpheum's use of it: the kinds, their targets and handlers.
//
// # Invariants
//
//   - A claim takes the highest priority whose time has come, then the earliest time, then the
//     oldest, and only of its runner's kind.
//     TestClaim_TakesTheHighestPriorityThenTheEarliestThenTheOldest.
//   - A target has at most one queued and one running job of a kind (D231 §8). A repeat request
//     while one is queued merges into it: the higher priority wins, with its reason, and the
//     arguments merge as the kind's ArgsMerge says (the queued job's are kept by default).
//     TestEnqueue_ARepeatRequestWhileQueuedTakesTheHigherPriorityAndMergesTheArguments.
//   - A repeat request wakes a job waiting on its start or its back-off, to the soonest time asked
//     for, but never brings a snoozed job forward.
//     TestEnqueue_ARepeatRequestWakesAJobWaitingOnItsBackoffButNotASnoozedOne.
//   - While a job runs, a request it covers (by default, the same arguments) is dropped, and
//     another is queued once to follow it.
//     TestEnqueue_WhileAJobRunsACoveredRequestIsDroppedAndAnotherQueuedOnceToFollow.
//   - A job's own failure retries after the first retry delay, doubling, each wait jittered by up
//     to a tenth either way (D231 §10), until the attempt limit, then fails.
//     TestRunDue_RetriesAFailureWithJitteredDoublingBackoffThenFails.
//   - No wait, jittered or scheduled, passes a day. TestRunDue_NoJitterTakesAWaitPastADay.
//   - A snooze costs no attempt, a repeat request does not shorten it, and the runner goes on
//     with other jobs. TestRunDue_ASnoozeCostsNoAttemptAndTheRunnerGoesOn. A RetryAtError and
//     Blocked also hold the runner: TestRunDue_ARetryAfterWaitsWithoutCostingAnAttemptAndHoldsTheKind,
//     TestRunDue_ABlockedKindWaitsTheBackoffWithoutCostingAttempts (internal/core/jobs, on cache.db).
//   - A tool's failure costs no attempt and the next attempt is told of it.
//     TestRunDue_AStallCostsNoAttemptAndTheNextAttemptKnowsTheLastOneStalled (internal/core/jobs).
//   - A panic in a handler is recovered and is the tool's failure: free the first time, the
//     job's own failure the second in a row (D233 §6).
//     TestRunDue_APanicIsTheToolsFailureFreeOnceAndTheJobsFailureTheSecondTimeInARow.
//   - A claim refuses to run a job that has had its attempts, so a job that stops the program in
//     the middle of every attempt ends (S86). TestRunDue_AClaimRefusesAJobThatHasHadItsAttempts,
//     TestScenario_S86_AJobThatBringsTheServerDownEveryTimeFailsAtItsLimitAndAPanicFailsOnlyItsJob.
//   - What a stop left running returns to the queue with the attempt its claim counted.
//     TestRequeueRunning_ReturnsWhatAStopLeftRunningWithItsAttemptCounted.
//   - A cancelled queued job never runs; a running one's context ends with ErrCancelled; either
//     stays cancelled whatever its handler answers after.
//     TestCancel_AQueuedJobNeverRunsAndARunningOnesContextEndsWithErrCancelled.
//   - Time comes from a clock.Clock and jitter from the Random a queue is given, so every wait is
//     testable. Enforced by the forbidigo rule tagged [clock]; the tests use clocktest and
//     jobqueuetest.Random.
//   - Imports nothing from the module but internal/platform. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D231 §2, §8, §10: our own queue, generic; one job per target, merged; jittered back-off.
//   - D233 §6: a panic is the tool's failure, and a claim checks the attempt count.
//   - D170: provider rate limits and back-off.
//   - docs/contributors/design/plans/jobs-rebuild.md: the phases, and what Phase 2 adds here.
//
// # Where to change
//
//   - job.go: the kinds, states, requests, jobs and rows. storage.go: what a storage keeps and
//     the extension point for Phase 2's parked destinations (Claim). answers.go: what a handler
//     can answer. queue.go: enqueue and its merge, cancel, progress, the listing. runner.go:
//     claiming, running and recording each outcome. backoff.go: the back-off and its jitter.
//   - internal/store/jobs.go: the storage on cache.db. jobqueuetest: the storage in memory.
package jobqueue

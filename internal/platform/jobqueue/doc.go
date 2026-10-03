// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobqueue is a job queue that knows no program's kinds, targets or destinations (D231
// §2): a kind is a name, a target a comparable value it only compares, a destination a name kept
// beside it, and its storage an interface, which internal/store implements on cache.db so that
// only store touches SQLite. It claims by priority, merges a repeat request into the job already
// open for its kind and target, retries with jittered back-off, snoozes without spending an
// attempt, parks the work of a destination that is down and probes it (D231 §5, §6), cancels,
// returns what a stop left running, refuses a job past its attempt limit, and recovers from a
// panic in a handler. Work is derived by sweeps from the program's own state, so the queue is a
// cache of what that state implies (D231 §3); a job that fails is recorded by its kind as its
// item's own failure (§4, §10). internal/core/jobs is Orpheum's use of it: the kinds, their
// targets and handlers, and its destinations.
//
// # Invariants
//
//   - A job kept wanted keeps the later of the times it was given; past it, DemoteUnwanted gives it
//     the priority asked for, forgets its time and returns it; a job never kept wanted is left
//     (D234 §3). TestDemoteUnwanted_GivesTheJobsNoOneWaitsForAnyMoreTheirNewPriority.
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
//     with other jobs. TestRunDue_ASnoozeCostsNoAttemptAndTheRunnerGoesOn.
//   - A job answered Park costs no attempt and is kept with the destination it parked; no job
//     kept with a parked destination is claimed, and other work runs on (D231 §6).
//     TestRunDue_AParkedDestinationHoldsItsJobsAtNoCostAndOtherWorkRunsOn,
//     TestJobStorage_AClaimPassesOverAParkedDestinationAndAWaitKeepsWhereItWasParked (store).
//   - A parked destination is probed by one job at a time, on a back-off jittered as a retry's,
//     from 30 s doubling to an hour, or at the time it named; a failed probe costs nothing, and a
//     probe that succeeds unparks it and its work resumes in priority order.
//     TestRunDue_OneProbeOnABackoffFrom30sToAnHourAFailedOneCostingNothingThenAllResumeByPriority,
//     TestRunDue_AProbeGoesAtTheTimeTheDestinationNamed. A failure that is not the probe's
//     changes nothing of how a parked destination comes back, but for a later time it names.
//     TestPark_AFailureThatIsNotTheProbeChangesNothingOfHowAParkedDestinationComesBack.
//   - A person's request for a job whose destination is parked probes it at once; a destination
//     only a signal brings back is never probed, and resumes when unparked.
//     TestEnqueue_APersonsRequestLooksAtAParkedDestinationAtOnce,
//     TestRunDue_ADestinationThatAwaitsASignalIsNeverProbedAndResumesWhenUnparked.
//   - The program's own storage is a destination: a claim, a record or an answer it refuses for
//     want of room parks it, no runner claims until one claim finds room, and an outcome it could
//     not write is written then, so no job is left running (S77).
//     TestRunDue_AFullStorageStopsEveryClaimUntilAProbeFindsRoomAndNothingIsLeftRunning.
//   - Parked state is held in memory: after a restart the first job for a destination probes it
//     again (D231 §6). Enforced by review: nothing here writes it.
//   - A tool's failure costs no attempt and the next attempt is told of it.
//     TestRunDue_AStallCostsNoAttemptAndTheNextAttemptKnowsTheLastOneStalled (internal/core/jobs).
//     A tool's failure that parks its job keeps that: the attempt after the destination returns is
//     told of it (D233 §10). TestRunDue_AToolFailureThatParksItsJobIsStillATheToolsFailure.
//   - A panic in a handler is recovered and is the tool's failure: free the first time, the
//     job's own failure the second in a row (D233 §6).
//     TestRunDue_APanicIsTheToolsFailureFreeOnceAndTheJobsFailureTheSecondTimeInARow.
//   - A claim refuses to run a job that has had its attempts, so a job that stops the program in
//     the middle of every attempt ends (S86). TestRunDue_AClaimRefusesAJobThatHasHadItsAttempts,
//     TestScenario_S86_AJobThatBringsTheServerDownEveryTimeFailsAtItsLimitAndAPanicFailsOnlyItsJob.
//   - What a stop left running returns to the queue with the attempt its claim counted.
//     TestRequeueRunning_ReturnsWhatAStopLeftRunningWithItsAttemptCounted.
//   - A job a handler gave to a node (Assign) is assigned: its runner goes on, and nothing waits
//     for it. The node's outcome is taken once (Runner.Resolve), as a handler's answer; one past
//     its time is its own failure, retried or failed at its limit, and a late outcome is dropped.
//     An assigned job counts as running for a request and a cancel, and a restart leaves it with
//     its node (D234 §7). ResolveBy decides the outcome from the job as assigned, and a cancel is
//     told to the queue's cancel hook (OnCancel), so the node can be told to stop. Work given with
//     AssignThen is handed over only once its job is recorded assigned, so the node's word, however
//     soon, finds it assigned. TestAssignThen_HandsTheWorkOverOnceTheJobIsRecordedAssigned,
//     TestResolveBy_DecidesFromTheJobAndACancelIsHeardByTheHook,
//     TestAssign_AJobGivenToANodeFreesItsRunnerAndItsOutcomeIsTakenOnce,
//     TestResolve_TakesTheNodesOutcomeAsAHandlersAnswer,
//     TestAssign_AJobWhoseTimeRunsOutFailsAndALateOutcomeIsDropped,
//     TestAssign_AnAssignedJobCountsAsRunningForRequestsCancelsAndRestarts,
//     TestJobStorage_KeepsAnAssignedJobOpenAndRunningThroughARestart,
//     TestJobStorage_ACancelEndsAnAssignedJobAndNoOutcomeTakesIt,
//     TestJobStorage_TakesAnAssignedJobBackOnceByItsKindWhenItsTimeRanOut (store).
//   - A cancelled queued job never runs; a running one's context ends with ErrCancelled; either
//     stays cancelled whatever its handler answers after.
//     TestCancel_AQueuedJobNeverRunsAndARunningOnesContextEndsWithErrCancelled.
//   - A sweep derives work again from the program's state: a job lost, cancelled or never queued
//     is queued by the next sweep, and one already queued merges (D231 §3, §9).
//     TestSweep_WorkLostFromTheQueueIsDerivedAgainAndARepeatMerges.
//   - Sweeping runs every sweep at start, each again when its interval has passed, those asked
//     for, and every one when a destination comes back.
//     TestSweeping_RunsEverySweepAtOnceThenEachOnItsTimerAndWhenAskedOrADestinationReturns.
//   - A request a sweep derived looks at no parked destination, whatever its priority.
//     TestEnqueue_APersonsRequestLooksAtAParkedDestinationAtOnce.
//   - A job that fails is recorded by its kind's FailureRecorder as its item's own failure: at its
//     attempt limit, on a permanent answer, or refused at its claim; a parked or cancelled job is
//     not (D231 §4, §10). TestRunDue_AJobThatFailsRecordsItsItemsFailureAndOneThatParksOrIsCancelledDoesNot.
//   - What became of a finished job is no fact a program derives work from: the storage lists the
//     newest jobs for a person and the open ones for a cancel, and nothing else (D231 §3).
//     TestArch_NothingButTheQueueAndTheJobViewReadsJobHistory.
//   - CancelWhere cancels the open work of what a program retires, and Forget drops its parked
//     destination (D231 §7). TestCancelWhere_CancelsTheOpenWorkOfWhatIsRetiredAndForgetDropsItsDestination.
//   - Time comes from a clock.Clock and jitter from the Random a queue is given, so every wait is
//     testable. Enforced by the forbidigo rule tagged [clock]; the tests use clocktest and
//     jobqueuetest.Random.
//   - Imports nothing from the module but internal/platform. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D231 §2, §8, §10: our own queue, generic; one job per target, merged; jittered back-off.
//   - D231 §3, §4, §9: work derived by sweeps; an item's failure a fact; cancel stops a run.
//   - D231 §5, §6: destinations, what counts as down, parking and probing.
//   - D233 §6: a panic is the tool's failure, and a claim checks the attempt count.
//   - D234 §7: the core never blocks on a node; a job given to one is assigned.
//   - D170: provider rate limits and back-off.
//   - docs/contributors/design/plans/jobs-rebuild.md: the phases.
//
// # Where to change
//
//   - job.go: the kinds, states, requests, jobs and rows. storage.go: what a storage keeps and
//     what a claim passes over. answers.go: what a handler can answer, Assign among them. queue.go: enqueue and its
//     merge, cancel, progress, the listing. runner.go: claiming, probing, running and recording
//     each outcome, resolving an assigned job and failing one past its time, and a kind's failure
//     recorder. destination.go: parked destinations, their
//     probes and their state. sweep.go: the sweeps, and cancelling what is retired.
//     backoff.go: the back-off and its jitter.
//   - internal/store/jobs.go: the storage on cache.db. jobqueuetest: the storage in memory.
package jobqueue

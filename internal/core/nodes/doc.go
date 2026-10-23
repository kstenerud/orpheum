// SPDX-License-Identifier: AGPL-3.0-or-later

// Package nodes is the core's side of the node channel (D207): it enrols a node by name on its
// hello, queues its commands for the long-poll and takes its status, results and progress. It starts
// no node: each is a service of its own that dials the core's
// socket (D233 §2, D235 §4).
//
// # Invariants
//
//   - The core never reaches os/exec, and starts no process. Enforced by the depguard rule
//     core-boundary and TestArch_CoreNeverExecs.
//   - A node is enrolled once per name (D207), and its hello asks for a scan of each of its roots a
//     library covers, and no other, when should_scan_on_node_hello says so: a folder is scanned only
//     once it is in a library, which says what kind of thing it holds (the owner, 2026-10-02).
//     TestHello_EnrolsANodeByNameOnceAndQueuesAScanOfEachRootALibraryCovers,
//     TestHello_ScansNothingWhenTheSettingSaysNot.
//   - What a node's machine can do is its config's, told in its hello: after each hello the
//     prepare slots of every node that said one are added up for whoever runs prepares, and each
//     node's slowest read is kept for waiting on its measurements (nodeproto.Capacity).
//     TestHello_TellsTheCoreEachNodesSlotsAndHowFastItReads.
//   - Each status a node posts is taken as it arrives: the node is heard, and its last status kept;
//     one that is not a status is refused. TestStatus_TheLastIsKeptAndTheNodeHeard,
//     TestStatus_ABodyThatIsNotAStatusIsRefused.
//   - A node silent for node_silent_after_seconds is reported as silent; restarting a node that
//     hangs or exits is its service manager's (D234 §4).
//     TestNodeConnection_IsConnectedWhileItPollsOrWasHeardLatelyAndSilentOtherwise.
//   - A node's word on how a job ended, bounded, is one Result posted to one path, whichever the
//     work: it goes to the result sink (Exchanges) and nowhere else, and one from a node that has
//     not said hello is refused. An artwork result is the artwork store's one way in (D212 §8).
//     TestResult_AResultReachesTheResultSinkFromAKnownNodeOnly. Acknowledged means recorded (D234
//     §5, D238 §2): each result is recorded by its kind's Recorder, by the content its echoed work
//     names, as it arrives and whether or not a job still waits (S81), before any job hears it and
//     before the node is answered; a result not recorded is refused, and the node sends it again.
//     TestExchanges_RecordEveryResultBeforeAnyJobHearsIt. A result too large to post is fetched from
//     the node's byte socket by its attempt's token and taken only at the size and SHA-256 its post
//     named, for the same job and attempt; one that cannot be fetched leaves the post unacknowledged.
//     TestResult_AHeldResultIsFetchedWholeBeforeItIsTaken,
//     TestTakeHeld_TakesOnlyTheBytesThePostNamedForItsAttempt,
//     TestReadHeldResult_FetchesItFromTheNodesByteSocket. Its word on where a job is, its
//     progress, has a path of its own, best effort: it starts the job's silence again and reaches the
//     job's OnProgress, its own attempt's only (D234 §5).
//     TestProgress_ReachesTheResultSinkFromAKnownNodeOnly,
//     TestExchange_ProgressKeepsTheWaitAliveAndOnlyItsOwnReachesTheJob.
//   - A job gives a node work through Exchanges, the one way it is done: the command sent, named
//     by an attempt token made for that sending, the node's results resolving the job by that
//     token and only from the node it was given to, never by the job's id alone, which an earlier
//     attempt and a discarded cache.db share (S83, S84), its silence bounded by the core's clock,
//     and the node sent CancelCommand, naming the attempt, on every way out that leaves it working —
//     a cancel, a silence, progress refused (audit-2026-09-30 §3). Playback and artwork both give
//     work this way. TestExchange_AResultReachesTheJobThatAskedAndAFinishedOneIsNotCancelled,
//     TestExchange_ASilenceTellsTheNodeToStopAndParksOnlyWhenItWentAway,
//     TestExchange_ACancelTellsTheNodeToStop,
//     TestExchanges_AResultForNoWaitingJobIsLoggedByItsJobID,
//     TestService_ACancelledJobTellsTheNodeToStopItByItsID.
//   - A job whose work a node does is given to it, not waited for (D234 §7): Assign keeps its attempt,
//     by token, with its job, kind, work and the start of its node (D238 §3), in cache.db, and is
//     the handler's answer, the job assigned until assigned_job_limit_hours; the command is sent
//     once the job is recorded assigned.
//     A job whose time ran out tells its node to stop the work, and its attempt is forgotten
//     (Overdue, G4, D244 §6). TestExchange_AJobWhoseTimeRanOutTellsTheNodeToStop.
//     The node's word on how it ended resolves it, by the token, with its kind's answers (Assignee),
//     after a restart too (Load); an attempt said nothing of for the silence limit, or whose node
//     started again, is unanswered, and its node told to stop; a cancel tells its node to stop and
//     its kind (Cancelled); progress its kind refuses ends it.
//     TestAssign_AJobGivenToANodeIsResolvedByItsWordAndForgotten,
//     TestLoad_AnAttemptKeptThroughARestartIsResolvedByItsNodesWord,
//     TestDeliver_AResultBeforeItsKindIsResolvedIsNotTaken,
//     TestWatch_ASilentAttemptIsUnansweredAndProgressKeepsItHeard,
//     TestCancelledAndInterrupt_EndTheAttemptsOfTheirJobAndNode.
//   - A job that ends while its command waits for the node's next poll has the command withdrawn,
//     its work and its cancel, and never sent (S27, G4); a stop of work the node never took sends
//     nothing. TestWithdraw_TakesAnEndedAttemptsCommandsOutBeforeTheNodeTakesThem,
//     TestExchange_ACancelBeforeTheNodeTookTheWorkSendsItNothing.
//   - Each status is reconciled with what the node was given (D234 §6), judging no attempt whose command
//     the node had not taken: one held is adopted, heard of (S82); one not held, given to the start
//     it says hello from, is lost, a broken invariant logged and counted, its job resolved as work
//     lost, an attempt's failure and never the file's (ErrWorkLost, D248); work held that no job
//     was given, or wants, is a broken invariant, and the node told to drop it (S91, G4); work
//     whose result is on its way is left to it.
//     TestReconcile_ReconcilesWhatANodeWasGivenWithWhatItHolds,
//     TestStatus_IsReconciledWithWhatTheNodeWasGivenButForCommandsOnTheirWay.
//   - The core holds work only while its node says it moves: work held that the status says has not
//     moved for MovementWindow, past every window the node judges its own work by, is stopped and
//     resolved as ErrNotMoving, which UnansweredOutcome takes as a stall (D233 §5).
//     TestReconcile_WorkItsNodeSaysHasNotMovedForTheWindowIsStopped.
//   - A hello makes each library root on its node that it does not name unavailable, whether or not
//     it asks for scans (D241 §2). TestHello_MakesALibraryRootItNoLongerNamesUnavailable.
//   - Work that stalls waiting on its disk the first time makes the root it reads unavailable
//     (DiskStalls, which asks for a scan) and parks on the root as the tool's failure; a listing
//     or an examining, the scan itself, is tried again instead. The same stall after is the
//     target's own failure, its disk unreadable (D233 §10).
//     TestExchange_AStallOnTheDiskParksOnItsRootTheFirstTimeAndIsTheKindsToAnswerAfter,
//     TestStallOutcome_ASecondStallOnTheDiskIsTheTargetsUnreadableDisk.
//   - A prepare whose run waited on a file outside its root, as its node said, waited on its
//     prepared directory's disk: its root stays available and the node's prepares park on the
//     prepared directory, probed on the back-off (D244 §6).
//     TestExchange_AStallOnThePreparedDirectorysDiskParksOnItAndLeavesTheRoot.
//   - A node whose hello names another start than the one an attempt given to it keeps started again
//     and lost the work it held in memory (D238 §3), whether or not the core started again too: at
//     each hello every job given to another start of its node learns so at once (ErrNodeRestarted),
//     no broken invariant, nothing is sent to stop what it no longer has, and each handler answers
//     it as work lost, an attempt's failure and never the file's (D248). The start kept with the attempt in cache.db is
//     the one way a restart is known. TestHello_ANodeThatStartedAgainInterruptsTheJobsWaitingOnIt,
//     TestHello_ANodeThatStartedAgainWithTheCoreInterruptsTheWorkItsLastStartWasGiven,
//     TestLoad_AnAttemptKeptThroughARestartKeepsTheStartOfItsNode,
//     TestExchange_ANodeThatStartedAgainEndsTheWaitOfItsJobsOnly.
//   - A command carries its job's priority, by which the node orders what waits (D234 §3).
//     TestExchange_PlayPriorityWorkIsSentPlayDrivenAndBackgroundWorkIsNot.
//   - The reply to a hello names the renditions it listed that the index keeps (RenditionIndex),
//     and the node evicts the others; with no index, or one that cannot be reconciled, every one is
//     kept, so the node evicts nothing for the core's failure (D238 §2).
//     TestHello_RepliesWithTheRenditionsTheIndexKeepsAndKeepsAllWhenItCannotSay.
//   - The core gives a node only as many jobs of a slot as it runs at once, and as many reading a
//     device as may read it, each with node_jobs_beyond_free_slots more; work beyond waits, parked at no
//     cost on the slot or device that is full, brought back when one of the node's jobs there ends
//     or the node says hello (D240 §5); play-driven work is never held back for its device's
//     readers, the node pausing background work for it (D240 §6).
//     TestAssign_WorkBeyondANodesWindowWaitsForTheSlotOrDeviceThatIsFull,
//     TestAssign_PlayDrivenWorkIsNotHeldBackForItsDevicesReaders,
//     TestHello_BringsBackTheWorkParkedOnTheNodeAndOnAnyNode.
//   - Work is in one of three tiers (D255 §1): a person's, by its job's priority; slow, reading whole
//     films or encoding them; and fast, the rest, as a scan's examining. Where a slot or device is
//     full, work of a higher tier bumps the lowest held there, the lowest priority of it, then the
//     newest: its node is told to stop, and its job goes back to the head of its queue at no cost
//     (D255 §2, D324 §4). Work of the same tier or lower waits.
//     TestAssign_WorkOfAHigherTierBumpsTheLowestWhereNothingIsFree,
//     TestAssign_OfOneTierTheLowestPriorityIsBumpedFirst.
//   - Slow work leaves a device's last reader to work of a higher tier that came for it within the
//     last five seconds, waiting on the device's last reader, its own destination, until then, so
//     that work's next piece is not given the reader by bumping a read-through just started (D255
//     §4). A person's work is never held for it.
//     TestAssign_SlowWorkLeavesADisksLastReaderToFasterWorkThatCameLately,
//     TestAssign_SlowWorkTakesTheLastReaderOnceFasterWorkIsQuiet.
//   - Background work that reads whole films, a read-through or a subtitle extracted from inside one,
//     waits while a scan is on its quick pass of the film's disk, on that disk's quick pass rather
//     than its readers, asking again every half minute (D255); a person's goes ahead (D324 §4).
//     TestAssign_ABackgroundReadThroughWaitsForAScansQuickPassOfItsDisk,
//     TestAssign_APersonsReadThroughGoesAheadOfAScansQuickPass and, for when a root is on it,
//     TestScans_ARootIsOnItsQuickPassUntilItsNewFilesAreRead (internal/core/library).
//   - Playing is not starved (D324 §5): a node that encodes in software and holds a played encode
//     is given no other encode, which waits on its encode slot; held back (HoldBackFor), its played
//     encode behind playback, it stops the work no one plays and is given none until the hold ends,
//     on its playing destination. A node that encodes on a GPU runs work beside it.
//     TestAssign_NoOtherEncodeIsGivenBesideAPlayedEncodeInSoftware,
//     TestHoldBackFor_StopsWhatIsNotPlayedAndGivesNoneUntilItEnds; whether a node encodes in software
//     is its last hello's, TestHello_TellsTheCoreWhetherEachNodeEncodesInSoftware.
//   - A job no one waits for any more is told to its node at its new priority, no longer
//     play-driven (D234 §3). TestReprioritize_TellsTheNodeHoldingAJobsAttemptItsNewPriority.
//   - Work a person waits for, to play, is sent play-driven (D240 §6), and each answer to a poll for
//     commands carries the reader limits the settings say then. Where a node last said a job's work
//     is, by its progress or status, is kept for the job view (ReportedProgressOf).
//     TestExchange_PlayPriorityWorkIsSentPlayDrivenAndBackgroundWorkIsNot,
//     TestCommands_EachAnswerCarriesTheReaderLimitsTheSettingsSayNow,
//     TestReportedProgressOf_IsWhereTheNodeLastSaidAJobsWorkIs.
//   - A device's reader limit an admin set wins over its kind's setting until cleared, in the hello's
//     reply, each poll's answer and the System page (D240 §5).
//     TestSetReaderLimit_ADevicesSetLimitWinsOverItsKindsUntilCleared.
//   - The reply to a hello gives each device it lists the reader limit of its kind's setting
//     (device_readers_*), an unknown kind's for one the core does not know (D240 §5).
//     TestHello_RepliesWithEachDevicesReaderLimitByItsKind.
//   - What a node's last hello said about its FFmpeg's confinement is kept while the core runs,
//     for doctor and the admin to say out loud (D112).
//     TestHello_KeepsWhatTheNodeSaidAboutItsConfinement.
//   - A node that says hello in another version of the node protocol is refused, shown as needing
//     an update, and its work parks with that reason until it says hello in the core's version; a
//     command it refused, not knowing it, parks its job the same way (D233 §9).
//     TestHello_ANodeOnAnotherProtocolIsRefusedAndItsWorkWaitsUntilItIsUpdated,
//     TestExchange_ARefusalParksTheJobUntilTheNodeIsUpdatedAndStopsNothing.
//   - A node's last hello and last status are kept whole, for the System page (D225): its
//     hardware, disks and prepared directory. TestFacts_KeepTheHellosHardwareAndTheLastStatussDisks.
//   - A node is connected while a long-poll for its commands is open or it was heard within
//     node_silent_after_seconds, silent after, and awaiting its hello until it says one since the
//     core started (D231 §11). TestNodeConnection_IsConnectedWhileItPollsOrWasHeardLatelyAndSilentOtherwise.
//   - Work is sent with Send, which parks its job on a node that is not connected and sends
//     nothing; a wait for an answer that runs out parks the job when the node went away, and is
//     the job's own failure when it did not. TestSend_ACommandToANodeNotConnectedParksItsJobAndSendsNothing,
//     TestUnanswered_ParksOnANodeThatWentAwayAndIsTheJobsOwnFailureOtherwise.
//   - A node's hello brings back the work parked on it and on any node; its status showing room
//     brings back the prepares parked on its prepared directory (D231 §5, §6).
//     TestHello_BringsBackTheWorkParkedOnTheNodeAndOnAnyNode,
//     TestStatus_ShowingRoomBringsBackThePreparesParkedOnThePreparedDirectory. Room is the disk
//     within its fullness limit and keeping its floor of free space.
//   - A node an admin retired is removed and forgotten: it is told to say hello first, and its
//     name, saying hello again, enrols a new node (D231 §7).
//     TestRemoveNode_ARetiredNodeIsToldToSayHelloAgainAndItsNameEnrolsANewNode.
//   - A node watcher's notice of a change is handed to what scans it, and each root's watch its
//     last status said is kept for the libraries page (D272 §9).
//     TestChanged_AWatchersNoticeIsHandedOnAndItsRootsWatchesKept.
//
// # Read before changing
//
//   - D43, D44: one process per role, and never exit when you could report; D233 §2 and D235 §4
//     made the node a service of its own.
//   - D107: the socket's permissions are the authentication in Cut A.
//   - D34, D37: liveness is judged from what the core observes.
//   - D238: each fact has one owner; the node holds its work in memory, and a restart loses it.
//   - D231 §5, §6, §11: a node not connected is a destination that parks its work, and its hello
//     brings the work back.
//   - docs/contributors/design/plans/cut-a.md §2 and §4.
//
// # Where to change
//
//   - service.go: the service, its parts and its paths; enrol.go: hello, enrolment and retiring a
//     node; connection.go: whether each node is connected and when it was last heard; send.go:
//     sending a node work, and what a wait for it that ran out means; facts.go: what each node
//     last said about itself — its roots, slots, hardware and status;
//     channel.go: commands, the status and the nodes' results; bytes.go:
//     reading a file's bytes from a node's byte socket (cut-a §4); exchange.go: the exchanges,
//     and the results and progress nodes send; assign.go: a job given to a node, assigned, and
//     resolved by its word; reconcile.go: what a node was given reconciled with what it holds.
//   - devices.go: how many may read each of a node's devices, by its kind's setting or an admin's
//     limit (D240 §5); disk.go: work stalled on its disk, which parks its root (D233 §10).
package nodes

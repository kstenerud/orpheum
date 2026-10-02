// SPDX-License-Identifier: GPL-3.0-or-later

// Package nodes is the core's side of the node channel (D207): it enrols a node by name on its
// hello, queues its commands for the long-poll, reads its JSONL report as it arrives and hands it
// to the ingest in batches. It starts no node: each is a service of its own that dials the core's
// socket (D233 §2, D235 §4).
//
// # Invariants
//
//   - The core never reaches os/exec, and starts no process. Enforced by the depguard rule
//     core-boundary and TestArch_CoreNeverExecs.
//   - A node is enrolled once per name (D207), and its hello scans each of its roots a library
//     covers, and no other: a folder is scanned only once it is in a library, which says what kind
//     of thing it holds (the owner, 2026-10-02).
//     TestHello_EnrolsANodeByNameOnceAndQueuesAScanOfEachRootALibraryCovers.
//   - What a node's machine can do is its config's, told in its hello: after each hello the
//     prepare slots of every node that said one are added up for whoever runs prepares, and each
//     node's slowest read is kept for waiting on its measurements (nodeproto.Capacity).
//     TestHello_TellsTheCoreEachNodesSlotsAndHowFastItReads.
//   - A report's records reach the ingest in order, batched, and as soon as the stream goes
//     quiet; a record out of sequence is refused. TestServeReport_HandsRecordsToTheSinkInOrderAndRecordsHowFarItGot,
//     TestServeReport_ARecordReachesTheIngestWhileTheReportIsStillOpen, TestServeReport_RefusesARecordOutOfSequence.
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
//     by token, with its job, kind and work, in cache.db, and is the handler's answer, the job
//     assigned until assigned_job_limit_hours; the command is sent once the job is recorded assigned.
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
//   - Each status is squared with what the node was given (D234 §6), judging no attempt whose command
//     the node had not taken: one held is adopted, heard of (S82); one not held is lost, a broken
//     invariant logged and counted, its job resolved as the tool's failure (ErrWorkLost); work held
//     that no job was given, or wants, is a broken invariant, and the node told to drop it (S91, G4);
//     work whose result is on its way is left to it. TestSquare_SquaresWhatANodeWasGivenWithWhatItHolds,
//     TestStatus_IsSquaredWithWhatTheNodeWasGivenButForCommandsOnTheirWay.
//   - A node whose hello names another start than its last hello to this core started again and
//     lost the work it held in memory (D238 §3): every job waiting on it learns so at once
//     (ErrNodeRestarted), nothing is sent to stop what it no longer has, and each handler answers
//     it as the tool's failure, free the first time. TestHello_ANodeThatStartedAgainInterruptsTheJobsWaitingOnIt,
//     TestExchange_ANodeThatStartedAgainEndsTheWaitOfItsJobsOnly.
//   - A command carries its job's priority, by which the node orders what waits (D234 §3).
//     TestExchange_ASilenceTellsTheNodeToStopAndParksOnlyWhenItWentAway.
//   - The reply to a hello names the renditions it listed that the index keeps (RenditionIndex),
//     and the node evicts the others; with no index, or one that cannot be squared, every one is
//     kept, so the node evicts nothing for the core's failure (D238 §2).
//     TestHello_RepliesWithTheRenditionsTheIndexKeepsAndKeepsAllWhenItCannotSay.
//   - The core gives a node only as many jobs of a slot as it runs at once, and as many reading a
//     device as may read it, each with node_work_window_extra more; work beyond waits, parked at no
//     cost on the slot or device that is full, brought back when one of the node's jobs there ends
//     or the node says hello (D240 §5). TestAssign_WorkBeyondANodesWindowWaitsForTheSlotOrDeviceThatIsFull,
//     TestHello_BringsBackTheWorkParkedOnTheNodeAndOnAnyNode.
//   - A job no one waits for any more is told to its node at its new priority, no longer
//     play-driven (D234 §3). TestReprioritize_TellsTheNodeHoldingAJobsAttemptItsNewPriority.
//   - Work a person waits for, to play, is sent play-driven (D240 §6), and each answer to a poll for
//     commands carries the reader limits the settings say then. Where a node last said a job's work
//     is, by its progress or status, is kept for the job view (NodeWordOf).
//     TestExchange_PlayPriorityWorkIsSentPlayDrivenAndBackgroundWorkIsNot,
//     TestCommands_EachAnswerCarriesTheReaderLimitsTheSettingsSayNow,
//     TestNodeWordOf_IsWhereTheNodeLastSaidAJobsWorkIs.
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
//   - docs/contributors/design/plans/cut-a.md §2 and §4; scanner.md, The core reads the report
//     while it is still being written.
//
// # Where to change
//
//   - service.go: the service, its parts and its paths; enrol.go: hello, enrolment and retiring a
//     node; connection.go: whether each node is connected and when it was last heard; send.go:
//     sending a node work, and what a wait for it that ran out means; facts.go: what each node
//     last said about itself — its roots, slots, hardware and status;
//     channel.go: commands, the report and the nodes' results; bytes.go:
//     reading a file's bytes from a node's byte socket (cut-a §4); exchange.go: the exchanges,
//     and the results and progress nodes send; assign.go: a job given to a node, assigned, and
//     resolved by its word; square.go: what a node was given squared with what it holds.
package nodes

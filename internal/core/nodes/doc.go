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
//   - A node is enrolled once per name (D207). TestHello_EnrolsANodeByNameOnceAndQueuesAScanOfEachRoot.
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
//   - A job asks a node for work through Exchanges, the one way it is done: the command sent,
//     named by an attempt token made for that sending, the node's results handed to the job by
//     that token and only from the node it asked, never by the job's id alone, which an earlier
//     attempt and a discarded cache.db share (S83, S84), the wait bounded by the core's clock, and
//     the node sent CancelCommand, naming the attempt, on every way out that leaves it working — a cancel, a silence,
//     a failure after the send (audit-2026-09-30 §3). Playback and artwork both ask this way.
//     TestExchange_AResultReachesTheJobThatAskedAndAFinishedOneIsNotCancelled,
//     TestExchange_ASilenceTellsTheNodeToStopAndParksOnlyWhenItWentAway,
//     TestExchange_AContextEndingTellsTheNodeToStop,
//     TestExchanges_AResultForNoWaitingJobIsLoggedByItsJobID,
//     TestService_ACancelledJobTellsTheNodeToStopItByItsID.
//   - A node whose hello names another start than its last hello to this core started again and
//     lost the work it held in memory (D238 §3): every job waiting on it learns so at once
//     (ErrNodeRestarted), nothing is sent to stop what it no longer has, and each handler answers
//     it as the tool's failure, free the first time. TestHello_ANodeThatStartedAgainInterruptsTheJobsWaitingOnIt,
//     TestExchange_ANodeThatStartedAgainEndsTheWaitOfItsJobsOnly.
//   - A command carries its job's priority, by which the node orders what waits (D234 §3).
//     TestExchange_ASilenceTellsTheNodeToStopAndParksOnlyWhenItWentAway.
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
//     TestStatus_ShowingRoomBringsBackThePreparesParkedOnThePreparedDirectory.
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
//     reading a file's bytes from a node's byte socket (cut-a §4); exchange.go: a job asking a
//     node for work and waiting for its results, and the cancel it sends.
package nodes

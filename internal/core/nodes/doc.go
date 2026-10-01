// SPDX-License-Identifier: GPL-3.0-or-later

// Package nodes is the core's side of the node channel (D207): it enrols a node by name on its
// hello, queues its commands for the long-poll, reads its JSONL report as it arrives and hands it
// to the ingest in batches, and supervises the co-located node child — started through a Starter
// that cmd/orpheum builds on execx, because the core never starts a process.
//
// # Invariants
//
//   - The core never reaches os/exec: the child is started through the Starter interface.
//     Enforced by the depguard rule core-boundary and TestArch_CoreNeverExecs.
//   - A node is enrolled once per name (D207). TestHello_EnrolsANodeByNameOnceAndQueuesAScanOfEachRoot.
//   - What a node's machine can do is its config's, told in its hello: after each hello the
//     prepare slots of every node that said one are added up for whoever runs prepares, and each
//     node's slowest read is kept for waiting on its measurements (nodeproto.Capacity).
//     TestHello_TellsTheCoreEachNodesSlotsAndHowFastItReads.
//   - A report's records reach the ingest in order, batched, and as soon as the stream goes
//     quiet; a record out of sequence is refused. TestServeReport_HandsRecordsToTheSinkInOrderAndRecordsHowFarItGot,
//     TestServeReport_ARecordReachesTheIngestWhileTheReportIsStillOpen, TestServeReport_RefusesARecordOutOfSequence.
//   - A node silent for node_silent_after_seconds is reported; silent for
//     node_restart_after_silent_seconds it is restarted; one that exits is started again after
//     node_restart_after_exit_seconds, so it cannot spin.
//     TestSupervisor_RestartsANodeThatStaysSilent, TestSupervisor_StartsAnExitedNodeAgainAfterTheDelay.
//   - A node's artwork answer, bounded, goes to the artwork service and nowhere else; one from a
//     node that has not said hello is refused (D212 §8).
//     TestArtwork_AnAnswerReachesTheArtworkSinkFromAKnownNodeOnly. A prepare report goes to the
//     playback service the same way, through the same handler.
//   - A job asks a node for work through Exchanges, the one way it is done: the command sent,
//     the node's results handed to the job by its id, the wait bounded by the core's clock, and
//     the node sent CancelCommand on every way out that leaves it working — a cancel, a silence,
//     a failure after the send (audit-2026-09-30 §3). Playback and artwork both ask this way.
//     TestExchange_AResultReachesTheJobThatAskedAndAFinishedOneIsNotCancelled,
//     TestExchange_ASilenceTellsTheNodeToStopAndParksOnlyWhenItWentAway,
//     TestExchange_AContextEndingTellsTheNodeToStop,
//     TestExchanges_AResultForNoWaitingJobIsLoggedByItsJobID,
//     TestService_ACancelledJobTellsTheNodeToStopItByItsID.
//   - What a node's last hello said about its FFmpeg's confinement is kept while the core runs,
//     for doctor and the admin to say out loud (D112).
//     TestHello_KeepsWhatTheNodeSaidAboutItsConfinement.
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
//   - D43, D44: the thin supervisor, and never exit when you could report.
//   - D107: the socket's permissions are the authentication in Cut A.
//   - D34, D37: liveness is judged from what the core observes.
//   - D231 §5, §6, §11: a node not connected is a destination that parks its work, and its hello
//     brings the work back.
//   - docs/contributors/design/plans/cut-a.md §2 and §4; scanner.md, The core reads the report
//     while it is still being written.
//
// # Where to change
//
//   - service.go: hello and enrolment, each node's connection, sending it work, and each
//     connected node's roots as its hello named them;
//     channel.go: commands, the report and artwork answers; supervisor.go: the child; bytes.go:
//     reading a file's bytes from a node's byte socket (cut-a §4); exchange.go: a job asking a
//     node for work and waiting for its results, and the cancel it sends.
package nodes

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
//   - A report's records reach the ingest in order, batched, and as soon as the stream goes
//     quiet; a record out of sequence is refused. TestServeReport_HandsRecordsToTheSinkInOrderAndRecordsHowFarItGot,
//     TestServeReport_ARecordReachesTheIngestWhileTheReportIsStillOpen, TestServeReport_RefusesARecordOutOfSequence.
//   - A node silent for node_silent_after_s is reported; silent for node_restart_after_silent_s it is
//     restarted; one that exits is started again after node_restart_after_exit_s, so it cannot spin.
//     TestSupervisor_RestartsANodeThatStaysSilent, TestSupervisor_StartsAnExitedNodeAgainAfterTheDelay.
//   - A node's artwork answer, bounded, goes to the artwork service and nowhere else; one from a
//     node that has not said hello is refused (D212 §8).
//     TestArtwork_AnAnswerReachesTheArtworkSinkFromAKnownNodeOnly. A prepare report goes to the
//     playback service the same way, through the same handler.
//   - What a node's last hello said about its FFmpeg's confinement is kept while the core runs,
//     for doctor and the admin to say out loud (D112).
//     TestHello_KeepsWhatTheNodeSaidAboutItsConfinement.
//   - A node's last hello and last status are kept whole, for the System page (D225): its
//     hardware, disks and prepared directory. TestFacts_KeepTheHellosHardwareAndTheLastStatussDisks.
//
// # Read before changing
//
//   - D43, D44: the thin supervisor, and never exit when you could report.
//   - D107: the socket's permissions are the authentication in Cut A.
//   - D34, D37: liveness is judged from what the core observes.
//   - docs/contributors/design/plans/cut-a.md §2 and §4; scanner.md, The core reads the report
//     while it is still being written.
//
// # Where to change
//
//   - service.go: hello and enrolment, and each connected node's roots as its hello named them;
//     channel.go: commands, the report and artwork answers; supervisor.go: the child; bytes.go:
//     reading a file's bytes from a node's byte socket (cut-a §4).
package nodes

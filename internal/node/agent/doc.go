// SPDX-License-Identifier: GPL-3.0-or-later

// Package agent is the node's loop (D207): it dials the core's socket, says hello, long-polls for
// commands and runs them one at a time, and streams its report — scan records, and a status record
// every tick — as one JSONL request whose records reach the core as they are written. When the
// core goes away it says hello again at the next tick, and numbers its records on from the last
// one the core ingested.
//
// # Invariants
//
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//   - An artwork command naming an image beside the media is read through the root it is under,
//     never from outside the node's roots, and answered with the SHA-256 of what was read
//     (D215 §3). TestAgent_ReadsAnImageBesideTheMediaOnlyUnderItsRoots.
//   - Records reach the core in the order of their numbers, numbered on from the core's last,
//     each as soon as it is written. TestAgent_SaysHelloRunsAScanAndStreamsItsReportInNumberedOrder.
//   - A root is scanned only if the node was configured with it, whatever the core asks (D107).
//     TestAgent_RefusesToScanARootItWasNotGiven.
//   - Losing the core ends the session and the next tick starts another.
//     TestAgent_SaysHelloAgainAfterLosingTheCore.
//   - An artwork command's answer — the sizes the deriver made, or why none — is posted to the
//     core's artwork path (D212 §8). TestAgent_ReEncodesArtworkAndPostsTheAnswer.
//
// # Read before changing
//
//   - D107: a node's roots are set on the node.
//   - D127, D193 §4: the node dials out; the one transport is HTTP and JSONL over a Unix socket.
//   - D34: silence is only ever read as silence; the status record is how a node is heard.
//   - D212: artwork is re-encoded here, and posted to the core.
//   - docs/contributors/design/plans/cut-a.md §4.
//
// # Where to change
//
//   - agent.go: the session and its four parts; report.go: numbering.
package agent

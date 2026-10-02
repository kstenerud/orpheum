// SPDX-License-Identifier: GPL-3.0-or-later

// Package nodeproto is the wire between core and node: the JSON messages of cut-a §4 (hello, the
// JSONL report records, the commands, and the answers to the jobs they carry), the byte socket's
// routes and the probe record, as plain types. The version is the path's (/node/v1, /bytes/v1);
// no message carries one. It holds no logic, so both sides can import it without importing each
// other.
//
// # Invariants
//
//   - Imports nothing from the module except internal/platform. Enforced by the depguard rule
//     nodeproto-is-types and TestArch_NodeprotoHasNoLogic.
//   - Every field is camelCase on the wire with an explicit tag. Enforced by tagliatelle and
//     musttag.
//   - A closed set of record or command types is a sum type, and every type switch over it is
//     exhaustive. Enforced by gochecksumtype.
//   - Every hello says the protocol version it speaks (ProtocolVersion), raised with any change the
//     other side would misread (D233 §9). Enforced by review.
//   - A command and a result name their job in the envelope alone, and the node answers every job on
//     one path (D234 §2). TestEnvelope_AJobIsNamedByTheEnvelopeAloneNeverByWhatItCarries.
//
// # Read before changing
//
//   - D39: reports are JSONL.
//   - D100: a command names its target by path and payload hash.
//   - D107: the node channel is authenticated by socket permissions in Cut A.
//   - D234 §2: the core sends requests the node answers, each matched by id.
//   - D127: the node dials out; the file channel is deleted (D193).
//   - D85: the node protocol is under the compatibility contract.
//   - docs/contributors/design/plans/cut-a.md §4.
//
// # Where to change
//
//   - One file per feature, each holding both directions: hello.go, report.go (the JSONL records),
//     commands.go (the long-poll, each command naming its job), result.go (the node's word on a
//     job, by its id, on one path), and artwork.go, prepare.go, subtitle.go and loudness.go, each
//     a command's payload and its answer's. stall.go: what a node saw of an FFmpeg run it stopped
//     as stalled, which those answers carry. bytes.go names a file on the node's byte socket. probe.go: the
//     probe record a scan reports and the core keeps.
package nodeproto

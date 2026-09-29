// SPDX-License-Identifier: GPL-3.0-or-later

// Package nodeproto is the wire between core and node: the versioned JSON messages of cut-a §4
// (hello, the JSONL report records, the commands) as plain types. It holds no logic, so both sides
// can import it without importing each other.
//
// # Invariants
//
//   - Imports nothing from the module except internal/platform. Enforced by the depguard rule
//     nodeproto-is-types and TestArch_NodeprotoHasNoLogic.
//   - Every field is camelCase on the wire with an explicit tag. Enforced by tagliatelle and
//     musttag.
//   - A closed set of record or command types is a sum type, and every type switch over it is
//     exhaustive. Enforced by gochecksumtype.
//
// # Read before changing
//
//   - D39: reports are JSONL.
//   - D100: a command names its target by path and payload hash.
//   - D107: the node channel is authenticated by socket permissions in Cut A.
//   - D127: the node dials out; the file channel is deleted (D193).
//   - D85: the node protocol is under the compatibility contract.
//   - docs/contributors/design/plans/cut-a.md §4.
//
// # Where to change
//
//   - One file per feature, each holding both directions: hello.go, report.go (the JSONL records),
//     commands.go (the long-poll), and artwork.go, prepare.go, subtitle.go and loudness.go, each
//     a command and its answer. bytes.go names a file on the node's byte socket.
package nodeproto

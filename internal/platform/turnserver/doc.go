// SPDX-License-Identifier: AGPL-3.0-or-later

// Package turnserver is what the binary's two TURN servers share (AGENTS.md rule 11): the core's
// embedded voice relay (internal/core/turn) and the hosted relay (internal/relay, D558) each listen
// over UDP and TCP on one port, and each hands pion/turn a logger that writes to the process's log.
//
// # Invariants
//
//   - A listen on port 0 serves UDP and TCP on one port, choosing again while TCP finds the port
//     UDP took already taken; a port named is taken or refused as it is. Enforced by
//     TestListenOnOnePort_ChoosesAgainWhenTCPsPortIsTaken.
//   - pion's warnings and errors reach the log; its trace, debug and info do not. Review-only.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D243 §8, §10: the embedded relay's port and pion/turn. D558: the hosted relay.
//
// # Where to change
//
//   - listen.go: the listeners on one port.
//   - log.go: pion's logs, into the process's.
package turnserver

// SPDX-License-Identifier: GPL-3.0-or-later

// Package unixsocket is HTTP over a unix socket: the one way the node dials the core's socket and
// the core dials a node's byte socket, where each had written its own (AGENTS.md rule 11).
//
// # Invariants
//
//   - Every connection a Transport makes is to its socket, whatever host the request names.
//     TestTransport_ReachesTheSocketWhateverTheHost.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D107: the node channel is authenticated by the socket's permissions in Cut A.
//
// # Where to change
//
//   - unixsocket.go.
package unixsocket

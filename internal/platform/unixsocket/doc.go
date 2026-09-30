// SPDX-License-Identifier: GPL-3.0-or-later

// Package unixsocket is HTTP over a unix socket: the one way the node dials the core's socket and
// the core dials a node's byte socket, where each had written its own (AGENTS.md rule 11), and
// the one way each listens on its own: the core's socket for its node, and the node's byte
// socket. It is a platform package so that the node role, which imports nothing from the core,
// can use it.
//
// # Invariants
//
//   - Every connection a Transport makes is to its socket, whatever host the request names.
//     TestTransport_ReachesTheSocketWhateverTheHost.
//   - A socket Listen makes is mode 0600 in a directory of mode 0700, its permissions being the
//     node channel's authentication; a socket a crash left is replaced, and anything else at its
//     path is refused, never removed. TestListen_MakesAPrivateSocketAndReplacesAStaleOne,
//     TestListen_RefusesToRemoveWhatIsNotASocket.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D107: the node channel is authenticated by the socket's permissions in Cut A.
//
// # Where to change
//
//   - unixsocket.go: dialling. listen.go: listening.
package unixsocket

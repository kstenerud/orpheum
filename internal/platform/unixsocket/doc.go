// SPDX-License-Identifier: AGPL-3.0-or-later

// Package unixsocket is HTTP over a unix socket: the one way the node dials the core's socket and
// the core dials a node's byte socket, where each had written its own (AGENTS.md rule 11), and
// the one way each listens on its own: the core's socket for its node, and the node's byte
// socket. It is a platform package so that the node role, which imports nothing from the core,
// can use it.
//
// # Invariants
//
//   - Every connection a Client makes is to its socket, whatever host the request names.
//     TestClient_ReachesTheSocketWhateverTheHost.
//   - A socket serves only the holder of its key: the listener keeps a fresh key beside the socket
//     at each start (WriteKey, mode 0600) and refuses every request without it (RequireKey); a
//     Client presents the key it finds beside the socket, read as each request is sent, so a
//     listener that started again is reached with its new key. Socket permissions alone admit
//     every process of the user, FFmpeg's runs among them, which Landlock keeps from the key
//     (D331 §1). TestRequireKey_ServesOnlyTheHolderOfTheKeyKeptBesideTheSocket.
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
//   - D331: and by a key beside each socket, since a GPU run may open a Unix socket.
//
// # Where to change
//
//   - unixsocket.go: dialling. listen.go: listening. key.go: the socket's key.
package unixsocket

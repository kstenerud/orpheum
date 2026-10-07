// SPDX-License-Identifier: GPL-3.0-or-later

// Package pacing lets a response's sender pace its connection and mark its packets lower than best
// effort: while a person's voice is live, the film's download gives way to it, a few chunks at a
// steady rate instead of bursts that fill the link's queue (D262 §2). The server keeps each
// connection in its requests' context (ConnContext); a handler paces the request's connection
// (Pace) or sets it back (Unpace).
//
// # Invariants
//
//   - A paced connection sends at most the rate asked, in bytes a second, and its packets carry
//     DSCP LE, the lowest effort; set back, it is unpaced and best effort again. Enforced by
//     TestPace_PacesTheConnectionAndMarksItLowerEffortAndUnpaceSetsItBack.
//
//   - A request whose connection was not kept, or is not a TCP socket, is left as it is, never an
//     error to its handler. Enforced by TestPace_LeavesARequestWithNoKeptConnectionAlone.
//
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D262 §2: the film gives way to voice.
//
// # Where to change
//
//   - pacing.go: keeping a connection, and pacing it. socket_linux.go: the socket options;
//     socket_other.go: nothing, where the kernel has no pacing.
package pacing

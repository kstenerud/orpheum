// SPDX-License-Identifier: AGPL-3.0-or-later

// Package ffstreamertest is FFmpeg's side of ffstreamer, in Go, for tests: a test binary that stands
// in for the patched FFmpeg reads its input through the node with it, as the real one does
// (orpheum-ffmpeg's PROTOCOL.md, draft 6). It keeps to the protocol unless told not to, so a node
// that judges it wrongly is the node's fault. It is imported only by tests.
//
// # Invariants
//
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary.
//
// # Read before changing
//
//   - D330, and PROTOCOL.md §5 to §7: the frames FFmpeg's side sends and expects.
//
// # Where to change
//
//   - client.go.
package ffstreamertest

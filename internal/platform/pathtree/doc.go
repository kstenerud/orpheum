// SPDX-License-Identifier: GPL-3.0-or-later

// Package pathtree says whether one absolute path is at or under another: which root, mount or
// directory holds a path. It is the one test of it, where copies had grown in the node's hardware,
// the node protocol's devices and the core's disk stalls (AGENTS.md rule 11).
//
// # Invariants
//
//   - A path is under a directory only at a separator: /media/films is not under /media/film, and
//     everything is under /. TestIsAtOrUnder_CutsOnlyAtASeparator.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - AGENTS.md rule 11: one way to do each job.
//
// # Where to change
//
//   - pathtree.go.
package pathtree

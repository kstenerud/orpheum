// SPDX-License-Identifier: GPL-3.0-or-later

// Package diskspace measures the filesystem that holds a path: its size and what a user who is
// not root may still write. It is the one way to ask (AGENTS.md rule 11), for the store's check
// before a migration, the node's prepared directory limit and the System page (D225).
//
// # Invariants
//
//   - The free space is what a user who is not root may use: blocks reserved for root count as
//     used. Enforced by TestMeasure_SaysTheSizeAndFreeSpaceOfTheFilesystemAtAPath.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D225: the System page shows each disk.
//   - D217 §5: the prepared directory's disk limit.
//
// # Where to change
//
//   - diskspace.go.
package diskspace

// SPDX-License-Identifier: GPL-3.0-or-later

// Package diskspace measures the filesystem that holds a path: its size and what a user who is
// not root may still write; and what the files under a directory take. It is the one way to ask
// each (AGENTS.md rule 11), for the store's check before a migration, the node's prepared
// directory limit and budget, a rendition's size and the System page (D225).
//
// # Invariants
//
//   - The free space is what a user who is not root may use: blocks reserved for root count as
//     used. Enforced by TestMeasure_SaysTheSizeAndFreeSpaceOfTheFilesystemAtAPath.
//   - What a directory takes is the size of its regular files alone: a symbolic link, device or
//     pipe adds nothing, so what a node reports it made is what its budget counts.
//     TestTreeBytes_CountsOnlyTheRegularFilesUnderADirectory.
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

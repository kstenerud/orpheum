// SPDX-License-Identifier: GPL-3.0-or-later

// Package prepare runs one FFmpeg command per prepare job, through execx, producing an fMP4 HLS
// rendition in the node's cache (an EVENT playlist that grows while the job runs), plus
// extract_subtitle jobs for text tracks. The output is a cache; a crash restarts the job from
// zero.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx, the forbidigo rules tagged [exec] and [execve], and
//     TestArch_OnlyExecxImportsOsExec.
//   - Removes, renames and truncates nothing. Enforced by the forbidigo rule tagged [delete].
//
// # Read before changing
//
//   - D193 §2: prepare, the profiles and scheduling.
//   - D4: the third playback path, background-prepared.
//   - D169: the prepared rendition is a cache.
//   - D180: its play state is its source's.
//   - docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - The profile table of cut-a §6 and the FFmpeg argument builder; progress reporting as job
//     records.
package prepare

// SPDX-License-Identifier: GPL-3.0-or-later

// Package probe runs ffprobe on one file, through execx, and turns its output into the probe
// record the core stores verbatim. Hostile bytes stop here: the core receives only the parsed
// record.
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
//   - D126: hostile bytes never reach the core.
//   - D26: FFmpeg is confined.
//   - D112: the node reports its confinement.
//   - docs/contributors/design/plans/scanner.md.
//
// # Where to change
//
//   - The ffprobe arguments and the output parser.
package probe

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
//   - ffprobe reads the input by the file: protocol only, so no filename is read as a URL or a
//     concat list. TestProbe_RunsFFprobeOnTheFileProtocolOnly.
//   - A record is bounded: valid UTF-8, bounded strings, and a file with too many streams or
//     chapters is refused rather than cut short. TestParse_BoundsWhatAHostileFileCanMakeTheNodeReport.
//   - The same output always makes the same record, whatever order Go iterates a map in
//     (scanner.md: deleting the node's cache changes nothing it reports).
//     TestParse_IsTheSameRecordEveryTime.
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
//   - The ffprobe arguments and the output parser: probe.go runs ffprobe, bounded in what it may
//     say; parse.go reads its output into the record, refusing a file past the bounds on streams
//     and chapters.
package probe

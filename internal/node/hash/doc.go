// SPDX-License-Identifier: GPL-3.0-or-later

// Package hash computes a file's payload hash: a sampling hash of the media payload, not of the
// whole file's bytes, so a metadata edit keeps a file's identity and a remux gets a new one.
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
//   - D116: the content hash is a sampling hash.
//   - D179: identity is a hash of the media payload.
//   - docs/contributors/design/plans/scanner.md.
//
// # Where to change
//
//   - The sampling scheme and its version; D179's table becomes its tests in M2.
package hash

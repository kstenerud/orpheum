// SPDX-License-Identifier: GPL-3.0-or-later

// Package scan walks a media root, read-only, and reports what it observes as JSONL records:
// scan_begin, observed, gone, skip, error and scan_complete. It reports; the core decides. Roots
// are configured on the node and never remotely.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx, the forbidigo rules tagged [exec] and [execve], and
//     TestArch_OnlyExecxImportsOsExec.
//   - Removes, renames and truncates nothing. Enforced by the forbidigo rule tagged [delete].
//   - Opens media only through the read-only confine.MediaRoot. Enforced by the forbidigo rule
//     tagged [media-root]; the fixture-hash test (principles §2) lands with the first scan.
//
// # Read before changing
//
//   - D39: reports are JSONL.
//   - D40: error and skip records.
//   - D42: nodes are sensors; the core decides.
//   - D107: a node's roots are set on the node.
//   - D118: the scan is incremental, prioritised and steerable.
//   - docs/contributors/design/plans/scanner.md.
//
// # Where to change
//
//   - The walk and its prioritisation; the report records (their wire types are in
//     internal/nodeproto).
package scan

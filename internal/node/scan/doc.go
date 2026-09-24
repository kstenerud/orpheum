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
//     tagged [media-root]; the fixture-hash test (principles §2) lands with the node process.
//   - Every entry it enumerates gets a record — observed, skip with its rule, or error — in byte
//     order of name. TestScan_ReportsEveryEntryWithAFilingInByteOrder.
//   - A symlink is followed only into a media root, and a directory is walked once however many
//     links reach it (D206). TestScan_FollowsASymlinkOnlyIntoAMediaRoot.
//   - An error keeps its place in the stream; repeats of one code and scope in one directory
//     collapse into a count; the record says who owns what could not be read (D40, D163 §8).
//     TestScan_CollapsesRepeatedErrorsAndSaysWhoCouldNotRead.
//   - A root that keeps failing is abandoned with a root-scoped error (D40).
//     TestScan_StopsARootThatKeepsFailing.
//   - A cancelled scan ends with a run-scoped error and never with scan_complete, so the core
//     applies no removals from it (D39). TestScan_ACancelledScanNeverSaysComplete.
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
//   - scan.go: the walk and the skip rules; errors.go: the error vocabulary and collapsing. The
//     wire types are in internal/nodeproto. Prioritisation (D118) and the probe cache come
//     later; both change cost, never content.
package scan

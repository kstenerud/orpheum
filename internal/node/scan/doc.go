// SPDX-License-Identifier: AGPL-3.0-or-later

// Package scan lists under a media root, read-only, and examines the files the core names (D240):
// a listing reports every entry with its stat and reads no file; an examining probes and hashes a
// video, reads an NFO's bytes and hashes a subtitle. It reports; the core decides. Roots are
// configured on the node and never remotely.
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
//   - A listing lists every entry with its stat, in byte order of name, and reads no file; the
//     library's rules (trash, samples, AppleDouble, extensions) are the core's, and only a link out
//     of the roots and what is neither a file nor a directory are skipped here (D240 §3).
//     TestList_ListsEveryEntryWithItsStatAndReadsNoFile.
//   - A listing goes only as deep as asked, from where asked.
//     TestList_GoesOnlyAsDeepAsAskedFromWhereAsked.
//   - A symlink is followed only into a media root, and a directory is walked once however many
//     links reach it; an examining reads a file through a link where the listing found it (D206).
//     TestList_ListsALinkOnlyIntoAMediaRoot.
//   - An error keeps its place in the listing; repeats of one code and scope in one directory
//     collapse into a count; the record says who owns what could not be read (D40, D163 §8).
//     TestList_CollapsesRepeatedErrorsAndSaysWhoCouldNotRead.
//   - A root that keeps failing is abandoned with a root-scoped error (D40).
//     TestList_StopsARootThatKeepsFailing.
//   - A cancelled listing ends with a run-scoped error and never with scan_complete, so the core
//     removes nothing for it (D39). TestList_ACancelledListingNeverSaysComplete.
//   - A name that is not UTF-8 keeps its bytes, listed and examined (D49).
//     TestListAndExamine_ANameThatIsNotUTF8KeepsItsBytes.
//   - scan_begin carries the root's device and mount facts, and every directory on another device
//     than its parent is reported as a mount record (D42, D208). The facts:
//     TestList_ListsEveryEntryWithItsStatAndReadsNoFile. A mount record needs a mount, which a test
//     cannot make without privileges: the core's handling of both is tested with scripted records
//     (internal/core/library's TestScenario_S7b_*).
//   - An examining answers each file it is given, in turn, with its record or its own error, and
//     waits before each as it is told (D240 §4, §6). TestExamine_ReadsEachFileAsItsKindSaysOrSaysWhyNot,
//     TestExamine_StopsWhenToldToBetweenFiles. An NFO is read to a bound, and a subtitle carries
//     its payload hash. TestExamine_RefusesAnNFOPastTheBound. A video carries its OpenSubtitles
//     moviehash (D224 §3). TestExamine_ReportsEachVideosOpenSubtitlesHash.
//   - A listing moves with each entry it lists, and an examining with each read of a hash too
//     (execx.Moved), so the node tells a reading stalled on its disk from one still working (D233 §5).
//     TestListAndExamine_MoveWithEachRecordAndEachRead.
//
// # Read before changing
//
//   - D40: error and skip records.
//   - D42: nodes are sensors; the core decides.
//   - D107: a node's roots are set on the node.
//   - D240: a scan is a listing the core compares, and the examining of what it needs read.
//   - docs/contributors/design/plans/scanner.md.
//
// # Where to change
//
//   - scan.go: the listing; examine.go: reading the files the core names; errors.go: the error
//     vocabulary and collapsing. The wire types are in internal/nodeproto.
package scan

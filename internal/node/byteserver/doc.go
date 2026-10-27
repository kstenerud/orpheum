// SPDX-License-Identifier: AGPL-3.0-or-later

// Package byteserver serves the node's read-only socket (/run/orpheum/node.sock): GET only, file ranges
// by path and payload hash, and the chunks in the prepared directory. It carries no commands. A request whose
// hash no longer matches the path is refused.
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
//     tagged [media-root].
//   - A file's bytes are served only while its payload hash is the one the request names; a file
//     replaced at its path is refused, even after its identity was remembered. It is opened and
//     checked by internal/node/namedfile, as every command's file is. Enforced by
//     TestServeFile_RefusesAFileThatIsNoLongerTheOneAskedFor.
//   - The payload hashes it remembers are the node's one namedfile.Hashes, which the agent shares
//     (SharingHashes). TestServeFile_RemembersHashesInTheOnesItShares.
//   - Nothing outside a root is served: not a path that climbs out, an absolute path, a
//     directory or a root the node does not have. Enforced by TestServeFile_NamesNothingOutsideARoot.
//   - A store's init and chunks are served only by a name of a store's shape, never its manifest or
//     a run's raw files (D247 §10). Enforced by TestServeStoreFile_ServesAStoresInitAndChunksAndNothingElse.
//   - A result the node holds, too large to post, is served whole by its attempt's token, and
//     nothing for a token it does not hold (D234 §5).
//     TestServeResult_ServesAHeldResultByItsTokenAndNothingElse.
//   - A probe of a place, a root or a directory inside one, answers whether it can be read now, with
//     its facts, opening nothing under it, outside every queue (D327 §1, D328 §6); a path outside
//     every root is refused. A place whose root holds a reading that has not returned is not
//     answering at once, and asks while a probe of a place runs share it, one probe at a time
//     (D327 §2). TestServeProbe_AnswersAReadableRootWithItsFacts,
//     TestServeProbe_AnswersAPlaceInsideARoot,
//     TestServeProbe_ARootHoldingAStuckReadingIsNotAnsweringAtOnce,
//     TestServeProbe_AsksWhileAProbeRunsShareIt.
//   - The folders in a folder, a root or a folder inside one by a clean path, are its real folders, no
//     link and no hidden one, for an admin choosing a library folder (D359 §2); a path no root holds
//     is refused, and one whose share has gone away is not answering within the open window, never
//     waited on, its asks shared as a probe's are.
//     TestServeFolders_ListsTheRealFoldersInAFolderInsideARootAndNothingOutside,
//     TestServeFolders_AShareThatHasGoneIsNotAnsweringWithinTheWindow. A file named under a library
//     folder inside a root is served as one under the root, nothing climbing out of the folder, and
//     its disk keyed by the root (D359 §4).
//     TestServeFile_ServesAFileUnderALibraryFolderInsideARootAndNothingOutsideIt.
//   - GET and HEAD only; Range and If-Range are honoured. Enforced by TestServeFile_OnlyReads and
//     TestServeFile_HonoursRangeAndIfRange.
//
// # Read before changing
//
//   - D193 §1: the core reads bytes from the node's socket.
//   - D100: a command names its target by identity, not by path.
//   - D128: the node keeps what it makes.
//   - D327: the probe, a question answered outside every queue.
//   - docs/contributors/design/plans/cut-a.md §4.
//
// # Where to change
//
//   - byteserver.go: the file route, the chunks' route (D217 §5, D247 §10), and the held results' route.
//   - probe.go: the probe of a root (D327), and the sharing of a read among asks.
//   - folders.go: the folders in a folder (D359 §2).
package byteserver

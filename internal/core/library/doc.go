// SPDX-License-Identifier: GPL-3.0-or-later

// Package library is the library model: Title, Edition, Version and File, extras, collections, and
// what the scanner saw. It runs the scans of the nodes' roots, a listing it compares and the batches
// of files it has the node read (D240), ingests what they find, gives every observed file a filing
// with a reason, and serves browse and search. Identity is the media payload hash, not the path or
// the bytes.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//
//   - Never opens a media path; it knows files only from node reports. Enforced by the forbidigo
//     rule tagged [core-fs]; M2's test runs the core with the media root unreadable to it.
//
//   - The library's rules for what a scan reads are the core's: a listing's trash, samples,
//     AppleDouble files and extensions it does not read are filed skipped, an image beside the media
//     is recorded as listed and not read, and the rest is read (D40, D215, D240 §3).
//     TestTakeListing_FilesEachEntryByTheLibrarysRules,
//     TestTakeListing_RecordsAnImageBesideTheMediaWithoutReadingIt.
//
//   - A file whose size and modified time are as stored, and which was read whole, is seen and not
//     read again (D240 §3). TestTakeListing_AFileAsStoredIsSeenAndNotReadAgain. A listing the node
//     stopped removes nothing (D39). TestTakeListing_AListingThatWasStoppedEndsUnreadable.
//
//   - A scan has its node read what it found new or changed in batches of one folder's videos, each
//     with the files named for it, and completes, applying its removals, once the last is read; a
//     batch given up leaves its files unread, removing none (D240 §8, D40).
//     TestBatchesOf_CutsEachFolderIntoBatchesOfVideosWithTheFilesNamedForThem,
//     TestScans_AScanListsReadsWhatIsNewAndCompletesOnceItIsRead,
//     TestScans_ABatchGivenUpRemovesNoneOfItsFiles, TestScans_AScanWhoseListingFailsIsGivenUp.
//
//   - A root's scan in progress is listed with it: listing, or how many files it found and how many
//     of those new or changed it has read; and whether one more follows (D236 §2).
//     TestAdmin_ListsARootsScanInProgress.
//
//   - A scan asked for while one of its root runs starts none; one more runs after it (D236 §3).
//     TestScans_AScanAskedForWhileOneRunsRunsOnceMoreAfterIt. A scan the core stopped during
//     after taking its listing starts again. TestScans_AScanTheCoreStoppedDuringStartsAgain.
//
//   - Every path a scan enumerated has a row with its filing and reason (D163).
//     TestIngest_EveryEnumeratedPathGetsARowWithItsFilingAndReason.
//
//   - A file is matched by its payload hash: seen again, moved (its row follows it), replaced
//     (the old row tombstoned, D173) or new (D179). TestIngest_D179MatchesAFileByItsPayloadWhereverItIs.
//
//   - A copy is a duplicate, and the recorded path keeps the identity (D179).
//     TestIngest_ACopyIsADuplicateAndTheRecordedPathKeepsTheIdentity.
//
//   - Only a completed scan tombstones, never inside a region it could not read, and it records
//     the root as scanned in the same transaction (D39, D40, schema.md root_scan).
//     TestIngest_OnlyACompletedScanRemovesAndNeverWhereItCouldNotRead.
//
//   - Paths are stored in NFC beside their bytes (D49). TestIngest_APathIsStoredInNFCBesideItsBytes.
//
//   - What a node reports is cleaned where it enters the core: paths (the text; the bytes kept
//     as bytes), track tags, error text, codes and rules (principles §22, D211).
//     TestIngest_WhatANodeReportsIsCleanWhereItEnters. A video's OpenSubtitles hash is kept
//     only as 16 hex digits (D224 §3). TestIngest_KeepsAVideosOpenSubtitlesHashOnlyInItsOwnShape.
//
//   - A file is known by the bytes its node reported, so two names that are one NFC text — two
//     invalid UTF-8 names, or two normalisations of one name — are two files (D211).
//     TestIngest_TwoNamesThatAreOneTextAreTwoFiles.
//
//   - Which root a file is under is judged in one place, RelativeToRoot, by the bytes its node
//     reported, so a root whose name is not NFC holds its files and a root of "/" holds what is
//     below it; the deepest root holds a file when roots nest (D211).
//     TestRelativeToRoot_ComparesTheReportedBytesAndTakesARootOfSlash,
//     TestIngest_UnderARootNotInNFCOrOfSlashAScanMovesRemovesKeepsAndAttaches.
//
//   - A clean scan of an unplugged disk's empty mount point removes nothing: a changed device or
//     mount, or a removal of all of a root's files or more than half and more than twenty, waits
//     for the admin (D41, D208 §3). TestScenario_S7_S8_AnUnpluggedDiskRemovesNothingAndItsReturnRestoresEverything,
//     TestScenario_S7_RemovalsPastTheThresholdWaitAndFewerGoAhead.
//
//   - A disk missing from a mount point inside a root — found by comparing the mount boundaries a
//     scan crossed with the last complete scan's — removes nothing under it (D208, as corrected).
//     TestScenario_S7b_ADiskUnpluggedInsideARootRemovesNothingUnderIt.
//
//   - A file under a root the last scan could not read or withheld, or under a disk missing from a
//     mount point inside one, is not there to read: UnavailableUnder names that root or disk, and
//     work that reads the file parks on it; a complete scan says which root and disks it found, so
//     that work resumes (D231 §5, §6).
//     TestUnavailableUnder_AFileUnderAMissingRootOrDiskWaitsAndAScanThatFindsThemSaysSo.
//
//   - Work that stalls waiting on a root's disk makes the root unavailable from then, and asks for a
//     scan of it, which brings it back (D233 §10).
//     TestScans_AStallOnARootsDiskMakesItUnavailableUntilAScanReadsIt.
//
//   - A library root its node's hello no longer names is unavailable from then, nothing removed
//     (D241 §2). TestScans_ARootTheHelloNoLongerNamesIsUnavailableUntilAScanReadsIt. Whether a file
//     can be read now, its node away or its root or disk missing, has one answer: UnreadableFile.
//
//   - One file that cannot be read now is gone on the admin's word, a tombstone keeping its history;
//     one that can be read is refused (D241 §5). A root that moved is repointed: it and its live
//     files take the new path with their history, their modified times forgotten so the next scan,
//     asked for at once, reads each again and confirms it by its payload hash (D241 §6).
//     TestRetireFile_AFileThatCannotBeReadIsGoneOnTheAdminsWord,
//     TestRepointRoot_ARootThatMovedTakesItsFilesAndHistoryToTheNewPath (internal/core/api).
//
//   - A sidecar is filed as attached to the video it belongs to, and an NFO's bytes are stored as
//     identification's input (D182). TestIngest_ASidecarIsAttachedToTheVideoItBelongsTo.
//
//   - A file observed again keeps its stream rows, changed in place by stream index, and loses
//     only a track its probe no longer has, so an id a job or an address names holds.
//     TestIngest_AFileObservedAgainKeepsItsStreamRowsAndDropsOnlyWhatItLost.
//
//   - A subtitle file keeps its payload hash and, while it belongs to a video, is one track of it,
//     with the language and flags its name gives after the video's name; its row keeps its id
//     across scans. TestIngest_ASubtitleFileIsATrackOfItsVideoUntilItsVideoIsGone,
//     TestSidecarTrackOf_ReadsTheLanguageAndFlagsAfterTheVideosName.
//
//   - A payload that returns revives its tombstone, with its history (D208 §4).
//     TestScenario_S10_AFilmPutBackRevivesItsTombstone.
//
//   - Once a scan's end is committed its library is handed on, to identification.
//     TestIngest_AfterAScanEndsItsLibraryIsHandedOn.
//
//   - An admin's approval lets a root's next complete scan remove what it does not see without
//     waiting, once; a disk the admin says is gone loses its boundary, and its files go with that
//     scan (D208). TestScenario_S7_AnApprovedRemovalGoesAheadOnTheNextScanOnly,
//     TestScenario_S7b_AGoneDiskIsForgottenAndItsFilesRemoved.
//
//   - A root, a node and a library each have a retire action, a library's being its delete, and
//     offline never ends without one or a scan that finds it (D41, D231 §7). A retire tombstones
//     the live files it covers, keeping their history, is scanned no more, and cancels, through
//     the job queue, the work of those files, of their streams and fetched subtitles, of titles
//     left with no live file, and of the destinations it forgets: the roots, the disks mounted
//     inside them, and a node's own. A deleted library's films, series and episodes are
//     tombstoned with no merged_into, hidden with their history. A retired node is forgotten, so
//     its name enrols a new node. TestRetireRoot_TombstonesItsFilesCancelsTheirWorkForgetsItsDisksAndScansItNoMore,
//     TestRetireNode_RetiresItsRootsCancelsItsWorkAndForgetsIt,
//     TestDelete_RemovesTheLibraryItsRootsAndFilesAndCancelsItsWork.
//
//   - A root, or a disk mounted inside one, is offline since the first scan in a row that found
//     it missing, until one finds it. TestOffline_IsSinceTheFirstScanThatFoundARootOrDiskMissingUntilOneFindsIt.
//
//   - Films and series are listed in the sort_locale's order (D47) or newest first, a page at a
//     time behind an opaque cursor, and only those with a live file. TestBrowse_FilmsInTheLocalesOrderAPageAtATime,
//     TestBrowse_FilmsNewestFirst. A title's extras, too, are only those with a live file.
//     TestBrowse_ATitlesExtrasAreOnlyThoseWithALiveFile.
//
//   - A search is folded and routed by internal/core/textindex and answered by the index that
//     can (D48). TestSearch_EveryRouteFindsWhatItShould.
//
//   - A library covers only roots its node configured, as the node's hello named them (D107);
//     creating one scans it. TestAdmin_CreatesALibraryOverTheNodesRootsAndScansIt.
//
//   - Continue Watching shows each film or episode once, ordered by its latest watching, at its
//     most recent position partway through by the resume thresholds, naming the file when more
//     than one is partway; an extra never, and a work hidden since its last watching not at all
//     (play-state.md, D162, D188). Enforced by
//     TestContinueWatching_EachWorkOnceAtItsLatestPositionPartwayThrough and
//     TestContinueWatching_AHiddenWorkLeavesUntilItHasActivityAndUndoBringsItBack.
//
// # Read before changing
//
//   - D79: four levels, Title, Edition, Version, File.
//   - D163: every observed file carries a filing with a reason.
//   - D173: a replaced file is a deletion and an addition; tombstones, not deletes.
//   - D179: a file's identity is a hash of its media payload.
//   - D42: nodes are sensors; the core is the only thing that decides.
//   - D240: a scan is a listing the core compares, and the batches it has the node read.
//   - docs/contributors/design/plans/library-model.md, docs/contributors/design/plans/scanner.md.
//
// # Where to change
//
//   - scans.go: a scan's jobs, from its request to its end; compare.go: a listing compared with what
//     is stored, by the library's rules; batches.go: the batches a scan's node reads;
//   - ingest.go, observe.go, complete.go, sidecar.go: the ingest; streams.go: the stream rows a probe
//     derives; create.go: libraries; admin.go: what an admin does with them (create over a node's
//     roots, list, scan, the scanner view); browse.go and detail.go: lists, a title's detail, a
//     series' episodes, and search; home.go: Continue Watching and the resume thresholds;
//     subtitle_sidecar.go: a subtitle file's track; rootpath.go: which root a file is under, and a
//     path's text; unavailable.go: the missing root or disk a file is under; vocabulary.go: the
//     filings, title kinds and image kinds identification shares; filing.go: FileAs and
//     PutInVersion, through which identification records how a file is filed and which version
//     holds it (this package writes the file table, schema.md).
package library

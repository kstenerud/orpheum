// SPDX-License-Identifier: GPL-3.0-or-later

// Package library is the library model: Title, Edition, Version and File, extras, collections, and
// what the scanner saw. It ingests the node's reports, gives every observed file a filing with a
// reason, and serves browse and search. Identity is the media payload hash, not the path or the
// bytes.
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
//     TestIngest_WhatANodeReportsIsCleanWhereItEnters.
//
//   - A file is known by the bytes its node reported, so two names that are one NFC text — two
//     invalid UTF-8 names, or two normalisations of one name — are two files (D211).
//     TestIngest_TwoNamesThatAreOneTextAreTwoFiles.
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
//
//   - Once a scan's end is committed its library is handed on, to identification.
//     TestIngest_AfterAScanEndsItsLibraryIsHandedOn.
//
//   - An admin's approval lets a root's next complete scan remove what it does not see without
//     waiting, once; a disk the admin says is gone loses its boundary, and its files go with that
//     scan (D208). TestScenario_S7_AnApprovedRemovalGoesAheadOnTheNextScanOnly,
//     TestScenario_S7b_AGoneDiskIsForgottenAndItsFilesRemoved.
//
//   - Films and series are listed in the sort_locale's order (D47) or newest first, a page at a
//     time behind an opaque cursor, and only those with a live file. TestBrowse_FilmsInTheLocalesOrderAPageAtATime,
//     TestBrowse_FilmsNewestFirst.
//
//   - A search is folded and routed by internal/core/textindex and answered by the index that
//     can (D48). TestSearch_EveryRouteFindsWhatItShould.
//
//   - A library covers only roots its node configured, as the node's hello named them (D107);
//     creating one scans it. TestAdmin_CreatesALibraryOverTheNodesRootsAndScansIt.
//     TestScenario_S10_AFilmPutBackRevivesItsTombstone.
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
//   - docs/contributors/design/plans/library-model.md, docs/contributors/design/plans/scanner.md.
//
// # Where to change
//
//   - ingest.go, observe.go, complete.go, sidecar.go: the report ingest; streams.go: the stream rows a probe
//     derives; create.go: libraries; admin.go: what an admin does with them (create over a node's
//     roots, list, scan, the scanner view); browse.go and detail.go: lists, a title's detail, a
//     series' episodes, and search; home.go: Continue Watching and the resume thresholds;
//     subtitle_sidecar.go: a subtitle file's track.
package library

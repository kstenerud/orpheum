// SPDX-License-Identifier: AGPL-3.0-or-later

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
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//
//   - Never opens a media path; it knows files only from node reports. Enforced by the forbidigo
//     rule tagged [core-fs]; M2's test runs the core with the media root unreadable to it.
//
//   - The library's rules for what a scan reads are the core's: a listing's trash, samples,
//     AppleDouble files, the folders a NAS or an operating system keeps for itself, a disc's folder
//     (D306 §5) and extensions it does not read are filed skipped, a disc's image is read as a video
//     whatever the extensions (D350), an image beside the media is recorded as listed and not read,
//     and the rest is read (D40, D215, D240 §3).
//     TestTakeListing_FilesEachEntryByTheLibrarysRules,
//     TestTakeListing_SkipsTheFoldersANASOrAnOperatingSystemKeepsForItself,
//     TestTakeListing_ReadsADiscsImageAndSkipsADiscsFolder,
//     TestTakeListing_RecordsAnImageBesideTheMediaWithoutReadingIt.
//     The extensions it reads as videos and as subtitles are the code's, not settings, whatever a
//     stored row of the retired settings says (D360). TestReadRules_ReadTheOlderContainersByDefault.
//
//   - A file whose size and modified time are as stored, and which was read whole, is seen and not
//     read again (D240 §3). TestTakeListing_AFileAsStoredIsSeenAndNotReadAgain. A video listed at a
//     new path on the device and inode a stored one was read at, its size and modified time as
//     stored, moved there, and keeps its row without a read (D304 §5).
//     TestTakeListing_AVideoMovedOnItsDiskKeepsItsRowWithoutBeingReadAgain.
//
//   - A scan that reads files again, a full rescan's (D392 §2), reads each it names (ReadAgain: a
//     library's every file, a film's or series' own videos), changed on disk or not, and every NFO and
//     subtitle file it lists; what it does not name, stored as listed, is seen and not read. What it
//     reads again is kept with the scan in cache.db, through an ask that joins it and in the scan that
//     follows it. TestTakeListing_AScanReadingAgainReadsWhatItNamesAndEveryNFOAndSubtitle,
//     TestScans_WhatAScanReadsAgainSurvivesAJoinAndTheScanAfter,
//     TestRescan_AFullRescanReadsItsFilesAgainChangedOnDiskOrNot.
//
//   - Each video a full rescan reads again, already stored, is examined with its file's id to check
//     (nodeproto.ExamineFile.ChecksChunksOf): its node checks every chunk of the file's stores against
//     its record and drops what is missing or damaged, and the scan counts what it checked and dropped
//     in its totals and logs at warn each file it dropped chunks of; an ordinary scan checks nothing,
//     a file changed on disk among them (D392 §4, §5). The core keeps nothing of a chunk dropped: the
//     node answers it not made, and it is made again when asked for (chunks.Service.Want).
//     TestScans_AFullRescanHasItsNodeCheckTheChunksOfEachVideoItReadsAgain.
//
//   - What is kept that was made from a file's probe, and from which of its facts, is said in one
//     place, derivedFromProbe (derived.go), so a new derived thing declares its facts there and the
//     server's wiring how it is thrown out (DerivedDroppers, wired whole:
//     TestBuild_EveryThingDerivedFromAProbeCanBeThrownOut, internal/core/assembly):
//
//     the picture's chunks (cache.db), from its container's formatNames and its main picture's
//     streamIndex, codec, codecProfile and video facts; each audio track's chunks (cache.db), from its
//     container's formatNames and the track's streamIndex, codec, codecProfile and audio facts; each
//     audio track's loudness, from the track's streamIndex, codec, codecProfile and audio facts; the
//     picture's timeline, from its streamIndex and codec; each own subtitle track's text, from its
//     streamIndex and codec; the frame stills, from the container's durationMs and the picture's
//     streamIndex, width and height.
//
//     A probe read again is compared with the one kept fact by fact (a fact at its zero value is
//     none), and each derived thing whose facts differ is thrown out by the package that keeps it,
//     persistent.db's in the write that keeps the new probe, cache.db's after it; nothing else, and
//     nothing a person set or watched (D357 §3). Each correction is logged at info, the file, the facts
//     that differed and what was thrown out, and a scan's totals at its end; probes alike change
//     nothing (D392 §3, §5). TestScans_AFileReadAgainThrowsOutOnlyWhatItsChangedFactsTouch,
//     TestScans_ASecondFullScanOfAnUnchangedLibraryChangesNothing,
//     TestDerivedFromProbe_EachThingDeclaresItsFactsAndOneWayToThrowItOut. A store of chunks also
//     names the facts it was made from (PictureChunksFactsKey, SoundChunksFactsKey; chunks.StoreKey),
//     so one made before they changed is never served, whoever read the file again.
//     TestFactsKey_ChangesWithTheFactsAStreamsChunksAreMadeFromAlone.
//
//   - A new video is stored at its listing, unread, so it is filed from its path before it is read;
//     its read fills that row in, or, finding a file stored before, moved or come back, retires it,
//     what identification filed it in going with it (D304 §1, §5).
//     TestTakeListing_ANewVideoIsStoredUnreadAndItsReadFillsItsRow,
//     TestTakeListing_AVideoMovedToAnotherDiskReadAtItsNewPathKeepsItsRow. A title whose files are
//     only unread is still reading, not playable, and a play of one asks for its read before what
//     else waits (D304 §4). TestReadNow_RaisesTheBatchThatReadsAFileOrScansItsFolder. The files a listing
//     finds as stored are marked seen in one statement, not one each (D303, DL7).
//     TestTakeListing_OfFilesAllAsStoredRunsTheSameStatementsAtTwiceTheirNumber. A listing the node
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
//   - A root (a node and an absolute path) has one scan in progress, however many libraries cover it:
//     a scan asked for of it, by any way of asking, joins it while its listing is not with the node,
//     widening its scope to cover both at the higher priority, and the root is listed once.
//     TestScans_AnAskForARootWhoseListingIsNotWithItsNodeJoinsIt,
//     TestScans_FoldersAskedForWhileTheListingWaitsAreListedOnceByTheFolderHoldingThem,
//     TestScans_TwoLibrariesOverOneRootPathMakeOneListingAndReadEachFileOnce. A node's hello's ask
//     also joins a listing with the node that covers it, which went to the start that said hello.
//     Otherwise one asked for while one of its root runs starts none; one more runs after it (D236
//     §3). TestScans_AHellosAskJoinsTheListingWithItsNodeAndOtherAsksFollowIt,
//     TestScans_AScanAskedForWhileOneRunsRunsOnceMoreAfterIt. A scan the core stopped during
//     after taking its listing starts again. TestScans_AScanTheCoreStoppedDuringStartsAgain.
//
//   - A root of a library checked on an interval is scanned once its check_every_hours has passed
//     since its last scan, and a watched one's once watched_library_scan_every_hours has, beneath
//     the watching, which can miss a change unseen (D27, D272 §1); a root checked only when asked
//     never is. TestDueRoots_ARootIsDueOnceItsLibrariesIntervalHasPassedSinceItsLastScan,
//     TestScanSchedule_AsksForTheDueScansOfLibrariesThatCheckByThemselves.
//
//   - A scan of a folder, a watcher's notice (D272 §9), removes nothing outside it, counts a copy
//     there as a second file and never as the file outside it moved, and leaves the root's last
//     scan and facts the whole scan's. TestScans_AScanOfAFolderRemovesOnlyWhatItCovers,
//     TestScans_ACopyInTheFolderIsADuplicateNotAMoveOfTheFileOutsideIt. One that could not read its
//     folder, would wait for the admin, lacks a disk mounted below it, or is under a root whose last
//     scan was not complete becomes a scan of the whole root, removing nothing itself.
//     TestScans_AScanOfAFolderThatMayNotJudgeBecomesTheWholeRoots. Folders asked for while a scan
//     runs are scanned once after it, by the folder holding them all.
//     TestScans_FoldersAskedForWhileOneRunsAreScannedOnceByTheFolderHoldingThem.
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
//   - A scan that would remove nothing completes though its root's device or mount changed, and
//     the new one is recorded (D276).
//     TestScenario_ARootOnANewDeviceWithEveryFileCompletesAndKeepsTheNewDevice.
//
//   - A scan held back for the admin that read the root leaves it available; only its removals
//     wait, and Waiting names them for the System page, as UnreadPaths names what scans could not
//     read (D277).
//     TestScenario_AHeldBackScanThatReadTheRootLeavesItPlayableAndOneThatSawNoneDoesNot,
//     TestSystem_SaysWhatAHeldBackScanWouldRemoveAndWhatCouldNotBeRead.
//
//   - A disk missing from a mount point inside a root — found by comparing the mount boundaries a
//     scan crossed with the last complete scan's — removes nothing under it (D208, as corrected).
//     TestScenario_S7b_ADiskUnpluggedInsideARootRemovesNothingUnderIt.
//
//   - Availability is kept per place, a root or a disk mounted inside one, in unavailable_place,
//     with since when and why (D328): a root the last scan could not read, or whose held-back scan
//     saw none of it, a disk missing from a mount point inside one, a place whose disk did not
//     answer, a root its hello no longer names. A file under one is not there to read:
//     UnavailableUnder names the deepest, read from that table alone (unavailableOfNode), and work
//     that reads the file parks on it; a complete scan says which root and disks it found, so that
//     work resumes (D231 §5, §6).
//     TestUnavailableUnder_AFileUnderAMissingRootOrDiskWaitsAndAScanThatFindsThemSaysSo,
//     TestScans_ARootAnUnreadableScanLeftIsBroughtBackToo. Only this package writes the table
//     (TestArch_EachTableHasOneWritingPackage), and within it only availability.go (review).
//
//   - A place unavailable comes back without a scan when its disk answers with the facts its last
//     complete scan recorded, and a scan follows in the background; on other facts it is the empty
//     directory an unmounted disk leaves, and waits for a scan (D326, D327, D328 §6).
//     TestScans_AnUnavailableRootWhoseDiskAnswersAgainAsItWasIsBroughtBackAtOnce,
//     TestScans_AnUnavailableRootOnAnotherDiskIsNotBroughtBack.
//
//   - Work that stalls waiting on a disk makes the deepest place the scans know that holds what it
//     waited on unavailable from then, a disk mounted inside the root or else the root, found from
//     the scans' record of the mounts, and asks for a scan of the root, which brings it back (D233
//     §10, D328 §5). TestScans_AStallOnARootsDiskMakesItUnavailableUntilAScanReadsIt,
//     TestScans_AStallOnADiskInsideARootMakesOnlyThatDiskUnavailable.
//
//   - A library root its node's hello no longer names is unavailable from then, nothing removed
//     (D241 §2). TestScans_ARootTheHelloNoLongerNamesIsUnavailableUntilAScanReadsIt. Whether a file
//     can be read now, its node away or its root or disk missing, has one answer: UnreadableFile.
//
//   - A playable file, its probe, where its bytes are on its node's byte socket, and whether a job
//     reading it waits, ends or goes ahead have one answer each, which the play, the runs of chunks, the
//     extractions and the measurements share (playable.go): PlayableFile, ProbeOf, BytesFileOf,
//     RefuseUnreadable, ParkIfUnavailable. A file under a root of "/" is named by its root:
//     TestBytesFileOf_NamesAFileUnderARootOfSlash.
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
//   - A payload that returns revives its tombstone, with its history (D208 §4), wherever it is put
//     back, another file now at its old path or not. TestScenario_S10_AFilmPutBackRevivesItsTombstone,
//     TestScenario_AFilmPutBackElsewhereWhileItsOldPathIsTakenRevivesItsTombstone.
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
//     offline never ends without one or a scan that finds it (D41, D231 §7). A retired folder or a
//     deleted library is scanned no more and deletes no item: the places no other library covers are
//     left to none, kept with what was learned and watched until a library covers them again or the
//     existing budgets reap them, oldest first (D363 §10); the work of those files, of their streams
//     and fetched subtitles, and of the destinations it forgets (the roots, the disks mounted inside
//     them) is cancelled through the job queue. A retired node's places are tombstoned, a file's first
//     place handing it to a live copy elsewhere, and the node is forgotten, so its name enrols a new
//     node. TestRetireRoot_LeavesItsFilesToNoLibraryCancelsTheirWorkForgetsItsDisksAndScansItNoMore,
//     TestRetireNode_RetiresItsRootsCancelsItsWorkAndForgetsIt,
//     TestDelete_RemovesTheLibraryAndItsRootsKeepsItsFilesAndCancelsTheirWork.
//
//   - A file is found at one or more places, a node and a path (D363 §2): a video read at a second
//     place whose payload a live video found before it has is a candidate, both are read whole (D116),
//     and the same bytes make it the first's place from then on, its tracks, filings and what other
//     packages hold of it the file's, the subtitles beside it its place's; other bytes leave two files.
//     Each is logged. When a file's first place goes, a live copy's place is the file's.
//     TestCopies_AFileFoundAtASecondPlaceWithTheSameBytesIsOneFileAtTwoPlaces,
//     TestCopies_AFileWithTheSamePayloadAndOtherBytesStaysAFileOfItsOwn,
//     TestRemovePlace_AFilesFirstPlaceGoingLeavesItAtItsCopysPlace.
//
//   - Libraries may overlap, and each place is scanned and watched once: a folder inside another
//     library folder on its node is scanned within the outermost, of its scope, and a node watches the
//     outermost of its watched folders (D363 §1, §2).
//     TestScans_AFolderInsideAnotherLibrarysFolderIsScannedOnceWithinIt,
//     TestScans_AnAskForAnInnerFolderIsItsScopeWithinTheOuter,
//     TestScanSchedule_ANodeWatchesAFolderInsideAnotherOnlyThroughIt,
//     TestScanRootOf_IsTheOutermostFolderAtOrAboveWithTheScopeWithinIt.
//
//   - A person reaches an item when a library they reach covers a place of one of its files; lists
//     show each item once; a detail names only the places they reach, an admin's every place; a play
//     reads from a place they reach, the first before a copy, and falls back only among those; a
//     subtitle beside a copy is offered where that place is reached (D363 §3, §7, §8).
//     TestBrowse_AFilmAtTwoPlacesOfTwoLibrariesIsListedOnceToWhoeverReachesEither,
//     TestBrowse_ATitleShowsOnlyThePlacesItsPersonReaches,
//     TestPlaceToRead_IsAReachedPlaceAndFallsBackOnlyAmongThem,
//     TestSubtitleTracks_ASubtitleBesideACopyIsOfferedWhereItsPlaceIsReached.
//
//   - A place at which a read found its file not there, renamed or deleted, is read last of the
//     places its reader reaches until a complete scan of it sees the file there again (Absences, the
//     ingester's, in memory), and asks for a scan of its folder (D363 §8).
//     TestPlaceToRead_AfterAReadFindsTheFileNotAtAPlaceItReadsAnotherUntilAScanSeesItThere,
//     TestAbsences_APlaceNotedAsksForAScanOfItsFolder.
//
//   - A root, or a disk mounted inside one, is offline since the first scan in a row that found
//     it missing, until one finds it. TestOffline_IsSinceTheFirstScanThatFoundARootOrDiskMissingUntilOneFindsIt.
//
//   - Films and series are listed in the sort_locale's order (D47) or newest first, a page at a
//     time behind an opaque cursor, and only those with a live file. TestBrowse_FilmsInTheLocalesOrderAPageAtATime,
//     TestBrowse_FilmsNewestFirst. Newest first is by the scan that found it, most recent first,
//     then by its file's own time: a film as the scan that first found it, a series as its newest
//     episode (the owner, 2026-10-25; the store's step 0058).
//     TestFound_FilmsNewestFirstByTheScanThatFoundThemThenTheirFilesTime,
//     TestFound_ASeriesIsAsNewAsItsNewestEpisode (internal/store). A title's extras, too, are only
//     those with a live file. TestBrowse_ATitlesExtrasAreOnlyThoseWithALiveFile.
//
//   - A list is of one library alone, or of every library its person reaches, less, for films and
//     series, the libraries they left out; a library they do not reach is no such library, not an
//     empty one (D349 §1, §8). Every kind together is one list in one order, each kind read from
//     the same place and merged. Other videos come newest recorded first, by name, by file name or
//     newest added first; films and series by name, newest added or newest released, a film by its
//     release date and a series by its first air date (D349 §4, D366 §1).
//     TestBrowse_OneLibrarysListAndALibraryNotReached, TestBrowse_ALibraryLeftOutOfFilmsAndSeries,
//     TestBrowse_OtherVideosInTheirFourOrders (internal/core/api).
//
//   - Every sort runs either way, its own way by default, A to Z by a name and newest first by a
//     date, read by the same index the other way (browse_orders.sql); what has no release date
//     comes after every dated one whichever way, a page at a time across it (D366 §2, §3).
//     TestBrowse_EverySortRunsEitherWayAndWhatHasNoDateIsLast, TestBrowse_OtherVideosRunEitherWay
//     (internal/core/api). Z to A, a name starts the list at the last title before it (D366 §4).
//     TestBrowse_ZToAFromANameStartsAtTheLastTitleBeforeIt (internal/core/api).
//
//   - The alphabet rail's letters are those a list by name or file name has a title under in its
//     order, whichever way it runs, found by a read from each letter that has one, never the list
//     (D349 §9).
//     TestBrowse_TheRailsLettersAreThoseTheListHasTitlesUnder (internal/core/api).
//
//   - An other library's folders are its files' folders below its roots, every root's together,
//     each with how many videos it holds; no other kind of library is browsed by folder (D349 §5).
//     TestBrowse_AnOtherLibrarysFolders (internal/core/api).
//
//   - A list's entry shows how far its person is through it, in a constant number of statements
//     for its page: a film where Continue Watching would resume it, by the same rule (resumesAt);
//     a series how many of its episodes with a file they watched, of how many, in the ordering
//     they watch it in. TestBrowse_AListShowsHowFarThePersonIsThroughEach (internal/core/api).
//
//   - Whether a person watched a title is read from its kept mark, never from where they are now
//     (D370 §1, §5): a list's film, other video and series, a title's page, a season and its
//     episodes, Continue Watching's rewatch, a series' progress and Next Up all read it, so a finished
//     film or episode started again stays watched; a series or season is watched once every episode
//     with a file is (§3). A position watched at or before the title's last mark by hand counts for
//     nothing: not partway, not where it resumes (§4, IsBeforeMark).
//     TestWatched_APlayToItsFinishMarksItWatchedAndARestartKeepsIt,
//     TestWatched_RestartingAFinishedEpisodeKeepsTheSeriesWhereItWas,
//     TestWatched_ASeasonOrASeriesIsMarkedByHandAndCheckedOnceEveryEpisodeIs,
//     TestWatched_AMarkByHandIsAFreshStartAndUndoPutsItBack (internal/core/api),
//     TestMembersOf_ReadThePersonsPositionsAgainstTheThresholds. What a person may mark by hand is
//     answered here: a film, episode or other video they reach, never an extra (TitleToMark), and a
//     series' or a season's episodes with a file in the ordering they watch it in, present now
//     (EpisodesToMark). TestWatched_AnExtraIsNeverMarkedAndAnUndoCannotBeOfTheFuture,
//     TestWatched_ASeriesNotReachedIsNoSuchSeriesToMark (internal/core/api).
//
//   - A search is folded and routed by internal/core/textindex and answered by the index that
//     can (D48). TestSearch_EveryRouteFindsWhatItShould.
//
//   - A library covers only roots its node configured, as the node's hello named them, or folders
//     inside them by a clean path (D107, D359 §1); libraries may overlap, and within one library a
//     folder beneath another of its own is refused as covered, one above them replacing them (D363
//     §1); creating one, or adding a folder to one, scans it (D359 §3).
//     TestAdmin_CreatesALibraryOverTheNodesRootsAndScansIt,
//     TestAdmin_ALibraryFolderIsAConfiguredRootOrAFolderInsideOneByACleanPath,
//     TestAdmin_LibrariesMayOverlapAndWithinOneAFolderBeneathAnotherAddsNothing,
//     TestRepointRoot_MovesAFolderToAFolderInsideAConfiguredRoot. A folder inside a configured root is
//     listed and told of its watcher's notices as the root's own was (D359 §4), and shown with its own
//     watch, never its configured root's. TestAdmin_AFolderInsideAConfiguredRootIsListedWithItsOwnWatch,
//     TestScanSchedule_AWatchersNoticeUnderARootScansTheLibraryFoldersInsideItThatItTouches.
//
//   - A node watches each live folder of each live library on it that watches for changes, and nothing
//     else, and every change of them (a library made, deleted or set to watch or not, a folder added,
//     moved or retired) tells the nodes at once (D359's amendment of 2026-10-27).
//     TestScanSchedule_ANodeWatchesTheFoldersOfItsWatchedLibrariesAndEachChangeIsSaid.
//
//   - An admin's listing of a node's folders asks the node, for a folder a configured root holds, and
//     waits for it no longer than folderListingWait, a share gone away answered not answering (D359
//     §2); each is logged at debug, a refused folder at warn (§5).
//     TestAdmin_ListsANodesFoldersInsideItsRootsWithinTheWait.
//
//   - Continue Watching shows each film or episode once, ordered by its latest watching, at its
//     most recent position partway through by the resume thresholds, naming the file when more
//     than one is partway; an extra never, and a work hidden since its last watching not at all
//     (play-state.md, D162, D188), nor a position from before its last mark by hand (D370 §4). Enforced by
//     TestContinueWatching_EachWorkOnceAtItsLatestPositionPartwayThrough and
//     TestContinueWatching_AHiddenWorkLeavesUntilItHasActivityAndUndoBringsItBack.
//
//   - Next Up is a query over a collection's named ordering, never over "a series" (D159, D160): the
//     member its person is partway through, the latest watched of several; else the one after the
//     latest watched to its finish or marked so, the later in the ordering of several marked at once
//     (D370); else the first; none past the last, by the resume thresholds. One rule,
//     MembersAhead, serves the series' page's next episode and what interest raises (D324 §3).
//     TestMembersAhead_StartAtThePersonsPlaceInTheOrdering,
//     TestMembersAheadInGroup_StartAtThePersonsPlaceInTheGroup,
//     TestMembersOf_ReadThePersonsPositionsAgainstTheThresholds.
//
//   - What plays after a file (UpNext) is the member after its title in the ordering its person
//     watches its series in, offered as Next Up's is, its partway file first; none for a film, an
//     episode that ordering leaves out, or the last (plans/up-next.md). Enforced by
//     TestUpNext_IsTheNextEpisodeInThePersonsOrdering (internal/core/api).
//
//   - Which ordering a person sees a series in is answered in one place, ViewingOrdering: their
//     choice, the preference episode_ordering at the series' work scope, when the series still has
//     it; else aired, the files' order until an admin can name another (D159 §1, D210 §5;
//     episode-orderings.md §7 Q3). The series' page's seasons, a season's page, an episode's label
//     on its page and in Continue Watching, Next Up and what interest raises all follow it; aired's
//     specials come last, another ordering's groups by number. Choosing one moves no play position
//     or event (D159's first assertion, D157). Enforced by
//     TestOrderings_APersonsChosenOrderingIsTheSeriesTheySee (internal/core/api).
//
//   - A series' page offers every ordering it has, aired first, each with its kind, label and how
//     many of its episodes it holds; a group carries its ordering's name for it (group_text by
//     ordering); the episodes the ordering in use leaves out are listed apart at their aired places,
//     never as Next Up (episode-orderings.md §4, §7 Q4). Enforced by
//     TestOrderings_APersonChoosesAmongTheOrderingsASeriesHas (internal/core/api).
//
//   - An item shows any image of a kind before a frame still of its video, which stands in only where
//     no other is; an other video's 16:9 picture is its still, else its backdrop, else its frame still;
//     a film or series with no poster shows, in its poster's place, its backdrop, else a film's frame
//     still or a series' first episode's still, read anew with each page (D349 §3, D362 §2). Enforced
//     by TestWithLandscapes_AFrameStillShowsOnlyWhereNoOtherPictureIs and
//     TestWithPosters_APosterlessFilmOrSeriesShowsAStillUntilAPosterArrives.
//
//   - A series' aired seasons each show their own poster where one is stored, never as the series'
//     own; another ordering's groups, discs or parts, show none (D379 §1).
//     TestBrowse_ASeriesSeasonsShowTheirOwnPostersInAiredOrderAlone (internal/core/api).
//
//   - A removed file is deleted removed_files_kept_days after it went, by a daily sweep, in short
//     writes, after each package that holds something of it has dealt with it; a video goes only
//     once no sidecar of it stays, and its title and version stay (D304 §6, D306 §8).
//     TestPurgeRemovedFiles_KeepsWhatWasWatchedOnTheVersion (internal/core/playback).
//
// # Read before changing
//
//   - D79: four levels, Title, Edition, Version, File.
//   - D163: every observed file carries a filing with a reason.
//   - D173: a replaced file is a deletion and an addition; tombstones, not deletes.
//   - D179: a file's identity is a hash of its media payload.
//   - D42: nodes are sensors; the core is the only thing that decides.
//   - D240: a scan is a listing the core compares, and the batches it has the node read.
//   - D360: the extensions a scan reads are the code's, each container proven to play.
//   - D392: a full rescan reads every file again and throws out only what the differences touch.
//   - docs/contributors/design/plans/library-model.md, docs/contributors/design/plans/scanner.md.
//   - Every list, search and detail a person asks for holds only the libraries their reach reaches,
//     and one not reached is answered as one that does not exist: the lists in SQL, a detail and a
//     request about one file or item by ReachFile and ReachItem, an image by ReachArt (D243 §4).
//     TestReach_ALibraryTakenBackIsGoneAtTheNextRequestEverywhere.
//
// # Where to change
//
//   - reach.go: whether a person reaches a file, an item or an image (D243 §4).
//   - scans.go: a scan's request, its listing and its jobs' assignees; scans_records.go: taking a
//     scan's listing and its batches' records, and finishing it; schedule.go: the scans libraries are
//     due; scope.go: a scan of a folder; compare.go: a listing compared with what is stored, by
//     the library's rules; extensions.go: the extensions a scan reads; batches.go: the batches a scan's
//     node reads;
//   - ingest.go, observe.go, complete.go, sidecar.go: the ingest; streams.go: the stream rows a probe
//     derives; create.go: libraries; admin.go: what an admin does with them (create over a node's
//     roots, list, scan, the scanner view); browse.go and detail.go: lists, a title's detail, a
//     series' episodes, and search; progress.go: how far a person is through a list's entries;
//     lists.go, lists_other.go, lists_released.go: each list's reads, either way (D366);
//     watched.go: whether a person watched titles, a mark's fresh start, and what they may mark by
//     hand (D370);
//     home.go: Continue Watching and the resume thresholds;
//     next_up.go: a collection's members as a person stands in them, their Next Up, and what plays
//     after a file (UpNext);
//     ordering.go: the ordering a person sees a series in, and an episode's place in it;
//     subtitle_sidecar.go: a subtitle file's track; rootpath.go: which root a file is under, and a
//     path's text; availability.go: the one writer of a place's availability (D328); places.go:
//     the scans' answers to it, a probe, a stall, a bring-back;
//     unavailable.go: the unavailable root or disk a file is under; vocabulary.go: the
//     filings, title kinds and image kinds identification shares; filing.go: FileAs and
//     PutInVersion, through which identification records how a file is filed and which version
//     holds it (this package writes the file table, schema.md).
//   - art.go: the images an item shows; clean.go: cleaning a node's text where it enters (D211);
//     readable.go: why a file cannot be read now (D241 §3); retire.go: retiring a node, a root
//     or a file (D231 §7, D241 §5); move.go: a root that moved, repointed (D241 §6);
//     rescan_scans.go: the scans a rescan asks for, of a library or of the folders holding one
//     film's or series' files, and whether they are over (D357 §2); read_again.go: what a full
//     rescan's scan reads again (D392 §2); derived.go: what is made from a probe, from which facts,
//     and how a probe read again corrects it (D392 §3, §5).
//     libraryfolders.go: which folders a library may have, adding one, and listing a node's folders
//     (D359).
package library

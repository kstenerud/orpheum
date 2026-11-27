// SPDX-License-Identifier: AGPL-3.0-or-later

// Package trickplay has each live video's scrub-bar pictures made once by its node, one picture of
// every 10 s at its keyframe, 320 px wide, in sheets of 10 × 10 (trickplay.md), and keeps them by the
// file's payload hash and trickplay version in cache.db, since they are rebuilt from the file: each
// run's end in trickplay, each sheet in trickplay_sheet, its image content-addressed by SHA-256 in
// the cache directory's trickplay folder (instancedir). It owns the trickplay job, its sweep, the
// sweep that forgets the pictures of files gone, and its node's sheets and answer.
//
// # Invariants
//
//   - Each live video with a picture is asked once per payload hash and version, a file of covers
//     never, at background priority behind every cheaper pass; once made, or failed, its bytes are
//     not asked again. TestTrickplay_EachVideoHasItsPicturesMadeOnceAndEachSheetKeptAsItArrives
//     (internal/core/api).
//
//   - Each sheet a node posts is kept as it arrives, its image before its row, so a row never names
//     an image not there; a sheet posted again changes nothing, and one for a run the core no longer
//     holds is refused, which stops its node. A run refused or lost keeps its sheets, and the next
//     starts after them (nodeproto.MakeTrickplay.SheetsMade).
//     TestTrickplay_ARunStoppedPartWayResumesAfterTheSheetsKept (internal/core/api),
//     TestTrickplay_ASheetIsTakenOnlyForAnAttemptTheCoreHolds and
//     TestTrickplaySheet_IsAcknowledgedOnceKeptAndRejectedForARunNoLongerWanted (internal/core/nodes).
//
//   - Only FFmpeg's word on a file, or two stalls in a row, is kept as its bytes' failure; a refusal
//     or one stall is tried again (D244 §1).
//     TestTrickplay_FFmpegsFailureAndTwoStallsInARowAreKeptAndNotAskedAgain (internal/core/api).
//
//   - A file under a root or disk that is unavailable is not swept, and its job parks on the root
//     (library.Unavailability, D231 §5). TestTrickplay_AFileUnderAMissingRootIsNotSwept
//     (internal/core/api).
//
//   - A raised trickplay moves in the queue only: its node is told a priority below a person's and
//     never play-driven, so its run never takes the CPU or the disk from the playback that raised it
//     (trickplay.md §4, question 4).
//     TestTrickplay_ARaisedRunIsNeverSentPlayDrivenNorAtAPersonsPriority (internal/core/nodes).
//
//   - The pictures are forgotten, with every image no other row names, when the probe facts they were
//     made from change (library.DerivedDroppers.Trickplay, D392 §3) and once no file names their
//     bytes; an image no row names, as a cache.db thrown out leaves, is removed by the same sweep.
//     TestTrickplay_PicturesOfChangedOrRemovedBytesAreForgottenWithTheirImages (internal/core/api),
//     TestScans_ADiscardingRescanThrowsOutEverythingMadeOfAnUnchangedFile (internal/core/library).
//
// # Read before changing
//
// docs/contributors/design/plans/trickplay.md; D231 (jobs as facts), D244 §1 (failures kept), D324
// (interest's raise), D392 §3 (derived things dropped), D530 (the cache's eviction, not yet built).
//
// # Where to change
//
//   - trickplay.go: the job, what a file's bytes lack, the sweep, the sheets a node posts and its
//     answer, and how each is kept.
//   - forget.go: the pictures forgotten when a file's facts change or no file names its bytes, and
//     the images no row names.
package trickplay

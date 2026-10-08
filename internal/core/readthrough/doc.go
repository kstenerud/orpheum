// SPDX-License-Identifier: GPL-3.0-or-later

// Package readthrough has each video read through once by its node, one FFmpeg reading the whole
// file, and keeps what the read learned by the file's payload hash, in persistent.db, since making
// it again reads the whole file (the architecture map's F1): how loud each audio track is, in
// loudness (D226), which internal/core/loudness turns into gains; and its main picture's timeline,
// its keyframes and its bytes in each second, in video_timeline (D247 §10), which
// internal/core/chunks cuts films by. Both parts of an answer are kept in one write. A read asks
// only for what the bytes lack, so one whose loudness is kept and whose timeline is not reads only
// the picture. It owns the read_through job, its sweep and its node's answer.
//
// # Invariants
//
//   - A read-through a player waits on asks for the picture's timeline from the file's index alone;
//     a timeline kept so, its keyframes without its bytes in each second, is read through still by
//     the sweep, which adds the bytes and keeps the keyframes (D259 §2). A file with no index says
//     so, which is remembered in memory, keeping nothing; the sweep reads a file of a container whose
//     index the node does not read ahead of the others (D269 §2, §4).
//     TestReadThrough_APlayersReadAsksForTheTimelineAloneAndTheSweepReadsTheRest,
//     TestChunks_AFilmWithNoIndexIsCutOnTimeAndItsPlaybackStaysSo and
//     TestReadThrough_TheSweepReadsAFileWithNoIndexReaderFirst (internal/core/api),
//     TestVideoTimeline_KeepsItsKeyframesAndALaterReadAddsItsBytes (internal/store).
//
//   - Each live video is read through once per payload hash for its loudness, when it has sound,
//     and for its picture's timeline, when it has a picture, a cover never; one read asks for both
//     when both are lacking, and for neither when neither is. Its job runs behind anything a person
//     waits on, but at play priority when a player waits on it (ReadForPlay). Enforced by
//     TestReadThrough_APictureIsReadForItsTimelineOnlyWhileItsBytesLackIt,
//     TestReadThrough_OneReadAsksForBothPartsAndFFmpegsWordIsKeptForBoth,
//     TestReadThrough_ReadForPlayRunsAtPlayPriority and
//     TestLoudness_AFileIsMeasuredOnceAndPlaysAtTheLevel (internal/core/api), and end to end by
//     TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled (cmd/orpheum).
//
//   - One FFmpeg cannot read is not asked again, for either part, and one its node would not try
//     is (D226 §2): its job is tried again on its back-off, and once it gives up the next sweep
//     asks again. Enforced by
//     TestReadThrough_AFileFFmpegCannotReadIsNotAskedAgainButOneTheNodeDidNotTryIs,
//     TestReadThrough_OneReadAsksForBothPartsAndFFmpegsWordIsKeptForBoth and
//     TestReadThrough_AReadThroughTheNodeRefusedIsTriedAgainOnItsBackoff (internal/core/api).
//
//   - A read-through is given to its node, not waited for, and recorded as its result arrives, by
//     the file's payload hash, whether or not its job still waits (D234 §7, D238 §2).
//
//   - A read-through for a file under a root or mounted disk that is unavailable, missing at the
//     last scan or its disk work stalled since, parks on that root, and the sweep passes such files
//     over by the library's one rule, library.Unavailability (D231 §5, D233 §10).
//     TestReadThrough_AFileUnderAMissingRootIsNotSweptAndItsReadThroughWaitsForTheRoot and
//     TestReadThrough_AFileUnderARootWhoseDiskStalledIsNotSwept (internal/core/api).
//
//   - A node's word that FFmpeg stalled costs the job no attempt, and two in a row are the file's
//     failure. TestReadThrough_AStallIsTriedAgainAndTwoInARowAreTheFilesFailure (internal/core/api).
//
//   - Only FFmpeg's word on a file, or two stalls in a row, is kept as the file's own failure; a
//     read-through whose job gave up otherwise, refused, unanswered or out of attempts, keeps
//     nothing, so the next sweep asks again (D244 §1, D233 §10).
//     TestRunners_EveryKindsWorkIsDerivedByASweepOrAskedForByAPlayAndItsOwnFailureIsAFact
//     (cmd/orpheum).
//
//   - It is the one writer of loudness and video_timeline. Enforced by
//     TestArch_EachTableHasOneWritingPackage.
//
//   - Imports nothing from internal/node, never os/exec, no database/sql: the depguard rules
//     core-boundary and sql-only-in-store, TestArch_CoreNeverImportsNode and
//     TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - D226: loudness is measured by the node and levelled by the player.
//   - D247 §10 and docs/contributors/design/plans/chunks.md §2, §3: the timeline a film's chunks are
//     cut by, gathered in the same read as the loudness.
//   - D231 §3–§5: derived work, failures as facts, parking.
//   - D234 §7, D238 §2: work given to a node, and results known by what they are.
//
// # Where to change
//
//   - readthrough.go: the sweep for files to read through, what a file's bytes lack, the job, its
//     node's answer and how each part is kept.
package readthrough

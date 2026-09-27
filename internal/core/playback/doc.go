// SPDX-License-Identifier: GPL-3.0-or-later

// Package playback turns a decision into something playable and records what was watched: it
// issues single-playback media tokens, asks jobs for prepare work, and keeps play state (positions
// and watched spans, reported with the device's sequence) against the source file. Continue
// Watching, Next Up, hide and undo read from here.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Play state records against the source file, never a prepared rendition, and a position moves
//     only for a report about a later moment of watching, anchored from the device's monotonic
//     clock, so two devices reporting out of order never rewind it; a report sent again changes
//     nothing (D217 §7, D165). Enforced by
//     TestPlayState_TwoDevicesReportingOutOfOrderNeverRewindThePosition and
//     TestPlayState_ChangesAndSpansAreLoggedAtTheServersTimeOfTheWatching.
//   - A subtitle choice is kept per file, as the file's own, and carried to the work's other files
//     with none of their own, by language and kind, never over a file's own (D219). Enforced by
//     TestSubtitleChoice_IsKeptPerFileAndCarriedToTheSeriesFilesWithNoneOfTheirOwn.
//   - A decision reads how the asking device makes sound: TV speakers and headphones are stereo,
//     a receiver or a device not yet asked is not, and the answer says which so the player can
//     ask (D222). Enforced by TestPlayback_OnStereoOutputASurroundSoundIsAlwaysPreparedSoTheBoostCanAct.
//   - A cancelled prepare or subtitle extraction tells its node to stop, and a cancelled
//     rendition is recorded failed, so the next play prepares it again rather than waiting on it
//     (cut-a §5.3). Enforced by
//     TestJobs_TheViewListsAPrepareAndACancelStopsItOnTheNodeSoTheNextPlayPreparesAgain.
//   - The dialogue boost is the person's, in every file, off until they turn it on (D83, D222).
//     Enforced by TestFilePrefs_TheDialogueBoostIsThePersonsInEveryFile.
//   - A playback starts where the person left the file, unless that is before resume_min_percent
//     or after finished_after_percent of a known length; an unknown length is never read as
//     finished. Enforced by TestPlayState_APositionResumesOnlyBetweenTheThresholdsOfAKnownLength.
//   - A media token grants nothing alone: its bytes are served only to the person and session
//     that started the playback, and only until it has been idle for media_token_idle_expiry_h (D217
//     §2). Enforced by TestPlayback_AMediaAddressPlaysOnlyForItsSessionAndOnlyWhileInUse.
//   - A direct playback's bytes come from the file's node, named by root, path and payload
//     hash; the core never opens the file (D193 §1). Enforced by
//     TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore.
//   - A prepared playback's files are relayed from its node, and one not written yet is 425 with
//     Retry-After while its rendition is being made, never a 404 the player gives up on.
//     Enforced by TestPrepare_APreparedPlaybackIsTooEarlyUntilItsFileExists.
//   - A complete rendition whose node no longer has its playlist is prepared again, and the player
//     waits as for a new one (D217 §5). Enforced by
//     TestPrepare_ARenditionItsNodeNoLongerHasIsPreparedAgain.
//   - Each making of a copy writes into a run directory of its own, so no request reaches an
//     earlier making's files. Enforced by TestPrepare_ACopyMadeWithAnotherSegmentLengthIsMadeAgain.
//   - A copy is made with hls_segment_length_s's segment length, and one made with another length is
//     made again when next played. Enforced by
//     TestPrepare_ACopyMadeWithAnotherSegmentLengthIsMadeAgain.
//   - A prepare job names its source by root, path and payload hash with every audio track, and
//     the rendition is complete only when the node says so; FFmpeg's failure is not retried.
//     Enforced by TestPrepare_AJobCommandsTheNodeAndRecordsTheRenditionComplete and
//     TestPrepare_AFailedRenditionFailsThePlaybackAndIsNotRetried.
//   - Every text subtitle track of a played video, its own and its subtitle files', is extracted,
//     whatever the decision, once per payload hash of the file it is in; a track not yet extracted
//     is 425 with Retry-After, a failed one is prepare_failed and tried again when next played,
//     and a playback reaches only its own file's tracks (D217 §6). Enforced by
//     TestSubtitles_ASubtitleFileBesideTheVideoIsOfferedAndConvertedWhole,
//     TestSubtitles_ATextTrackIsOfferedAndQueuedWhenFirstPlayedAndIsTooEarlyUntilExtracted,
//     TestSubtitles_AnExtractedTrackIsServedAsASSAndWebVTTAndNotExtractedAgain and
//     TestSubtitles_AFailedExtractionSaysSoAndIsTriedAgainWhenNextPlayed.
//
// # Read before changing
//
//   - D217: the menu, the media token, and what M3 decided about playback; §6, subtitles.
//   - D219: a subtitle choice per file, carried across the work; D24: what a choice is.
//   - D157: a position belongs to the File.
//   - D165: the log records state changes and spans; an offline device must not rewind.
//   - D180: a prepared rendition's play state is its source's.
//   - D81: play state is an append-only event log plus a guarded position.
//   - D191: signed-in Watch Together viewing is also recorded.
//   - D83, D222: the dialogue boost is a person's; a device's sound decides whether we fold it down.
//   - docs/contributors/design/plans/play-state.md, docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - tokens.go: media tokens, in memory. playback.go: starting a playback and finding a direct
//     playback's bytes. prepare.go: renditions' rows, their jobs and the nodes' reports. hls.go:
//     a prepared playback's files, and the prepare job's handler. subtitles.go: the text tracks a
//     playback offers, their extraction jobs and their text. waiting.go: jobs waiting for a node's
//     answer. report.go: the play report, its log and the guarded position. resume.go: where a
//     playback starts. hide.go: hiding a work from Continue Watching, which library's home.go
//     lists. Next Up comes after M3.
package playback

// SPDX-License-Identifier: GPL-3.0-or-later

// Package playback turns a decision into something playable and records what was watched: it
// starts a playback with the person's choices, issues single-playback media tokens, wants a
// prepared copy from internal/core/renditions when the decision needs one, and keeps play state
// (positions and watched spans, reported with the device's sequence) against the source file.
// Continue Watching, Next Up, hide and undo read from here.
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
//   - Play state records against the source file, never a prepared rendition, and a position moves
//     only for a report about a later moment of watching, anchored from the device's monotonic
//     clock, so two devices reporting out of order never rewind it; a report sent again changes
//     nothing (D217 §7, D165). Enforced by
//     TestPlayState_TwoDevicesReportingOutOfOrderNeverRewindThePosition and
//     TestPlayState_ChangesAndSpansAreLoggedAtTheServersTimeOfTheWatching.
//
//   - A playback's first stall starts the lower quality its link carries where its player was, and
//     the answer to the report of a later stall offers it, once; a Watch Together viewer reports by
//     their membership and nothing of their watching is kept (D247 §2, D243 §2). Enforced by
//     TestReport_ASecondStallIsAnsweredWithTheLowerQualityTheFirstStarted and
//     TestReport_AViewersReportKeepsNothingAndTheirStallsAreAnswered (internal/core/api).
//
//   - A playback starts with the subtitle and audio tracks internal/core/preferences works out
//     from the file's tracks as they are now, and a play report keeps what it started with as what
//     the started file used (D230). Enforced by
//     TestSubtitleChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes and
//     TestAudioChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes (internal/core/api).
//     A chosen audio track that is not the one the original starts with is played prepared, since a
//     browser plays the original's first (decide's audio_track). A choice or a subtitle offset is
//     kept only for a file, and among tracks, the transaction that keeps it reads, so it never
//     decides on what that transaction does not see. Enforced by
//     TestKeepSubtitleOffset_RefusesAFileItsTransactionRemoved.
//
//   - A decision reads how the asking device makes sound: built-in speakers and headphones are stereo,
//     a receiver or a device not yet asked is not, and the answer says which so the player can
//     ask (D222). Enforced by TestPlayback_OnStereoOutputASurroundSoundIsAlwaysPreparedSoTheBoostCanAct.
//
//   - The dialogue boost is the person's, in every file, off until they turn it on (D83, D222).
//     Enforced by TestFilePreferences_TheDialogueBoostIsThePersonsInEveryFile.
//
//   - A playback starts where the person left the file, unless that is before resume_min_percent
//     or after finished_after_percent of a known length; an unknown length is never read as
//     finished. Enforced by TestPlayState_APositionResumesOnlyBetweenTheThresholdsOfAKnownLength.
//
//   - A media token grants nothing alone: its bytes are served only to the person and session
//     that started the playback, and only until it has been idle for media_token_idle_expiry_hours (D217
//     §2). Enforced by TestPlayback_AMediaAddressPlaysOnlyForItsSessionAndOnlyWhileInUse.
//
//   - A direct playback's bytes come from the file's node, named by root, path and payload
//     hash; the core never opens the file (D193 §1). Enforced by
//     TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore.
//
//   - A prepared playback's files are relayed from its node, and one not written yet is 425 with
//     Retry-After while its rendition is being made, never a 404 the player gives up on.
//     Enforced by TestPrepare_APreparedPlaybackIsTooEarlyUntilItsFileExists.
//
//   - A play of a file that cannot be read now, its node away or its root or a disk inside it
//     missing, is refused before anything is queued (library.RefuseUnreadable, D241 §4).
//     TestScenario_S29_APlayWhoseNodeIsAwayIsRefusedAtOnceAndQueuesNothing,
//     TestPlayback_APlayOfAFileUnderAMissingRootIsRefusedAtOnceAndQueuesNothing (internal/core/api).
//
//   - Levelling is the person's, in every file, on until they turn it off (D226 §5). Enforced by
//     TestLoudness_LevellingIsOnUntilThePersonTurnsItOff.
//
//   - A media token is held by the person and session that started it while they reach its file's
//     library: one taken back stops the playback at its next byte or report request (D243 §4).
//     TestReach_ALibraryTakenBackIsGoneAtTheNextRequestEverywhere.
//
//   - A playback's chunks are served only to the holder of its token, as its other bytes are, and
//     only while its holder reaches the film's library. Enforced by
//     TestChunks_AnotherPersonsTokenIsRefused and
//     TestReach_ALibraryTakenBackIsGoneAtTheNextRequestEverywhere (internal/core/api).
//
// # Read before changing
//
//   - D217: the menu, the media token, and what M3 decided about playback.
//   - D230: the last language chosen in a work follows its unwatched files; D24: what a choice is.
//   - D157: a position belongs to the File.
//   - D165: the log records state changes and spans; an offline device must not rewind.
//   - D180: a prepared rendition's play state is its source's.
//   - D81: play state is an append-only event log plus a guarded position.
//   - D191: signed-in Watch Together viewing is also recorded.
//   - D83, D222: the dialogue boost is a person's; a device's sound decides whether we fold it down.
//   - D226: loudness is measured by the node (internal/core/loudness) and levelled by the player.
//   - docs/contributors/design/plans/play-state.md, docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - tokens.go: media tokens, in memory. playback.go: starting a playback, finding a direct
//     playback's bytes, and a playback's subtitles through internal/core/subtitles. hls.go: a
//     prepared playback's files, through internal/core/renditions. chunks.go: a playback as its
//     film's chunks are served to it, through internal/core/chunks (D247 §10). report.go: the play report, its
//     log and the guarded position. resume.go: where a playback starts. hide.go: hiding a work from
//     Continue Watching, which library's home.go lists. choices.go: a person's choices in a file,
//     kept through internal/core/preferences, a file's audio tracks, and the subtitle tracks
//     internal/core/subtitles offers. Next Up comes after M3.
package playback

// SPDX-License-Identifier: AGPL-3.0-or-later

// Package playback turns a decision into something playable and records what was watched: it
// starts a playback with the person's choices, issues single-playback media tokens, starts a film
// its device cannot play as it is on its ladder of chunks (internal/core/chunks), and keeps play state
// (positions and watched spans, reported with the device's sequence) against the source file.
// Continue Watching, Next Up, hide and undo read from here.
//
// # Invariants
//
//   - A link speed a player measured is kept by player, a device or a deviceless session, and by
//     network, an IPv6 /64 or an IPv4 address, for a day (D257), with the spread of the samples it
//     rested on, in the store (link_memory), so a restart keeps it, by
//     TestLinkSpeeds_OutliveTheCoreThatHeardThem; a start's offer hands it to the player as a prior
//     counting as two of its own samples while under an hour old, fading to none at
//     link_speed_fresh_hours (bandwidth-sampling-2026-10-29.md P2), by
//     TestRememberedLink_CountsAsTwoSamplesFadingWithItsAge; and beside it the
//     steadiness level its chunked player last reported, which every ladder offer hands back
//     relaxed by one for each `link_steadiness_relaxes_seconds` since, an hour to start (D352 Q2), by
//     TestLinkSteadiness_IsKeptBesideTheSpeedAndRelaxesAnHourAtATime,
//     TestLinkSteadiness_RelaxesAsOftenAsItsSettingSays and
//     TestPlayback_ADeviceStartsAtTheSteadinessItsLastPlaybackOnTheNetworkReported
//     (internal/core/api). A film cut into chunks starts on
//     its ladder, at the top, or where its player's speed on its network calls for, and no prepared
//     copy is made for it; its stalls start nothing (D260). A film not cut yet within
//     play_ladder_wait_ms is cut on time where its device cannot play it as it is, keeping that grid,
//     and plays as it is where it can (D344 §2), by
//     TestStartOnLadder_WaitsForAFilmsKeyframesUpToTheLadderWaitThenStartsOnTime.
//     internal/core/clientnet's TestOf_IsAnIPv6AddressesSlash64OrAnIPv4AddressWhole,
//     TestLinkSpeeds_AreFreshForADayOnTheirOwnNetwork,
//     TestPlayback_ADeviceRememberedSlowStartsOnALowerQualityOnTheSameNetwork,
//     TestPlayback_AFilmItsDeviceCannotPlayStartsOnItsLadderAtItsMasterPlaylist and
//     TestReport_AStallOnTheLadderIsOfferedNothing (internal/core/api).
//
//   - Each playback holds what its player wants of its film's chunks, which decides the runs that go
//     on (D309): one picture and its sound, each from its own position, moved by its own stream's
//     chunks its player asks for, and both by its player's word, a picture asked for where a seek took
//     it, paused or not, and its start on its ladder (D355), with its stall's offer beside it; none
//     moves the position its player reports, which it resumes from; and where its player stands: playing, lapsing
//     after chunk_interest_lapses_after_seconds of silence; paused or gone from its page, wanting
//     only what it keeps ahead; gone to another film, said so or by starting one, wanting nothing.
//     TestInterestsIn_AreWhatEachPlayerLastAskedFor,
//     TestInterestsIn_EachStreamIsWhereItsOwnAsksPutItAndASeekMovesBoth,
//     TestSayInterest_ReplacesWhatThePlayerWantedAndLeavingSaysWhy,
//     TestReplaceOthers_APlayerStartingAnotherFilmLeavesTheOthersForGood; through the API,
//     TestInterest_ARunOfAQualityThePlayerLeftStops and
//     TestInterest_APausedPlayersSoughtAskBehindWhereItPausedStartsARunThere (internal/core/api).
//
//   - Each chunk store a player asked for is kept at the highest tier any playback gives it, by
//     where its player stands and how long since it was heard (D309 §8), and every answer to a node's
//     poll says them. TestStoreTiers_AreTheHighestAnyPlayerGivesEachStore and
//     TestCommands_EachAnswerCarriesHowStronglyEachChunkStoreIsKept (internal/core/nodes).
//
//   - What a playback needs of the server's upload is the average rates of the picture and sound
//     its player asked for within askedWithin, or, for one that asks for no chunks, what it was
//     sent; every byte of a film the server sends is counted to its playback, and the need, the
//     bytes sent and the films' share of server_upload_mbps are logged while anyone plays (D286 §5).
//     Enforced by TestUpload_IsEachPlaybacksRatesOrWhatItWasSent,
//     TestWatchUpload_LogsTheNeedBesideTheFilmsShareWhileAnyonePlays and
//     TestUpload_APlaybacksNeedAndTheBytesSentItAreCounted (internal/core/api).
//
//   - While the need is over the films' share of server_upload_mbps, the playback needing most is
//     lowered a quality at a time, the newest of those needing the same, onto a quality another
//     playback of its film loads where the next lower is made for no one; a lowered playback is
//     raised a quality at a time once the need has had room for 30 s; every chunk's answer tells the
//     player its ceiling, and an unanswered setting lifts every one (D286 §4, §5). Enforced by
//     TestUploadControl_LowersTheOneNeedingMostOntoAQualityAlreadyMade,
//     TestUploadControl_LowersTheNewestOfThoseNeedingTheSame,
//     TestUploadControl_RaisesOneQualityAtATimeOnceThereIsRoom,
//     TestUploadControl_SaysWhenEveryoneIsAtTheirLowestAndItIsStillOver and
//     TestUpload_APlaybackOverTheShareIsToldItsCeilingWithEachChunk (internal/core/api).
//
//   - The films' share is lowered by a fifth at most every 10 s, to a tenth of the setting's at
//     least, while two players at once report links too slow for what they need and the films need no
//     more than the share, and grown back a tenth every 30 s a minute after; a Watch Together group's
//     players each start within an even part of what other films leave (D286 §5). Enforced by
//     TestCrowding_LowersTheShareWhileCrowdedAndGrowsItBackOnceItStops,
//     TestIsCrowded_IsTwoSlowPlayersWhileTheFilmsAreWithinTheShare and
//     TestStartAtMostKbps_IsAnEvenPartForAGroupsPlayers.
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
//   - Play state records against the source file, never a converted copy of it, and a position moves
//     only for a report about a later moment of watching, anchored from the device's monotonic
//     clock, so two devices reporting out of order never rewind it; a report sent again changes
//     nothing; and only for a playback that has played, which a report says by its started event
//     or a span, never one whose player, not yet loaded, reads 0 (D217 §7, D165). Enforced by
//     TestPlayState_TwoDevicesReportingOutOfOrderNeverRewindThePosition,
//     TestPlayState_APlaybackThatHasNotPlayedMovesNoPosition and
//     TestPlayState_ChangesAndSpansAreLoggedAtTheServersTimeOfTheWatching.
//
//   - A playback not on its ladder: its first stall starts the lower quality its link carries where its player was, and
//     the answer to that report and to every report after it offers it, saying whether it is ready;
//     a Watch Together viewer reports by their membership and nothing of their watching is kept
//     (D253 §1, D243 §2). Enforced by
//     TestReport_AFirstStallIsAnsweredWithTheLowerQualityItStartedAndSoIsEveryReportAfter,
//     TestIsReadyAt_AnOfferIsReadyOnceItsPictureAndSoundAreMadeTenSecondsOnFromThePlayer
//     (internal/core/chunks) and
//     TestReport_AViewersReportKeepsNothingAndTheirStallsAreAnswered (internal/core/api).
//
//   - A playback starts with the subtitle and audio tracks internal/core/preferences works out
//     from the file's tracks as they are now, and a play report keeps what it started with as what
//     the started file used (D230). Enforced by
//     TestSubtitleChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes and
//     TestAudioChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes (internal/core/api).
//     A chosen audio track that is not the one the original starts with is played converted, since a
//     browser plays the original's first (decide's audio_track). A choice or a subtitle offset is
//     kept only for a file, and among tracks, the transaction that keeps it reads, so it never
//     decides on what that transaction does not see. Enforced by
//     TestKeepSubtitleOffset_RefusesAFileItsTransactionRemoved.
//
//   - A decision never reads how the asking device makes sound, and no one is asked: a film's sound
//     plays as it is where the browser plays it, and the person's dialogue boost, on, over a sound of
//     more than two channels converts it with its boosted mix (D336 §1, §2, §4). Enforced by
//     TestPlayback_ASurroundSoundPlaysAsItIsUntilThePersonTurnsTheBoostOn and
//     TestDevices_NothingTheAPIDocumentsAsksHowADeviceMakesSound.
//
//   - The dialogue boost is the person's, in every file, off until they turn it on (D83, D222).
//     Enforced by TestFilePreferences_TheDialogueBoostIsThePersonsInEveryFile.
//
//   - A playback starts where the person left the file, unless that is before in_progress_after_percent
//     or after finished_after_percent of a known length, or was watched before they last marked its
//     title by hand (D370 §4); an unknown length is never read as finished. Enforced by
//     TestPlayState_APositionResumesOnlyBetweenTheThresholdsOfAKnownLength and
//     TestWatched_AMarkByHandIsAFreshStartAndUndoPutsItBack (internal/core/api).
//
//   - A title is watched per person (D370 §1): a report that plays one of its files past
//     finished_after_percent of a known length, or to its end, makes every film, episode and other
//     video the file is filed as watched, never an extra, and never a report of unattended watching or
//     a far position in a file of unknown length; playing it again keeps it watched. A mark by hand
//     marks titles watched, or unwatched, and answers with each one's mark before, which
//     RestoreWatched puts back: its Undo (§4). Only this package writes title_watched
//     (TestArch_EachTableHasOneWritingPackage). Enforced by
//     TestWatched_APlayToItsFinishMarksItWatchedAndARestartKeepsIt and
//     TestWatched_AMarkByHandIsAFreshStartAndUndoPutsItBack (internal/core/api).
//
//   - A media token grants nothing alone: its bytes are served only to the person and session
//     that started the playback, and only until it has been idle for media_token_idle_expiry_hours (D217
//     §2). Enforced by TestPlayback_AMediaAddressPlaysOnlyForItsSessionAndOnlyWhileInUse.
//
//   - A direct playback's bytes come from the file's node, named by root, path and payload
//     hash; the core never opens the file (D193 §1). Enforced by
//     TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore.
//
//   - A play of a file that cannot be read now, its node away or its root or a disk inside it
//     missing, is refused before anything is queued (library.RefuseUnreadable, D241 §4).
//     TestScenario_S29_APlayWhoseNodeIsAwayIsRefusedAtOnceAndQueuesNothing,
//     TestPlayback_APlayOfAFileUnderAMissingRootIsRefusedAtOnceAndQueuesNothing (internal/core/api).
//
//   - Levelling is the person's, in every file, on until they turn it off (D226 §5). Enforced by
//     TestLoudness_LevellingIsOnUntilThePersonTurnsItOff.
//
//   - A playback's levelling is its starting track's as it is, never the main track's, and each sound
//     its tracks name says what levels it, its own track's levelling and the gain its chunks carry,
//     so a track switched to in place, on the ladder or by the element off it, plays at its own level
//     (audio-switch-and-retiming-2026-11-01.md §1.2). Enforced by
//     TestStartingLevelling_IsTheStartingTracksOwnNotTheMainTracks,
//     TestTracks_OffTheLadderEachTrackIsLevelledByItsOwn and
//     TestOffered_EachSoundIsLevelledByItsOwnTrack (internal/core/chunks).
//
//   - Each audio track's levelling as it is, which a player whose film was measured as it played takes
//     up from its file's waits, is what a playback started then would level it by (TrackLevellings).
//     Enforced by TestFileWaits_SayTheLevellingOnceTheFileIsMeasured (internal/core/api).
//
//   - The sound a playback plays is the one its player last asked for or said, which a stall's offer
//     starts in place of its starting track's; a player playing its file as it is that says the sound
//     it switched to wants none of the film's chunks for it. Enforced by
//     TestSayInterest_APlayerOffTheLadderSaysTheSoundAStallStartsAndWantsNoChunks and
//     TestStalled_StartsTheSoundThePlayerPlaysNotTheOneItStartedWith (internal/core/chunks).
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
//   - What was watched of a removed file outlives it: before library's purge deletes the file, how
//     often each person finished it and how long they watched are kept on its version, and a
//     person's totals are the same after (D306 §8). The position does not carry.
//     TestPurgeRemovedFiles_KeepsWhatWasWatchedOnTheVersion.
//
// # Read before changing
//
//   - D217: the menu, the media token, and what M3 decided about playback.
//   - D370: whether a person watched a title, kept per person, by playing and by hand.
//   - D230: the last language chosen in a work follows its unwatched files; D24: what a choice is.
//   - D157: a position belongs to the File. D355: a playback's picture and sound are wanted each from
//     its own position, apart from the one it reports and resumes from.
//   - D165: the log records state changes and spans; an offline device must not rewind.
//   - D180: a prepared rendition's play state is its source's.
//   - D81: play state is an append-only event log plus a guarded position.
//   - D191: signed-in Watch Together viewing is also recorded.
//   - D83, D222: the dialogue boost is a person's. D336: it alone of how a person hears a film needs
//     the sound made; how a device makes sound is asked of no one.
//   - D226: loudness is measured by the node (internal/core/loudness) and levelled by the player.
//   - docs/contributors/design/plans/play-state.md, docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - tokens.go: media tokens, in memory. speeds.go: each player's link on each network, kept in
//     the store. playback.go: starting a playback, finding a direct
//     playback's bytes, and a playback's subtitles through internal/core/subtitles. chunks.go: a playback as its
//     film's chunks are served to it, through internal/core/chunks (D247 §10). report.go: the play report, its
//     log and the guarded position. resume.go: where a playback starts. watched.go: a title watched
//     by playing to its finish, and marked by hand with its undo (D370). hide.go: hiding a work from
//     Continue Watching, which library's home.go lists. choices.go: a person's choices in a file,
//     kept through internal/core/preferences, a file's audio tracks, and the subtitle tracks
//     internal/core/subtitles offers. Next Up comes after M3.
package playback

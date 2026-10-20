// SPDX-License-Identifier: AGPL-3.0-or-later

// Package playback turns a decision into something playable and records what was watched: it
// starts a playback with the person's choices, issues single-playback media tokens, wants a
// prepared copy from internal/core/renditions when the decision needs one, and keeps play state
// (positions and watched spans, reported with the device's sequence) against the source file.
// Continue Watching, Next Up, hide and undo read from here.
//
// # Invariants
//
//   - A link speed a player measured is kept by player, a device or a deviceless session, and by
//     network, an IPv6 /64 or an IPv4 address, for a day (D257). A film cut into chunks starts on
//     its ladder, at the top, or where its player's speed on its network calls for, and no prepared
//     copy is made for it; its stalls start nothing (D260). A film not cut yet plays as its decision
//     says. internal/core/clientnet's TestOf_IsAnIPv6AddressesSlash64OrAnIPv4AddressWhole,
//     TestLinkSpeeds_AreFreshForADayOnTheirOwnNetwork,
//     TestPlayback_ADeviceRememberedSlowStartsOnALowerQualityOnTheSameNetwork,
//     TestPlayback_ACutFilmStartsOnItsLadderAndPreparesNoCopy and
//     TestReport_AStallOnTheLadderIsOfferedNothing (internal/core/api).
//
//   - Each playback holds what its player wants of its film's chunks, which decides the runs that go
//     on (D309): one picture and its sound from a position, read off each chunk it asks for, said by
//     its player, set as it starts on its ladder, with its stall's offer beside it; and where its
//     player stands: playing, lapsing after chunk_interest_lapses_after_seconds of silence; paused or
//     gone from its page, wanting only what it keeps ahead; gone to another film, said so or by
//     starting one, wanting nothing. TestInterestsIn_AreWhatEachPlayerLastAskedFor,
//     TestSayInterest_ReplacesWhatThePlayerWantedAndLeavingSaysWhy,
//     TestReplaceOthers_APlayerStartingAnotherFilmLeavesTheOthersForGood; through the API,
//     TestInterest_ARunOfAQualityThePlayerLeftStops (internal/core/api).
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
//   - Play state records against the source file, never a prepared rendition, and a position moves
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
//   - What was watched of a removed file outlives it: before library's purge deletes the file, how
//     often each person finished it and how long they watched are kept on its version, and a
//     person's totals are the same after (D306 §8). The position does not carry.
//     TestPurgeRemovedFiles_KeepsWhatWasWatchedOnTheVersion.
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

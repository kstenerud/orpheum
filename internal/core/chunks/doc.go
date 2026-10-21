// SPDX-License-Identifier: AGPL-3.0-or-later

// Package chunks cuts a film into chunks made when a player first asks for one, so a slow link
// gets lower qualities, among which the player's own bitrate controller chooses (D247 §10, D265,
// chunks.md). It holds the grid, a film's chunk boundaries: on its main picture's keyframes, from
// the file's own index or a read-through kept in persistent.db (internal/core/readthrough), each
// chunk starting at the first keyframe at or after the next multiple of
// hls_segment_length_seconds, so a chunk may be shorter or longer than that (D259 §1); or, for a
// file with no index not yet read through, on time, at each multiple (D269 §1). It holds too the
// streams a film is cut into, its picture copied, a quality of chunk_qualities or a sound, each a
// store on the film's node; the playlists a playback is offered; the make_chunks job, a run of one
// store's chunks given to the node; and, in memory, each store's chunks known made and its runs on
// their way. A film plays from its chunks from the start (D260).
//
// # Invariants
//
//   - A playback starts on its film's ladder at the top, the film's own picture where its player
//     decodes it, its twin, H.264 at its own height, where it does not (D323 §1), or, with its link
//     remembered, at the highest picture within link_share_for_quality_percent of it, the lowest when none is
//     (D260 §2). Enforced by TestStartOnLadder_StartsAtTheFilmsOwnPictureWithNoLinkRemembered,
//     TestStartOnLadder_StartsAtTheHighestPictureTheRememberedLinkCarries and
//     TestStartOnLadder_APlayerThatCannotDecodeThePictureStartsAtItsTwin.
//
//   - The qualities below the top step down from the twin's rate where it plays, not the original's,
//     and are chunk_qualities_hardware's where the film's node encodes on a GPU (D323 §1, §3).
//     Enforced by TestOffered_BelowTheTwinTheQualitiesStepDownFromItsRate and
//     TestOffered_TheLadderFollowsTheFilmsNodesEncoder.
//
//   - A film whose loudness is not measured offers its sounds unlevelled, each a store of its own
//     ("-raw"), so a levelled chunk never follows an unlevelled one; a playback goes on with the
//     sounds its playlist named once the loudness is measured (D259 §2).
//     TestChunks_ASoundOfferedBeforeItsLoudnessIsUnlevelledAndAPlaybackKeepsIt (internal/core/api).
//
//   - A grid starts at the picture's first keyframe, and each next start is the first keyframe at or
//     after the next multiple of the segment length, so a stretch cut alone agrees with the whole
//     film's (D259 §1); a last chunk shorter than that is merged into the one before, and a picture
//     with no keyframe before its end cannot be cut (ErrCannotBeCut).
//     Enforced by TestGridOf_StartsAChunkAtTheFirstKeyframeAtOrAfterEachMultipleOfTheLength,
//     TestGridOf_AStretchCutAloneAgreesWithTheWholeFilmsGrid and
//     TestGridOf_APictureWithNoKeyframeBeforeItsEndCannotBeCut.
//
//   - A film's grid is its main picture's timeline, a cover's never, by its bytes' payload hash;
//     while there is none it says so without an error, and a film FFmpeg could not read through,
//     or with no picture, cannot be cut. Enforced by
//     TestGrid_IsTheMainPicturesKeptTimelineMergedToTheSegmentLength and
//     TestGrid_SaysThereIsNoneYetOrThatTheFilmCannotBeCut.
//
//   - A stream's name reads back as the stream, a store is <file_id>/chunks-<stream>/<grid_id>, and
//     a grid's id is the hash of its starts and the film's length, so a changed grid is a new store.
//     Enforced by TestStreamNamed_ReadsEachStreamsNameBackAndRefusesOthers,
//     TestGridID_IsTheHashOfItsStartsAndLengthAndNamesTheStore and
//     TestStoreFileNamed_IsTheInitOrAChunkByItsNumber.
//
//   - A film whose file has no index of its keyframes is cut on time, a chunk at each multiple of the
//     segment length, and its playback offered the qualities, at least one, and not the picture
//     copied; it is read through ahead of the sweep, and a playback that began on time keeps that
//     grid to its end, a run on it made on it (D269). Enforced by
//     TestTimeGridOf_StartsAChunkAtEachMultipleOfTheLengthAndMergesAShortLastOne,
//     TestMasterPlaylist_AFilmWhoseFileHasNoIndexIsCutOnTimeWithoutItsPictureCopied,
//     TestMasterPlaylist_APlaybackThatBeganOnTimeStaysOnTimeOnceTheKeyframesAreKnown and
//     TestHandleMakeChunks_ARunOnTimeIsMadeOnTheTimeGrid; through the API by
//     TestChunks_AFilmWithNoIndexIsCutOnTimeAndItsPlaybackStaysSo (internal/core/api).
//
//   - A playback is offered the picture copied unless its player cannot decode it, each quality
//     below the film's own picture in bit rate and height, and every sound a prepare makes, its
//     starting track's standard mix the default; any other stream or file is ErrNoStream, before the
//     node is asked for anything. A film with no grid yet is read through at play priority.
//     Enforced by TestMasterPlaylist_OffersThePictureAndEachQualityBelowTheFilmsOwnWithEverySound,
//     TestMasterPlaylist_AFilmWithNoGridIsReadThroughForOne,
//     TestStoreFileOf_OffersOnlyTheStreamsAndFilesThePlaybackHas,
//     TestMasterPlaylist_ListsEachSoundThenEachPictureHighestFirst and
//     TestMediaPlaylist_ListsEveryChunkAtItsExactLengthAfterTheInit; through the API by
//     TestChunks_AFilmWithNoGridIsTooEarlyAndReadThroughThenListsWhatThePlaybackIsOffered
//     (internal/core/api).
//
//   - A stall is offered the highest quality within 70% of its link, the lowest when the link is not
//     measured or too slow for any (said so), or the one below whose chunks another stall is already
//     making there; the quality's run and the starting sound's are started from the stall's chunk
//     (D247 §2, §6–§8). Enforced by
//     TestStalled_OffersTheHighestQualityTheLinkCarriesAndStartsItWhereThePlayerIs,
//     TestStalled_OffersTheLowestWhenTheLinkIsUnknownOrTooSlowForAny,
//     TestStalled_TakesTheQualityBelowThatAnotherStallIsAlreadyMaking and
//     TestStalled_AFilmWithNoLowerQualitySaysSo.
//
//   - A chunk asked for queues at most one run, at play priority on the film's node, from the first
//     chunk in the minute of film from it that is neither made nor on its way, through at most five
//     minutes of film and never over a chunk made or a run's. Its job targets the store's chunk_store
//     row, so the runs of a film's streams queue side by side, and a run asked for while another of
//     its store waits queued is folded into that one and asked for again at the next ask. Enforced by
//     TestEnsure_QueuesOneRunFromTheChunkThroughFiveMinutesOfFilmAtPlayPriority,
//     TestEnsure_NeverQueuesARunOverChunksARunWillMakeOrThatAreMade,
//     TestEnsure_TwoStreamsOfOneFilmWarmedTogetherEachQueueARun,
//     TestEnsure_ARunTheQueueDidNotTakeIsAskedForAgainAtTheNextAsk and
//     TestEnsure_AFilmWithNoGridYetIsSaidSo; through the API by
//     TestChunks_AChunkTheNodeHasNotMadeIsAskedForAndRelayedOnceItIs (internal/core/api).
//
//   - A chunk the node served, whose next is known made or on its way, looks the same minute ahead,
//     so a run made faster than played has the next on its way before the player reaches it; after a
//     restart nothing past a served chunk is known, and nothing is queued over it. Enforced by
//     TestServed_AChunkOfARunMadeAheadStartsTheNextRunAMinuteBeforeThePlayerReachesIt.
//
//   - A run goes on while a player wants it, and stops at once when none does (D309 §2): each
//     player wants one picture and its sound from a position (InterestsBy, which
//     internal/core/playback keeps from its word, its requests and its stall's offer), and a run is
//     wanted by one playing from a chunk it has not passed, starting within the minute of film after
//     the first chunk not made from there, or by one stopped, paused or gone from its page, whose
//     chunk_kept_ahead_seconds ahead it is still making (§7). A player's ask stops the film's
//     unwanted runs before its own is planned (Want), as does
//     each step of a run, for an interest that lapsed (§3); a run the server starts for a player, as
//     it starts or stalls, stops nothing. A queued run that begins just past an ask's chunk, with
//     nothing between, is cancelled for one run from the ask's. Enforced by
//     TestStopUnwanted_APlayerThatSeeksAwayStopsTheRunItLeft,
//     TestStopUnwanted_ARunItsPlayerWillReachWithinAMinuteGoesOn,
//     TestStopUnwanted_APlayerOnMadeChunksWantsTheRunThatMakesWhatFollowsThem,
//     TestStopUnwanted_ARunOfAQualityItsPlayerLeftStops,
//     TestStopUnwanted_APausedPlayersRunStopsOnceWhatItKeepsAheadIsMade and
//     TestEnsure_AJumpJustBeforeAQueuedRunStartsOneRunWhereItLanded; through the API by
//     TestInterest_ARunOfAQualityThePlayerLeftStops (internal/core/api).
//
//   - Every run on a node stays faster than playback (D250 §6): when one of two or more measures
//     below chunk_run_slowest_speed_percent of it over chunk_run_speed_window_seconds, one is cancelled: the oldest no
//     player wants, its store refusing no one, so a player that comes to its chunks later starts a
//     run of its own; or else the newest, its players moved down a quality (MovePlayersBy, D309 §6)
//     and its store not evicted, by
//     TestKeepRunsFast_MovesARunsPlayersOntoAQualityAlreadyMadeRatherThanEvictIt; or, when one is at
//     the lowest, evicted, its store refusing its players with ErrNoRoomToMake until a stall asks for
//     it again: the chunks it was to make, not those made or another of its runs is making, and it
//     starts no run, by TestEnsure_AnEvictedStoreGivesWhatIsMadeOrOnItsWayAndStartsNoRun. The node is
//     then left chunk_run_speed_window_seconds to settle. Enforced by
//     TestKeepRunsFast_EvictsTheNewestWhenEveryRunIsInUse,
//     TestKeepRunsFast_CancelsTheOldestRunNoOneIsPlayingFromRefusingNoOne,
//     TestKeepRunsFast_EvictsARunASeekLeftThoughItsQualityIsStillPlayed and
//     TestKeepRunsFast_EvictsNothingWhileEveryRunKeepsUpOrOneRunsAlone. Before any is evicted, a run
//     players want of a quality, encoded in software, that is behind holds its node back for a speed
//     window, its other work stopped (HoldBack, D324 §5), and the node settles. Enforced by
//     TestKeepRunsFast_APlayedEncodeBehindPlaybackHoldsItsNodeBackBeforeAnyRunIsEvicted and
//     TestKeepRunsFast_ARunNoOneWantsHoldsNothingBack; through the API, the refusal
//     is a 429 by TestChunks_AnEvictedQualityIsRefusedWithItsOwnStatusAndCode (internal/core/api).
//
//   - A run is sent as its film's main picture by identity, its stream's recipe, a sound with the
//     track's mix and levelling gain as a prepare's and led in from the chunk before, and the grid's
//     bounds; a run on a grid the film no longer has ends for good, and its node's word ends it as a
//     prepare's does. Enforced by
//     TestHandleMakeChunks_SendsTheRunOfTheFilmsPictureAndTakesItsChunksAsTheyAreMade,
//     TestHandleMakeChunks_ASoundRunStartsAChunkEarlyWithItsTracksGainAndMix,
//     TestHandleMakeChunks_ARunOnAGridTheFilmNoLongerHasEndsForGood and
//     TestHandleMakeChunks_EachEndIsTakenAsAPreparesIs.
//
//   - It reads the timelines and never writes them: internal/core/readthrough writes
//     video_timeline. Enforced by TestArch_EachTableHasOneWritingPackage.
//
//   - Imports nothing from internal/node, never os/exec, no database/sql: the depguard rules
//     core-boundary and sql-only-in-store, TestArch_CoreNeverImportsNode and
//     TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - D247 §6 and §10: the qualities, and chunks cut at the source's keyframes, made on request.
//   - docs/contributors/design/plans/chunks.md §1 (the words) and §3 (the core).
//   - docs/contributors/design/research/slow-links.md §10: the measured recipe.
//   - D234 §7: work given to a node, resolved by its word.
//
// # Where to change
//
//   - grid.go: GridOf, the merging of keyframes, and Service.Grid, a film's grid from its timeline.
//   - streams.go: the streams' names, the grid's id and the store's directory.
//   - offer.go and playlists.go: what a playback is offered, and its playlists.
//   - files.go: a store's file a player asks for. ensure.go and runs.go: the runs a player's asks
//     queue, and the chunks known made. job.go: the make_chunks job.
//   - qualities/: the setting chunk_qualities, read and checked by one function.
package chunks

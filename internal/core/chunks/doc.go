// SPDX-License-Identifier: GPL-3.0-or-later

// Package chunks cuts a film into chunks made when a player first asks for one, so a slow link
// gets lower qualities hls.js switches between by itself (D247 §10, chunks.md). It holds the grid, a
// film's chunk boundaries, its main picture's keyframes, which a read-through keeps in
// persistent.db (internal/core/readthrough), merged so each chunk is at least
// hls_segment_length_seconds long; the streams a film is cut into, its picture copied, a quality of
// chunk_qualities or a sound, each a store on the film's node; the playlists a playback is offered;
// the make_chunks job, a run of one store's chunks given to the node; and, in memory, each store's
// chunks known made and its runs on their way. The player's move to the chunks comes after
// (chunks.md §5).
//
// # Invariants
//
//   - A grid starts at the picture's first keyframe, and each next start is the first keyframe at
//     least the segment length past the last; a last chunk shorter than that is merged into the one
//     before, and a picture with no keyframe before its end cannot be cut (ErrCannotBeCut).
//     Enforced by TestGridOf_MergesKeyframesGreedilyIntoChunksOfAtLeastTheLength and
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
//   - A seek starts a run where it lands; catching up never does (D250 §5). A chunk asked for more
//     than chunk_new_run_beyond_seconds of film past the chunk a run of its store makes next cancels
//     that run, queued or running, and its run is queued; one nearer waits for the run. Enforced by
//     TestEnsure_ASeekFarPastARunCancelsItAndQueuesTheSeeksRun and
//     TestEnsure_NeverQueuesARunOverChunksARunWillMakeOrThatAreMade.
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

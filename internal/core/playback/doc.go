// SPDX-License-Identifier: GPL-3.0-or-later

// Package playback turns a decision into something playable and records what was watched: it
// issues single-playback media tokens, asks jobs for prepare work, and keeps play state (positions
// and watched spans, reported with the device's sequence) against the source file. Continue
// Watching, Next Up, hide and undo read from here.
//
// # Invariants
//
//   - A prepare, an extraction, a conversion or a measurement is given to its node, not waited for:
//     its handler records what it begins and answers with the job assigned (nodes.Exchanges.Assign),
//     and its kind's Assignee resolves it from the node's word, a silence or a restart with the
//     answers the handler gave before; a prepare cancelled while its node holds it has its making
//     recorded failed, so the next play prepares again (D234 §7).
//     TestJobs_TheViewListsAPrepareAndACancelStopsItOnTheNodeSoTheNextPlayPreparesAgain
//     (internal/core/api).
//
//   - A player asking for a rendition still being prepared keeps its prepare wanted (D234 §3), so
//     it falls to background priority only ten minutes after the player stops asking (S87).
//     TestPrepare_APlayerAskingForTheRenditionKeepsItsPrepareWanted (internal/core/api).
//
//   - A prepare that waits, for its node or for room on it, has not begun its making: its rendition
//     is recorded running only once the node is given it.
//     TestHandle_APrepareThatWaitsHasNotBegunItsMaking.
//
//   - A node's result on a prepare, an extraction or a measurement is recorded as it arrives, by
//     what its echoed work names, whether or not its job still waits (D238 §2, S81): a making of a
//     rendition by its run, so a rendition made again since is never completed by an earlier one
//     and nothing heard after an end undoes it (G13); a text by its source's hash; a measurement
//     by the file's. The job only learns how the work ended.
//     TestRecordPrepare_RecordsAMakingByItsRunWhetherOrNotItsJobWaits.
//
//   - The index of renditions is squared with those a node's hello lists by their manifests (D238
//     §2): one it does not know whose source is still that file on that node is adopted complete,
//     which rebuilds the index after a cache.db discard; one whose making it names is kept, and
//     recorded complete if its end was not heard; any other is not kept, and the node evicts it.
//     TestAdoptRenditions_RebuildsTheIndexFromTheNodesManifestsAndKeepsOnlyWhatItNames, and end to
//     end TestPlayback_ARenditionOnTheNodesDiskIsAdoptedAndAnUnfinishedOneEvicted (cmd/orpheum).
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
//   - Play state records against the source file, never a prepared rendition, and a position moves
//     only for a report about a later moment of watching, anchored from the device's monotonic
//     clock, so two devices reporting out of order never rewind it; a report sent again changes
//     nothing (D217 §7, D165). Enforced by
//     TestPlayState_TwoDevicesReportingOutOfOrderNeverRewindThePosition and
//     TestPlayState_ChangesAndSpansAreLoggedAtTheServersTimeOfTheWatching.
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
//   - A cancelled prepare or subtitle extraction tells its node to stop, and a cancelled
//     rendition is recorded failed, so the next play prepares it again rather than waiting on it
//     (cut-a §5.3). Enforced by
//     TestJobs_TheViewListsAPrepareAndACancelStopsItOnTheNodeSoTheNextPlayPreparesAgain.
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
//   - A complete rendition whose node no longer has its playlist is prepared again, and the player
//     waits as for a new one (D217 §5). Enforced by
//     TestPrepare_ARenditionItsNodeNoLongerHasIsPreparedAgain.
//
//   - Each making of a copy writes into a run directory of its own, so no request reaches an
//     earlier making's files. Enforced by TestPrepare_ACopyMadeWithAnotherSegmentLengthIsMadeAgain.
//
//   - A copy is made with hls_segment_length_seconds's segment length, and one made with another length is
//     made again when next played. Enforced by
//     TestPrepare_ACopyMadeWithAnotherSegmentLengthIsMadeAgain.
//
//   - A copy records the language each audio track was tagged with, and one whose tracks the file's
//     probe now tags otherwise, as a relabel that keeps the payload hash does (D179), is made again
//     when next played. Enforced by TestPrepare_ACopyMadeBeforeAnAudioTrackWasRelabelledIsMadeAgain.
//
//   - A prepare job names its source by root, path and payload hash with every audio track, and
//     the rendition is complete only when the node says so; FFmpeg's failure is not retried, and
//     a node's refusal (a full disk, a file changed since the scan) is tried again, the playback
//     waiting. Enforced by TestPrepare_AJobCommandsTheNodeAndRecordsTheRenditionComplete,
//     TestPrepare_AFailedRenditionFailsThePlaybackAndIsNotRetried and
//     TestPrepare_ARefusedPrepareIsTriedAgainAndThePlaybackKeepsWaiting.
//
//   - A prepare, an extraction or a measurement for a node that is not connected, or that went
//     away while its job waited, parks the job on the node at no cost of an attempt (D231 §11); a
//     prepare the node refuses for want of room parks on its prepared directory until its status
//     shows room; and one for a file under a root or mounted disk the last scan found missing
//     parks on that root until a scan finds it, while the loudness sweep passes such files over
//     (D231 §5). Enforced by TestPrepare_APrepareForANodeThatIsNotConnectedParksOnItAndSendsNothing,
//     TestPrepare_ARefusalForWantOfRoomParksTheNodesPreparesUntilItsStatusShowsRoom and
//     TestLoudness_AFileUnderAMissingRootIsNotSweptAndItsMeasurementWaitsForTheRoot
//     (internal/core/api).
//
//   - A node's word that FFmpeg stalled on a prepare, an extraction or a measurement costs its job
//     no attempt, and two in a row are the target's failure. Enforced by
//     TestHandle_AStalledPrepareIsTriedAgainAndTwoInARowAreTheRenditionsFailure,
//     TestSubtitles_AStallIsTriedAgainAndTwoInARowAreTheTracksFailure and
//     TestLoudness_AStallIsTriedAgainAndTwoInARowAreTheFilesFailure. So does its node starting
//     again while it held the work, which it lost (D238 §3).
//     TestHandle_APrepareItsNodeLostByStartingAgainIsTriedAgainAndTwoInARowAreTheRenditionsFailure.
//
//   - A play of a file that cannot be read now, its node away or its root or a disk inside it
//     missing, is refused before anything is queued (library.RefuseUnreadable), and a
//     play-driven job whose file cannot be read ends rather than parks (D241 §4).
//     TestScenario_S29_APlayWhoseNodeIsAwayIsRefusedAtOnceAndQueuesNothing,
//     TestPlayback_APlayOfAFileUnderAMissingRootIsRefusedAtOnceAndQueuesNothing,
//     TestPrepare_APlayDrivenPrepareWhoseRootGoesMissingEndsRatherThanParks (internal/core/api).
//
//   - A prepare the node holds waiting for a slot outlives the silence limit; how long it waits is
//     bounded by whether the work ahead of it moves (internal/core/nodes, D233 §5). Enforced by
//     TestHandle_APrepareTheNodeHoldsWaitingForASlotOutlivesTheSilenceLimit.
//
//   - A prepare carries its source's length, from which the node sets its deadline. Enforced by
//     TestPrepareCommand_CarriesTheSourcesLengthForTheNodesDeadline.
//
//   - Every text subtitle track of a played video, its own and its subtitle files', is extracted,
//     whatever the decision, once per payload hash of the file it is in; a track not yet extracted
//     is 425 with Retry-After, a failed one is prepare_failed and tried again when next played, a
//     node's refusal is not kept and its job is tried again, and a playback reaches only its own
//     file's tracks (D217 §6). Enforced by
//     TestSubtitles_ARefusedExtractionIsNotKeptAndTheTrackKeepsWaiting,
//     TestSubtitles_ASubtitleFileBesideTheVideoIsOfferedAndConvertedWhole,
//     TestSubtitles_ATextTrackIsOfferedAndQueuedWhenFirstPlayedAndIsTooEarlyUntilExtracted,
//     TestSubtitles_AnExtractedTrackIsServedAsASSAndWebVTTAndNotExtractedAgain and
//     TestSubtitles_AFailedExtractionSaysSoAndIsTriedAgainWhenNextPlayed.
//
//   - The text a node made of a track, or why it could not, is kept in persistent.db, not cache.db:
//     making it again reads a whole video (the architecture map's F1, D231 §3).
//     TestSubtitles_ExtractedTextIsKeptInPersistentDBWhichOutlivesCacheDB.
//
//   - A measurement, an extraction or a conversion whose job fails is kept as its item's own
//     failure, by payload hash as FFmpeg's is (D231 §4, §10): the loudness sweep does not ask
//     again, and a subtitle shows it failed until the next play asks again.
//     TestLoudness_AMeasurementThatGaveUpIsKeptAsTheFilesOwnFailureAndNotSweptAgain,
//     TestSubtitles_AnExtractionThatGaveUpSaysSoAndTheNextPlayAsksAgain.
//
//   - A rendition the node finished records what made its video and how fast, as the node said
//     (D225 §3). TestFinish_RecordsWhatMadeTheVideoAndHowFast.
//
//   - A subtitle fetched on request is found by the work's ids or the series' and place, and the
//     file's moviehash, with a candidate whose hash matches listed first; is fetched by anyone
//     signed in; is kept gzipped in persistent.db; becomes the person's choice; and is offered as a
//     track of the file, which the video's node converts (D224).
//     TestSubtitleCandidates_AreFoundByTheWorksIDsAndTheFilesHashWithTheDownloadsLeft,
//     TestSubtitleCandidates_ForAnEpisodeAskByTheSeriesAndItsPlace,
//     TestSubtitleCandidates_APersonWhoIsNotAnAdminListsAndFetchesThem,
//     TestFetchSubtitle_KeepsItChoosesItAndItPlaysOnceTheNodeConvertsIt (internal/core/api).
//
//   - A fetched subtitle is converted by the video's node while the person waits, by the conversion
//     job at play priority, the one way a node is asked for work (D238), and kept and made their
//     choice only once converted: nothing unparsed becomes a choice, one the node cannot read or did
//     not convert within the silence limit is kept nowhere, and a node that is away is found before
//     a download is spent (D224 §5). TestFetchSubtitle_OnlyASubtitleItsNodeConvertedIsKeptAndChosen,
//     TestFetchSubtitle_OneItsNodeDidNotConvertInTimeIsKeptNowhere (internal/core/api).
//
//   - Levelling is the person's, in every file, on until they turn it off (D226 §5). Enforced by
//     TestLoudness_LevellingIsOnUntilThePersonTurnsItOff.
//
// # Read before changing
//
//   - D217: the menu, the media token, and what M3 decided about playback; §6, subtitles.
//   - D230: the last language chosen in a work follows its unwatched files; D24: what a choice is.
//   - D157: a position belongs to the File.
//   - D165: the log records state changes and spans; an offline device must not rewind.
//   - D180: a prepared rendition's play state is its source's.
//   - D81: play state is an append-only event log plus a guarded position.
//   - D191: signed-in Watch Together viewing is also recorded.
//   - D83, D222: the dialogue boost is a person's; a device's sound decides whether we fold it down.
//   - D224: fetching a subtitle on request; the provider client is internal/core/providers/opensubtitles.
//   - D225: a finished rendition records what made its video and how fast, for the System page.
//   - D226: loudness is measured by the node (internal/core/loudness) and levelled by the player.
//   - docs/contributors/design/plans/play-state.md, docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - tokens.go: media tokens, in memory. playback.go: starting a playback and finding a direct
//     playback's bytes. prepare.go: renditions' rows, their jobs and the nodes' reports. hls.go:
//     a prepared playback's files, and the prepare job's handler. subtitles.go: the text tracks a
//     playback offers, their extraction jobs and their text; a job asks a node for work through
//     nodes.Exchanges. report.go: the play report, its log and the guarded position. resume.go: where a
//     playback starts. hide.go: hiding a work from Continue Watching, which library's home.go
//     lists. finished.go: a node's finished prepares, for the System page. fetch.go: subtitles
//     fetched on request, and the job that converts one. choices.go: a person's
//     choices in a file, kept through internal/core/preferences, and a file's audio tracks. adopt.go: the index
//     squared with a node's renditions at its hello. Next Up comes after M3.
package playback

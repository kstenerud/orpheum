// SPDX-License-Identifier: AGPL-3.0-or-later

// Package renditions has a file's prepared copies made by its node and keeps their index in
// cache.db: it wants a rendition for a playback, queues and runs its prepare job, records the
// node's reports by the making they name, reconciles the index with a node's renditions at its
// hello, and says where a rendition's files are (cut-a §6). Which playback needs a copy is
// internal/core/decide's, and the media token that reaches it is internal/core/playback's.
//
// # Invariants
//
//   - A prepare is given to its node, not waited for: its handler records what it begins and
//     answers with the job assigned (nodes.Exchanges.Assign), and its kind's Assignee resolves it
//     from the node's word, a silence or a restart with the answers the handler gave before; a
//     prepare cancelled while its node holds it tells the node to stop and has its making recorded
//     failed, so the next play prepares again rather than waiting on it (D234 §7, cut-a §5.3).
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
//   - A node's result on a prepare is recorded as it arrives, by what its echoed work names,
//     whether or not its job still waits (D238 §2, S81): a making of a rendition by its run, so a
//     rendition made again since is never completed by an earlier one and nothing heard after an
//     end undoes it (G13). The job only learns how the work ended.
//     TestRecordPrepare_RecordsAMakingByItsRunWhetherOrNotItsJobWaits.
//
//   - The index of renditions is reconciled with those a node's hello lists by their manifests (D238
//     §2): one it does not know whose source is still that file on that node is adopted complete,
//     which rebuilds the index after a cache.db discard; one whose making it names is kept, and
//     recorded complete if its end was not heard; any other is not kept, and the node evicts it.
//     TestAdoptRenditions_RebuildsTheIndexFromTheNodesManifestsAndKeepsOnlyWhatItNames, and end to
//     end TestPlayback_ARenditionOnTheNodesDiskIsAdoptedAndAnUnfinishedOneEvicted (cmd/orpheum).
//
//   - A complete rendition whose node no longer has its playlist is prepared again, and the player
//     waits as for a new one (D217 §5). Enforced by
//     TestPrepare_ARenditionItsNodeNoLongerHasIsPreparedAgain.
//
//   - Each making of a copy writes into a run directory of its own, so no request reaches an
//     earlier making's files. Enforced by TestPrepare_ACopyMadeWithAnotherSegmentLengthIsMadeAgain.
//
//   - A copy is made with hls_segment_length_seconds's segment length, and one made with another
//     length is made again when next played. Enforced by
//     TestPrepare_ACopyMadeWithAnotherSegmentLengthIsMadeAgain.
//
//   - A copy records the language each audio track was tagged with, and one whose tracks the file's
//     probe now tags otherwise, as a relabel that keeps the payload hash does (D179), is made again
//     when next played. Enforced by
//     TestPrepare_ACopyMadeBeforeAnAudioTrackWasRelabelledIsMadeAgain.
//
//   - A prepare job names its source by root, path and payload hash with every audio track, and the
//     rendition is complete only when the node says so; FFmpeg's failure is not retried, and a
//     node's refusal (a full disk, a file changed since the scan) is tried again, the playback
//     waiting. Enforced by TestPrepare_AJobCommandsTheNodeAndRecordsTheRenditionComplete,
//     TestPrepare_AFailedRenditionFailsThePlaybackAndIsNotRetried and
//     TestPrepare_ARefusedPrepareIsTriedAgainAndThePlaybackKeepsWaiting.
//
//   - A prepare for a node that is not connected, or that went away while its job waited, parks the
//     job on the node at no cost of an attempt (D231 §11); one the node refuses for want of room
//     parks on its prepared directory until its status shows room; and one for a file under a root
//     or mounted disk the last scan found missing parks on that root until a scan finds it (D231
//     §5). Enforced by TestPrepare_APrepareForANodeThatIsNotConnectedParksOnItAndSendsNothing and
//     TestPrepare_ARefusalForWantOfRoomParksTheNodesPreparesUntilItsStatusShowsRoom
//     (internal/core/api).
//
//   - A node's word that FFmpeg stalled on a prepare costs its job no attempt, and two in a row are
//     the rendition's failure. Enforced by
//     TestHandle_AStalledPrepareIsTriedAgainAndTwoInARowAreTheRenditionsFailure. Its node starting
//     again while it held the work, which it lost, is an attempt's failure and never the
//     rendition's (D248).
//     TestHandle_APrepareItsNodeLostByStartingAgainIsTriedAgainAndNeverTheRenditionsFailure.
//
//   - A prepare the node holds waiting for a slot outlives the silence limit; how long it waits is
//     bounded by whether the work ahead of it moves (internal/core/nodes, D233 §5). Enforced by
//     TestHandle_APrepareTheNodeHoldsWaitingForASlotOutlivesTheSilenceLimit.
//
//   - A prepare carries its source's length, from which the node sets its deadline. Enforced by
//     TestPrepareCommand_CarriesTheSourcesLengthForTheNodesDeadline.
//
//   - A rendition the node finished records what made its video and how fast, as the node said
//     (D225 §3). TestFinish_RecordsWhatMadeTheVideoAndHowFast.
//
//   - A play-driven prepare whose file cannot be read ends rather than parks: no one can watch it
//     (D241 §4). TestPrepare_APlayDrivenPrepareWhoseRootGoesMissingEndsRatherThanParks
//     (internal/core/api).
//
//   - Imports nothing from internal/node, never os/exec, no database/sql: the depguard rules
//     core-boundary and sql-only-in-store, TestArch_CoreNeverImportsNode and
//     TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - D217 §3, §5: what a prepared copy holds, and one its node no longer has.
//   - D225 §3: a finished rendition records what made its video and how fast, for the System page.
//   - D226 §4: the gains a making puts in its audio tracks.
//   - D231 §5, §11: parking on a node, its prepared directory and a missing root.
//   - D234 §3, §7, D238 §2: work given to a node, kept wanted while a person waits, and results
//     known by what they are.
//   - docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - prepare.go: renditions' rows, their jobs and the nodes' reports. handle.go: the prepare job's
//     handler, and its failure. record.go: a node's result recorded by its making. audio.go: a
//     rendition's audio renditions and their levelling. files.go: where a rendition's files are,
//     and preparing one again. adopt.go: the index reconciled with a node's renditions at its hello.
//     finished.go: a node's finished prepares, for the System page. service.go: what the service
//     is built from.
package renditions

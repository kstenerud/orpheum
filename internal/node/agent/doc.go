// SPDX-License-Identifier: AGPL-3.0-or-later

// Package agent is the node's loop (D207): it dials the core's socket, says hello, long-polls for
// commands and holds the work they give it in memory (D238 §3), each job starting once a slot of its
// kind and a reader of its device are free, and posts its status every tick. When the core goes
// away it says hello again at the next tick.
//
// # Invariants
//
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//   - An artwork command naming an image beside the media is read through the root it is under,
//     never from outside the node's roots, and answered with the SHA-256 of what was read
//     (D215 §3). TestAgent_ReadsAnImageBesideTheMediaOnlyUnderItsRoots.
//   - The node posts its status every tick, for the core to reconcile with what it gave the node
//     (D234 §6). TestAgent_SaysHelloAndPostsItsStatusEachTick.
//   - A job's background work reads the device its file is on, and runs only with a reader of it
//     free, its limit the core's, one when the core named none; a paused run holds its slot but no
//     reader. Play-driven work never waits for another's there, nor for its slot (D240 §6).
//     TestWorkList_ADevicesReadersAreBoundedByItsLimit,
//     TestAgent_ACommandsWorkReadsItsFilesDeviceAndPlayDrivenWorkPausesTheBackground,
//     TestWorkList_PlayDrivenWorkNeverWaitsForAnothersOnItsDevice,
//     TestWorkList_PlayDrivenWorkNeverWaitsForAnothersSlot.
//   - Play-driven work that finds its device's readers taken pauses the background work reading
//     it, which continues once it ends; an examining of the device waits between files meanwhile; a device
//     with room for both pauses nothing (D240 §6). The status says what is paused.
//     TestWorkList_PlayDrivenWorkPausesBackgroundWorkOnItsDeviceUntilItEnds,
//     TestWorkList_ADeviceWithRoomForBothPausesNothing.
//   - A reprioritize command gives the attempt it names its new priority and its place in line
//     (D234 §3). TestWorkList_AReprioritizedJobTakesItsNewPlaceInLine,
//     TestAgent_AReprioritizeCommandGivesTheAttemptItsNewPriority.
//   - Each hello lists the devices the roots are on as found then (D240 §5).
//     TestAgent_TheHelloListsTheDevicesFoundAsItIsSaid.
//   - Each hello evicts the makings in the prepared directory that never finished, and what a
//     whole-film prepare left (D238 §2, D344 §1). TestAgent_EachHelloEvictsTheMakingsThatNeverFinished.
//   - A root is listed or examined only if the node was configured with it, whatever the
//     core asks (D107). TestAgent_RefusesToListOrExamineARootItWasNotGiven. Listing and examining:
//     TestAgent_ListsARootAndExaminesTheFilesTheCoreAsksFor. A library folder inside a root is listed
//     and examined as a root of its own, reached without a link, its readings keyed by the root, and
//     a run reads a file named under it (D359 §1, §4).
//     TestAgent_ListsAndExaminesALibraryFolderInsideARootReachedWithoutALink,
//     TestAgent_RefusesALibraryFolderReachedThroughALinkOrClimbingOutOfItsRoot,
//     TestAgent_ReadsThroughAVideoNamedUnderALibraryFolderInsideARoot.
//   - A listing or an examining holds no slot, only its device's reader; an examining, which reads in
//     the node's own process, waits between files while it is paused (D240 §5, §6).
//     TestWorkList_WorkHoldingNoSlotWaitsOnlyForItsDevice,
//     TestWorkList_PausedWorkWaitsBetweenFilesUntilItIsContinued.
//   - Each held attempt says in the status how long it has not moved, its paused time apart:
//     running work moves as its run says (execx.WithMoved), waiting work as the work running does
//     (D233 §5). TestWorkList_AJobMovesAsItsRunSaysAndAWaitingOneAsTheRunningOneDoes,
//     TestMovement_PausedTimeIsNotTimeWithoutMoving.
//   - A listing or an examining whose reading does not move for the read window is answered as
//     stalled on its disk, its reading listed as stuck until it returns and its late answer dropped
//     (D233 §10). TestAgent_AListingThatStopsMovingIsAnsweredStalledAndListedAsStuck. A file to read
//     that does not open within the open stall window is the same: its work is answered stalled on
//     its disk, waiting on the file, and the open listed as stuck until it returns.
//     TestAgent_AFileThatDoesNotOpenIsAnsweredStalledOnItsDiskAndListedAsStuck. The status
//     lists the processes the runner left running too. TestStatus_ListsTheProcessesLeftRunning.
//   - Losing the core ends the session and the next tick starts another.
//     TestAgent_SaysHelloAgainAfterLosingTheCore. Only the session's own requests, a status and
//     the command poll, end it. A job's result is posted beside the node's other work,
//     holding no slot, until the core acknowledges it, with a doubling wait and at once at the next
//     hello, and never given up while the node runs (D234 §5); it never stops the session or the
//     work running beside it. A session that ends stops none of the node's work: a run of chunks,
//     extraction, read-through or re-encode runs on the node's lifetime and posts its result to the
//     next session's core (D233 §2). TestAgent_ASessionThatEndsStopsNoWorkAndItsResultReachesTheNextSession,
//     TestAgent_AResultIsSentUntilTheCoreAcknowledgesIt,
//     TestAgent_AnUnacknowledgedResultIsSentAgainAtTheNextHello,
//     TestAgent_AnArtworkAnswerTheCoreRefusedOnceArrivesOnRetryAndStopsNoOtherWork,
//     TestAgent_ASubtitleAnswerTheCoreRefusedOnceArrivesOnRetry,
//     TestAgent_ARunsEndTheCoreRefusedArrivesOnRetry.
//   - A result whose JSON is larger than nodeproto.MaxPostedResultBytes is held in memory for the
//     core to fetch from the byte socket, and posted as its job, attempt, work, size and hash; it is
//     forgotten once acknowledged (D234 §5). TestAgent_AResultTooLargeToPostIsHeldUntilAcknowledged.
//   - An attempt that ended is remembered, its result's content forgotten once the core
//     acknowledged it, for node.ended_work_kept_hours after (D238 §2): the same command sent again is
//     not run again. TestAgent_ACommandSentAgainForAnAttemptThatEndedIsNotRunAgain.
//   - A command the node cannot read, unknown or without its payload, is refused by its job's id,
//     never ignored or crashed on, and every hello says this build's protocol version (D233 §9).
//     TestAgent_RefusesACommandItCannotReadAndSaysItsProtocolInItsHello.
//   - Every job's answer is a Result naming the job its command named, posted to the core's one
//     result path; an artwork command's — each image's sizes, or why none, re-encoded in turn and
//     answered once for all, each naming its image's key — is the artwork store's one way in
//     (D212 §8). TestAgent_ReEncodesArtworkAndPostsTheAnswer,
//     TestAgent_ReEncodesAJobsImagesAndAnswersOnceForAll.
//   - A run of chunks runs only on the file the core named, a regular file under one of the node's
//     roots whose payload hash is still the one named (D100), opened and checked by
//     internal/node/namedfile; how far it is and how it ended are posted to the core. One encode
//     and two remuxes run at once (cut-a §5.3).
//     TestAgent_MakesChunksOfOnlyTheVideoTheCoreNamedAndSaysEachAsItIsMade,
//     TestAgent_ARunOfAFileUnderNoRootOrOfADirectoryIsRefused.
//   - A subtitle is extracted only from the video or sidecar the core named, a regular file under
//     one of the node's roots whose payload hash is still the one named (D100, D217 §6). One extraction runs
//     at once; a video's answers each track it named, with its text or its own failure (D340).
//     TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot,
//     TestAgent_AnswersEachTrackOfAVideoWithItsTextOrItsOwnFailure.
//     A fetched subtitle's text, which the command carries, is converted as a sidecar is (D224 §5).
//     TestAgent_ConvertsAFetchedSubtitlesTextAndRefusesItBesideAFile.
//   - A read-through reads only the video the core named, whose payload hash is still the one
//     named, and answers what it was asked: the loudness, the picture's timeline, or both (D100,
//     D226, D247 §10). One read-through runs at once.
//     TestAgent_ReadsThroughOnlyTheVideoTheCoreNamed, TestAgent_AReadThroughAnswersEachPartItWasAskedFor.
//   - A run of chunks makes chunks only of the video the core named, tells the core each chunk as it
//     is made, and says how far it got however it ended; a quality runs in an encode slot, a copied
//     picture or a sound in a remux slot (D247 §10).
//     TestAgent_MakesChunksOfOnlyTheVideoTheCoreNamedAndSaysEachAsItIsMade.
//   - Each run of FFmpeg on a film the core named gets it from openFilm: on a node whose FFmpeg reads
//     through it (Workers.Films, D330), served from the very file the node opened and checked,
//     through its disk's gate and within a whole read's budget, the run given no path; otherwise by
//     its path, as before. TestAgent_ARunIsServedTheFileTheNodeCheckedOnlyWhenItsFFmpegReadsThroughIt.
//     A disc image's run is served its main title, and refused on a node that cannot stream it
//     (D350). TestAgent_ARunOfADiscImageIsServedItsMainTitle.
//   - A file a command names is checked against its payload hash through the node's one
//     namedfile.Hashes (Workers.Hashes), which the byte socket shares: a film opened unchanged by
//     run after run is hashed once. TestAgent_RunsOfAnUnchangedFilmHashItOnceInTheHashesTheNodeShares.
//   - A cancel is carried out as it arrives, not queued behind other work: it stops the job's
//     FFmpeg, frees its slot, and nothing more is reported for the job; a job cancelled while it
//     waits for a slot never starts (cut-a §5.3).
//     TestAgent_ACancelStopsARunningRunAtOnceAndNothingMoreIsReportedForIt,
//     TestAgent_AnArtworkJobTheCoreCancelledIsStopped,
//     TestWorkList_ACancelledWaitingJobNeverStartsAndARunningOneStops.
//   - Every attempt the node holds says where it is on the progress path, best effort and never
//     sent again (D234 §5): running when it starts, waiting at once when its slot is taken, and
//     each again at every status tick; a running run of chunks says how far it is as it makes each,
//     and a read-through as FFmpeg moves on, at most every node_progress_every_seconds the core's
//     tuning says, instead of at the tick.
//     TestAgent_EveryKindOfWorkSaysWhereItIsWhenItStartsAndAtEachTick,
//     TestAgent_ARunWaitingForASlotSaysSoAtOnceAndAtEachTick.
//   - The work the node holds is in memory, never on disk (D238 §3), held by its attempt's token,
//     which a cancel and every result name, and a command without one is refused; and a run is
//     held beside a scan's examining, never queued behind one. What waits for a slot of its kind
//     starts the highest priority first, the command's, and at one priority the first to come (D234 §3); an
//     attempt the core sends again while it is held is held once.
//     TestWorkList_StartsWhatWaitsByPriorityThenArrivalAndHoldsAJobOnce,
//     TestAgent_ARunRunsWhileAnExaminingIsStillGoing,
//     TestAgent_HoldsWorkByItsAttemptAndEveryReportNamesIt.
//   - A run of chunks, subtitle or read-through the node did not try (no worker for it, a file outside
//     its roots or changed since the scan, a disk with no room) is answered as a refusal, apart
//     from FFmpeg failing on the bytes, so the core can try a refusal again; a file changed since the
//     scan is named so (nodeproto.RefusedChanged, S74).
//     TestAgent_ARunTheDiskHasNoRoomForIsRefusedAndOneFFmpegFailsOnFails,
//     TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot.
//   - A run of chunks, subtitle or read-through whose FFmpeg the watchdog stopped is answered as a stall,
//     with what was seen, apart from a failure and a refusal, and its slot is freed.
//     TestAgent_AStalledRunIsAnsweredStalledNotFailedAndFreesItsSlot,
//     TestAgent_AStalledReadThroughOrExtractionIsAnsweredStalledNotFailed.
//   - A run of chunks that ends brings the prepared directory back within its budget then, not only
//     when the next run makes room (D217 §5).
//     TestAgent_AFinishedRunKeepsThePreparedDirectoryWithinItsBudget.
//   - The status lists every attempt the node holds, waiting or running, and every one whose result
//     the core has not acknowledged, with the last command the node took, for the core to reconcile
//     (D234 §6). TestStatus_ListsTheAttemptsTheNodeHoldsAndTheLastCommandItTook,
//     TestEndedAttempts_ListTheResultsNotYetAcknowledged.
//   - The status carries the disk of each root and of the prepared directory, or why one could
//     not be measured (D225). TestStatus_CarriesTheDisksOfTheRootsAndThePreparedDirectory.
//   - Each session posts the watcher's notices, those held while there was none first, and the
//     status says whether each library folder is watched (D272 §9).
//     TestAgent_PostsTheWatchersNoticesAndSaysEachRootsWatchInItsStatus. The watcher watches the
//     library folders the core names in the hello's reply and each poll's answer (D359's amendment of
//     2026-10-27). TestAgent_HasTheWatcherWatchTheLibraryFoldersTheCoreNames.
//
// # Read before changing
//
//   - D107: a node's roots are set on the node.
//   - D127, D193 §4: the node dials out; the one transport is HTTP and JSONL over a Unix socket.
//   - D34: silence is only ever read as silence; the status record is how a node is heard.
//   - D212: artwork is re-encoded here, and posted to the core.
//   - D217 §6: subtitles are extracted when it is cheap to.
//   - D226: loudness is measured by a job, one read of the file for every track.
//   - D247 §10: the picture's timeline, read in the same pass as the loudness.
//   - docs/contributors/design/plans/cut-a.md §4.
//
// # Where to change
//
//   - agent.go: the session, its poll and its status, and the commands; listing.go: listing and
//     examining;
//     answer.go: posting a job's result, and trying it again; progress.go: saying where a job is;
//     open.go: opening the file a command names; film.go: a run's film, from the file the node
//     opened; chunks.go: runs of chunks;
//     subtitle.go: subtitle extraction, and work that posts its whole answer when it ends;
//     readthrough.go: reading a video through; working.go: the work held, its slots and priority order,
//     and cancel.
//   - held.go: results too large to post, held for the byte socket until acknowledged (D234 §5);
//     movement.go: when held work last moved, for the watchdog; stall.go: what the core is told
//     of a run stopped as stalled; stuck.go: a listing's or examining's own reading left stuck.
package agent

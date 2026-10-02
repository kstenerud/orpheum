// SPDX-License-Identifier: GPL-3.0-or-later

// Package agent is the node's loop (D207): it dials the core's socket, says hello, long-polls for
// commands, runs its scans one at a time and holds the rest of its work in memory (D238 §3), each
// job starting once a slot of its kind is free, and streams its report — scan records, and a status
// record every tick — as one JSONL request whose records reach the core as they are written. When
// the core goes away it says hello again at the next tick, and numbers its records on from the last
// one the core ingested.
//
// # Invariants
//
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//   - An artwork command naming an image beside the media is read through the root it is under,
//     never from outside the node's roots, and answered with the SHA-256 of what was read
//     (D215 §3). TestAgent_ReadsAnImageBesideTheMediaOnlyUnderItsRoots.
//   - Records reach the core in the order of their numbers, numbered on from the core's last,
//     each as soon as it is written. TestAgent_SaysHelloRunsAScanAndStreamsItsReportInNumberedOrder.
//   - A root is scanned only if the node was configured with it, whatever the core asks (D107).
//     TestAgent_RefusesToScanARootItWasNotGiven.
//   - Losing the core ends the session and the next tick starts another.
//     TestAgent_SaysHelloAgainAfterLosingTheCore. Only the session's own requests, the report
//     stream and the command poll, end it. A job's result is posted beside the node's other work,
//     holding no slot, until the core acknowledges it, with a doubling wait and at once at the next
//     hello, and never given up while the node runs (D234 §5); it never stops the session or the
//     work running beside it. A session that ends stops none of the node's work: a prepare,
//     extraction, measurement or re-encode runs on the node's lifetime and posts its result to the
//     next session's core (D233 §2). TestAgent_ASessionThatEndsStopsNoWorkAndItsResultReachesTheNextSession,
//     TestAgent_AResultIsSentUntilTheCoreAcknowledgesIt,
//     TestAgent_AnUnacknowledgedResultIsSentAgainAtTheNextHello,
//     TestAgent_AnArtworkAnswerTheCoreRefusedOnceArrivesOnRetryAndStopsNoOtherWork,
//     TestAgent_ASubtitleAnswerTheCoreRefusedOnceArrivesOnRetry,
//     TestAgent_APreparesEndTheCoreRefusedArrivesOnRetry.
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
//   - A prepare runs only on the file the core named, a regular file under one of the node's roots
//     whose payload hash is still the one named (D100), opened and checked by
//     internal/node/namedfile; how far it is and how it ended are posted to the core. One
//     encode and two remuxes run at once (cut-a §5.3). TestAgent_PreparesOnlyTheFileTheCoreNamed.
//   - A subtitle is extracted only from the video or sidecar the core named, a regular file under
//     one of the node's roots whose payload hash is still the one named (D100, D217 §6). One extraction runs
//     at once. TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot.
//     A fetched subtitle's text, which the command carries, is converted as a sidecar is (D224 §5).
//     TestAgent_ConvertsAFetchedSubtitlesTextAndRefusesItBesideAFile.
//   - A loudness measurement reads only the video the core named, whose payload hash is still the
//     one named (D100, D226). One measurement runs at once.
//     TestAgent_MeasuresOnlyTheVideoTheCoreNamed.
//   - A cancel is carried out as it arrives, not queued behind other work: it stops the job's
//     FFmpeg, frees its slot, and nothing more is reported for the job; a job cancelled while it
//     waits for a slot never starts (cut-a §5.3).
//     TestAgent_ACancelStopsARunningPrepareAtOnceAndNothingMoreIsReportedForIt,
//     TestAgent_AnArtworkJobTheCoreCancelledIsStopped,
//     TestWorkList_ACancelledWaitingJobNeverStartsAndARunningOneStops.
//   - Every attempt the node holds says where it is on the progress path, best effort and never
//     sent again (D234 §5): running when it starts, waiting at once when its slot is taken, and
//     each again at every status tick; a running prepare says how far it is as FFmpeg moves on, at
//     most every node.progress_every_seconds, instead of at the tick.
//     TestAgent_EveryKindOfWorkSaysWhereItIsWhenItStartsAndAtEachTick,
//     TestAgent_APrepareWaitingForASlotSaysSoAtOnceAndAtEachTick.
//   - The work the node holds is in memory, never on disk (D238 §3), held by its attempt's token,
//     which a cancel and every result name, and a command without one is refused; and it is held
//     beside its scans, never queued behind one. What waits for a slot of its kind starts the
//     highest priority first, the command's, and at one priority the first to come (D234 §3); an
//     attempt the core sends again while it is held is held once.
//     TestWorkList_StartsWhatWaitsByPriorityThenArrivalAndHoldsAJobOnce,
//     TestAgent_APrepareRunsWhileAScanIsStillGoing,
//     TestAgent_HoldsWorkByItsAttemptAndEveryReportNamesIt.
//   - A finished prepare's report says what made its video and how fast (D225 §3).
//     TestAgent_PreparesOnlyTheFileTheCoreNamed.
//   - A prepare, subtitle or measurement the node did not try (no worker for it, a file outside
//     its roots or changed since the scan, a disk with no room) is answered as a refusal, apart
//     from FFmpeg failing on the bytes, so the core can try a refusal again; a file changed since the
//     scan is named so (nodeproto.RefusedChanged, S74).
//     TestAgent_APrepareTheDiskHasNoRoomForIsRefusedAndOneFFmpegFailsOnFails,
//     TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot.
//   - A prepare, subtitle or measurement whose FFmpeg the watchdog stopped is answered as a stall,
//     with what was seen, apart from a failure and a refusal, and its slot is freed.
//     TestAgent_AStalledPrepareIsAnsweredStalledNotFailedAndFreesItsSlot,
//     TestAgent_AStalledMeasurementOrExtractionIsAnsweredStalledNotFailed.
//   - A prepare that ends brings the prepared directory back within its budget then, not only
//     when the next prepare makes room (D217 §5).
//     TestAgent_AFinishedPrepareKeepsThePreparedDirectoryWithinItsBudget.
//   - The status lists every attempt the node holds, waiting or running, and every one whose result
//     the core has not acknowledged, with the last command the node took, for the core to square
//     (D234 §6). TestStatus_ListsTheAttemptsTheNodeHoldsAndTheLastCommandItTook,
//     TestEndedAttempts_ListTheResultsNotYetAcknowledged.
//   - The status carries the disk of each root and of the prepared directory, or why one could
//     not be measured (D225). TestStatus_CarriesTheDisksOfTheRootsAndThePreparedDirectory.
//
// # Read before changing
//
//   - D107: a node's roots are set on the node.
//   - D127, D193 §4: the node dials out; the one transport is HTTP and JSONL over a Unix socket.
//   - D34: silence is only ever read as silence; the status record is how a node is heard.
//   - D212: artwork is re-encoded here, and posted to the core.
//   - D217 §6: subtitles are extracted when it is cheap to.
//   - D226: loudness is measured by a job, one read of the file for every track.
//   - docs/contributors/design/plans/cut-a.md §4.
//
// # Where to change
//
//   - agent.go: the session and its four parts, the commands and scans; report.go: numbering;
//     answer.go: posting a job's result, and trying it again; progress.go: saying where a job is;
//     prepare.go: prepares;
//     subtitle.go: subtitle extraction, and work that posts its whole answer when it ends;
//     loudness.go: loudness measurement; working.go: the work held, its slots and priority order,
//     and cancel.
package agent

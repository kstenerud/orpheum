// SPDX-License-Identifier: GPL-3.0-or-later

// Package agent is the node's loop (D207): it dials the core's socket, says hello, long-polls for
// commands and runs them one at a time — except prepares, subtitle extractions and loudness
// measurements, which run beside the rest in their own slots — and streams its report — scan
// records, and a status record every tick — as one JSONL request whose records reach the core as
// they are written. When the core goes away it says hello again at the next tick, and numbers its
// records on from the last one the core ingested.
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
//     stream and the command poll, end it: a job's answer the core did not take is posted again
//     with a doubling wait for about two minutes, then logged and given up, and never stops the
//     session or the work running beside it. A session that ends stops none of the node's work: a
//     prepare, extraction, measurement or re-encode runs on the node's lifetime and posts its result
//     to the next session's core (D233 §2). TestAgent_ASessionThatEndsStopsNoWorkAndItsResultReachesTheNextSession.
//     TestAgent_AnArtworkAnswerTheCoreRefusedOnceArrivesOnRetryAndStopsNoOtherWork,
//     TestAgent_ASubtitleAnswerTheCoreRefusedOnceArrivesOnRetry,
//     TestAgent_APreparesEndTheCoreRefusedArrivesOnRetry.
//   - A command the node cannot read, unknown or without its payload, is refused by its job's id,
//     never ignored or crashed on, and every hello says this build's protocol version (D233 §9).
//     TestAgent_RefusesACommandItCannotReadAndSaysItsProtocolInItsHello.
//   - Every job's answer is a Result naming the job its command named, posted to the core's one
//     result path; an artwork command's — the sizes the deriver made, or why none — is the
//     artwork store's one way in (D212 §8). TestAgent_ReEncodesArtworkAndPostsTheAnswer.
//   - A prepare runs only on the file the core named, a regular file under one of the node's roots
//     whose payload hash is still the one named (D100), opened and checked by
//     internal/node/namedfile; its progress and its end are posted to the core. One
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
//     FFmpeg, frees its slot, and nothing more is reported for the job; a job cancelled before it
//     started starts cancelled (cut-a §5.3).
//     TestAgent_ACancelStopsARunningPrepareAtOnceAndNothingMoreIsReportedForIt,
//     TestAgent_AnArtworkJobTheCoreCancelledIsStopped.
//   - A finished prepare's report says what made its video and how fast (D225 §3).
//     TestAgent_PreparesOnlyTheFileTheCoreNamed.
//   - A prepare, subtitle or measurement the node did not try (no worker for it, a file outside
//     its roots or changed since the scan, a disk with no room) is answered as a refusal, apart
//     from FFmpeg failing on the bytes, so the core can try a refusal again.
//     TestAgent_APrepareTheDiskHasNoRoomForIsRefusedAndOneFFmpegFailsOnFails,
//     TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot.
//   - A prepare, subtitle or measurement whose FFmpeg the watchdog stopped is answered as a stall,
//     with what was seen, apart from a failure and a refusal, and its slot is freed.
//     TestAgent_AStalledPrepareIsAnsweredStalledNotFailedAndFreesItsSlot,
//     TestAgent_AStalledMeasurementOrExtractionIsAnsweredStalledNotFailed.
//   - A prepare that ends brings the prepared directory back within its budget then, not only
//     when the next prepare makes room (D217 §5).
//     TestAgent_AFinishedPrepareKeepsThePreparedDirectoryWithinItsBudget.
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
//   - agent.go: the session and its four parts; report.go: numbering; answer.go: posting a job's
//     result, and trying it again; prepare.go: prepares and their slots; subtitle.go: subtitle
//     extraction, and running a command in the background; loudness.go: loudness measurement;
//     working.go: the jobs running, and cancel.
package agent

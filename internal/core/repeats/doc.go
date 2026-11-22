// SPDX-License-Identifier: AGPL-3.0-or-later

// Package repeats is the fingerprint detector (D101, D115): its sweep queues a find_repeats job for
// each season whose videos' bytes are not measured yet, its handler gives the season's videos to the
// node that holds them, and what the node hears repeated becomes their intro and credits. It reads the
// library through internal/core/library and keeps its segments through it; browse never calls it.
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
//   - It writes repeat_measurement alone; a video's segments are internal/core/library's table, and
//     it replaces the fingerprint's rows through library.ReplaceDetectedSegments.
//     TestArch_EachTableHasOneWritingPackage.
//
//   - What a video's measurement is thrown out by, a sound's facts changed in a probe read again or a
//     discarding rescan (ForgetMeasurements, D392 §3, D535 §6), throws out its fingerprint segments
//     with it, a person's and the chapters' kept, so nothing the app found of its repeats outlives
//     what it was found from (D535 §10).
//     TestForgetMeasurements_ThrowsOutEveryVersionOfOneVideosMeasurement,
//     TestForgetMeasurements_ThrowsOutTheSegmentsItsRepeatsWereKeptAs.
//
//   - The sweep queues a find_repeats job for each season of an aired ordering, on a node, of two
//     episodes or more, with a video whose bytes are not measured at the measurement version, aimed at
//     the season (its group) and naming the node in its arguments, so the job is the season's and one
//     is open for a season at a time; a season on two nodes is compared on one, then the other. It
//     parks on the lowest such video and names one video of each episode, at most repeatsVideosPerJob,
//     a video whose sound failed left out. A node's spans become the opening's intro and the closing's
//     credits, provider fingerprint, as sure as the share of the videos compared they are heard in,
//     replacing only the fingerprint's earlier rows; a failure is kept as the bytes' measurement, a
//     refusal is not, and an answer for a video no longer in the season is not kept (D101, D115).
//     TestQueueRepeats_QueuesEachSeasonNotMeasuredOnce,
//     TestRepeats_SpansBecomeIntroAndCreditsWithTheirConfidence,
//     TestRepeats_APersonsSegmentSurvivesAFingerprintAgain,
//     TestRepeats_AFailedVideoIsNotAskedAgainAndARefusedOneIs,
//     TestRepeats_AnswersForAVideoGoneFromTheSeasonAreIgnored.
//
// # Read before changing
//
//   - D101: a measurement that spans files is a job the core supplies; the node never chooses the set.
//   - D115: intro detection is fingerprint first.
//   - D82: media segments are typed and carry their provider; a person's beats any detector.
//   - docs/contributors/design/plans/media-segments.md.
//
// # Where to change
//
//   - repeats.go: the seasons, their jobs, the command a job gives its node, and what is kept of its
//     answer.
package repeats

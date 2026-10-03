// SPDX-License-Identifier: GPL-3.0-or-later

// Package loudness measures how loud each file's sounds are and keeps the measurements by the
// file's payload hash, and says the gains that level them (D226). The arithmetic of a gain is
// internal/core/decide's (levels.go); a playback reads the levels through Service.Levels.
//
// # Invariants
//
//   - Each live video with sound is measured once per payload hash, by a job behind anything a
//     person waits on; one FFmpeg cannot measure is not asked again, and one its node would not
//     try is (D226 §2): its job is tried again on its back-off, and once it gives up the next
//     sweep asks again. Enforced by TestLoudness_AFileIsMeasuredOnceAndPlaysAtTheLevel,
//     TestLoudness_AFileFFmpegCannotMeasureIsNotAskedAgainButOneTheNodeDidNotTryIs and
//     TestLoudness_AMeasurementTheNodeRefusedIsTriedAgainOnItsBackoff (internal/core/api), and end
//     to end by TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled (cmd/orpheum).
//
//   - A measurement is given to its node, not waited for, and recorded as its result arrives, by
//     the file's payload hash, whether or not its job still waits (D234 §7, D238 §2).
//
//   - A measurement for a file under a root or mounted disk the last scan found missing parks on
//     that root, and the sweep passes such files over (D231 §5).
//     TestLoudness_AFileUnderAMissingRootIsNotSweptAndItsMeasurementWaitsForTheRoot
//     (internal/core/api).
//
//   - A node's word that FFmpeg stalled costs the job no attempt, and two in a row are the file's
//     failure. TestLoudness_AStallIsTriedAgainAndTwoInARowAreTheFilesFailure (internal/core/api).
//
//   - A measurement whose job fails is kept as the file's own failure, by payload hash as FFmpeg's
//     is (D231 §4, §10), so the sweep does not ask again.
//     TestLoudness_AMeasurementThatGaveUpIsKeptAsTheFilesOwnFailureAndNotSweptAgain
//     (internal/core/api).
//
//   - Imports nothing from internal/node, never os/exec, no database/sql: the depguard rules
//     core-boundary and sql-only-in-store, TestArch_CoreNeverImportsNode and
//     TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - D226: loudness is measured by the node and levelled by the player; the gain's rule.
//   - D231 §3–§5: derived work, failures as facts, parking.
//   - D234 §7, D238 §2: work given to a node, and results known by what they are.
//
// # Where to change
//
//   - loudness.go: the levels a file's measurements give, the sweep for files to measure, the
//     measurement job, its node's answer and how it is kept.
package loudness

// SPDX-License-Identifier: AGPL-3.0-or-later

// Package speedtrial runs a node's start-up speed trials (design/plans/start-quality-by-node-speed.md
// §1): how fast this node, with its FFmpeg and its chosen encoder, makes the top and the lowest
// quality of each kind of film. Each kind's clip, a generated picture with noise at a usual film's
// rate, is made once by the node's own FFmpeg and kept in its cache; each trial makes the clip into a
// quality with exactly the arguments a run of chunks uses (chunks.TrialArguments), written to nothing,
// decoded on the card first where a run would be and on the CPU where that fails. The trials run
// after hello, in the background, in the idle scheduling class, one at a time, out of every run of
// chunks' way. Each one ended is kept in the node's persistent directory under the key of what makes a
// film's speed (Key), and the node's status carries them all once all have ended.
//
// # Invariants
//
//   - Each kind is made into a clip once and tried once at its top quality and once at the lowest,
//     with a run of chunks' arguments, in the idle class, its speed the whole run's by the clock; the
//     results are published only when all have ended, and logged one line a trial and one at the end.
//     TestRun_TriesEachKindOnceAtItsTopAndLowestQuality.
//   - A kind whose encoder this FFmpeg lacks is not tried, and says so; it is no failure.
//     TestRun_AKindWithoutItsEncoderIsNotTried.
//   - A trial decoding on the card that fails as FFmpeg's own failure is made again on the CPU and
//     reports the card's failure in FFmpeg's words. TestRun_ACardsDecodeFailureFallsToTheCPUAndIsReported.
//   - A restart under the same key runs nothing and publishes what was kept; a changed key runs every
//     trial again. TestRun_KeptResultsRunNothingUntilTheKeyChanges.
//   - No trial goes while a run of chunks does, nor until quietAfterRun after it; a run starting stops
//     the trial going at once, and it is made again after. TestRun_NoTrialGoesWhileARunOfChunksDoes.
//   - A node that ends stops the trial going and waits until it has ended, so none writes on after
//     the node. TestStop_EndsTheTrialGoingAndWaitsForIt.
//   - The real FFmpeg makes a clip and measures it. TestRun_TheRealFFmpegMakesAClipAndMeasuresIt
//     (integration tier).
//
// # Read before changing
//
//   - docs/contributors/design/plans/start-quality-by-node-speed.md §1: what is tried, and why.
//   - D568: a run decoding on the card that fails is made again on the CPU.
//
// # Where to change
//
//   - kinds.go: the kinds and their clips; raising trialRecipe (kept.go) runs every trial again.
//     trials.go: the trials. yield.go: keeping out of a run of chunks' way. kept.go: the kept results.
package speedtrial

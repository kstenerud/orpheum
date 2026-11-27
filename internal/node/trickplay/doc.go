// SPDX-License-Identifier: AGPL-3.0-or-later

// Package trickplay makes a video's trickplay pictures (trickplay.md §3): a picture every interval
// of its time, the keyframe at or before each mark, decoding keyframes alone, scaled to the version's
// width and laid in sheets of columns × rows, each sheet a WebP. It is one FFmpeg run per video, in
// the idle scheduling class (execx.WatchedRun.IsIdle), whose sheets it gives one by one as FFmpeg
// finishes them; a run stopped partway resumes at the first sheet not made, sought to without
// decoding there, and gives none made already.
//
// # Invariants
//
//   - Each picture is the keyframe at or before its mark, within one keyframe gap of it, on a
//     generated fixture whose frames show their time; the last sheet is short.
//     TestMake_EachPictureIsTheKeyframeAtOrBeforeItsMark (integration tier).
//   - A resume makes each sheet not made as a run from the start makes it, and gives none made
//     again. TestMake_AResumeMakesEachSheetNotMadeAsARunFromTheStartDoesAndNoneMadeAgain
//     (integration tier), TestMake_GivesEachSheetAsItIsCutFromTheOutputAndNoneAlreadyMade.
//   - The run is idle; one from the start seeks nowhere.
//     TestMake_ARunFromTheStartSeeksNowhereAndIsIdle.
//   - A sheet refused, output that is not a WebP and output that ends partway through one end the
//     run with that error, never leaving FFmpeg blocked on its pipe.
//     TestMake_ASheetRefusedOrOutputThatIsNotWholeSheetsEndsTheRun.
//   - Its arguments change only with nodeproto.TrickplayRecipe (D536).
//     TestTrickplayRecipe_IsRaisedWithAnyChangeToItsArguments.
//
// # Read before changing
//
//   - docs/contributors/design/plans/trickplay.md §1, §3, §7: the sizes, the node's part, the tests.
//   - D536: a recipe for each thing the node makes.
//
// # Where to change
//
//   - make.go: the run and its arguments. sheets.go: cutting FFmpeg's output into its sheets.
package trickplay

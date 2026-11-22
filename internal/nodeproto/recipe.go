// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// The recipe of each thing the node makes of a file and the core keeps: the version of how it is
// made, raised with any change to what the node makes of the same file, so what an older recipe made
// is never served as this one's (D536). Each is named in the key of what it made (the core's
// chunks.StoreKey, artwork's frame still's ref, the loudness table's recipe), so a raise makes it
// again, once, where it is next wanted. A change to the FFmpeg arguments the node builds for a fixed
// set of commands fails until the thing's recipe is raised and its arguments recorded: the node's
// TestChunkRecipes_AreRaisedWithAnyChangeToTheirArguments (internal/node/chunks),
// TestFrameStillRecipe_IsRaisedWithAnyChangeToItsArguments (internal/node/artwork) and
// TestLoudnessRecipe_IsRaisedWithAnyChangeToItsArguments (internal/node/readthrough). A change to
// what the node does with FFmpeg's output is the review's. The wire's shape counts every recipe, so
// a core and its nodes always make at one recipe.
//
// 1 is each recipe as it was when recipes were first counted, which adds nothing to a key: the deploy
// that brought them in made nothing again (D536).
const (
	// PictureRecipe is the copied picture's.
	PictureRecipe = 1
	// QualityRecipe is an encoded quality's, on every encoder, counted from the core's encodeRecipe
	// "sourceTimeBase" (D392 §3).
	QualityRecipe = 1
	// SoundRecipe is a sound's, a track's mix encoded as stereo AAC, its gaps filled (D533 §1).
	SoundRecipe = 1
	// SoundCopiedRecipe is a track's sound copied as it is (D368 §1).
	SoundCopiedRecipe = 1
	// LoudnessRecipe is a loudness measurement's (D226 §1), held in the loudness table's key since
	// migration step 0100, so a measurement of another recipe is none of this one's (D536 §3):
	// TestLoudnessRecipe_AMeasurementOfAnotherRecipeIsNoneAndTheFileIsReadAgain
	// (internal/core/readthrough).
	LoudnessRecipe = 1
	// FrameStillRecipe is a frame still's (D362 §1).
	FrameStillRecipe = 1
)

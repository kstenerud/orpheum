// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// argumentsByRecipe is, for each kind of stream the node makes, a hash of the FFmpeg arguments it
// builds for recipeCommands at each version of its recipe (nodeproto.PictureRecipe and the others,
// D536). A version is added here with each raise of its recipe, and never changed after.
var argumentsByRecipe = map[nodeproto.ChunkStreamKind]map[int]string{
	// 1 is each recipe as it was when recipes were first counted (D536).
	nodeproto.StreamPicture:     {1: "24105f918eb0face5621f7a11e395f10cfdc6ec96f3505cc4c6d30dd53b98c42"},
	nodeproto.StreamQuality:     {1: "3dae99adffc429ad6e1b43218dcebdba9872491d21de6284ce4b663e505e9a72"},
	nodeproto.StreamSound:       {1: "169f1b599b8d85925f4efdeab2496fc4123af216b1e3e99f5785b94e6aee5615"},
	nodeproto.StreamSoundCopied: {1: "19ac78897a3cc6de988e058419c40f131722ee1acf5973b4b1a69548bb2feac4"},
}

// recipeOf is the recipe of a kind of stream, and its constant's name.
func recipeOf(kind nodeproto.ChunkStreamKind) (int, string) {
	switch kind {
	case nodeproto.StreamPicture:
		return nodeproto.PictureRecipe, "PictureRecipe"
	case nodeproto.StreamQuality:
		return nodeproto.QualityRecipe, "QualityRecipe"
	case nodeproto.StreamSound:
		return nodeproto.SoundRecipe, "SoundRecipe"
	case nodeproto.StreamSoundCopied:
		return nodeproto.SoundCopiedRecipe, "SoundCopiedRecipe"
	}
	return 0, string(kind)
}

// recipeCommands are the runs whose arguments stand for each kind's recipe: of the film's first
// chunks and later ones, led in and not, the picture H.264 and HEVC, re-timed and not, a quality on
// every encoder, a sound of two and of six channels, boosted, carrying a gain, read late and
// interleaved, and a track copied. Add a run here when a new kind of command changes what is made;
// the hashes change with it, so its recipe is raised.
func recipeCommands() map[nodeproto.ChunkStreamKind][]nodeproto.MakeChunks {
	later := func(stream nodeproto.ChunkStream) nodeproto.MakeChunks {
		command := run(stream)
		command.LeadInMs, command.IsLedIn = 88_839, true
		return command
	}
	first := func(stream nodeproto.ChunkStream) nodeproto.MakeChunks {
		command := run(stream)
		command.FirstChunk, command.BoundsMs = 0, []int64{21, 3_236, 6_400}
		return command
	}
	retimed := func(command nodeproto.MakeChunks) nodeproto.MakeChunks {
		command.VideoCodec, command.NeedsPictureRetiming = "hevc", true
		return command
	}
	picture := nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}
	quality := nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480}
	stereo := nodeproto.SoundTrack{AudioTrackIndex: 0, ChannelCount: 2}
	surround := nodeproto.SoundTrack{AudioTrackIndex: 1, ChannelCount: 6, CarriedGainDB: -3.5}
	sound := func(track nodeproto.SoundTrack, isBoosted bool) nodeproto.ChunkStream {
		return nodeproto.ChunkStream{Kind: nodeproto.StreamSound, Sound: &track, IsBoosted: isBoosted, SoundKbps: 96}
	}
	lateAndInterleaved := func(command nodeproto.MakeChunks) nodeproto.MakeChunks {
		command.SoundFirstPacketStretchMs, command.IsSoundInterleaved = 1_200, true
		return command
	}
	copied := nodeproto.ChunkStream{Kind: nodeproto.StreamSoundCopied, Sound: &surround}
	return map[nodeproto.ChunkStreamKind][]nodeproto.MakeChunks{
		nodeproto.StreamPicture: {
			first(picture), run(picture), later(picture), retimed(run(picture)), retimed(later(picture)),
		},
		nodeproto.StreamQuality: {first(quality), run(quality), later(quality), retimed(run(quality))},
		nodeproto.StreamSound: {
			first(sound(stereo, false)), run(sound(stereo, false)), later(sound(stereo, true)),
			run(sound(surround, false)), later(sound(surround, true)),
			lateAndInterleaved(later(sound(surround, false))), lateAndInterleaved(first(sound(stereo, false))),
		},
		nodeproto.StreamSoundCopied: {
			first(copied), run(copied), later(copied), lateAndInterleaved(later(copied)),
			lateAndInterleaved(first(copied)),
		},
	}
}

// recipeArguments is a hash of the arguments the node builds for each of a kind's recipeCommands,
// an encoded quality's on every encoder.
func recipeArguments(commands []nodeproto.MakeChunks) string {
	film := execx.Film{Path: "/films/a.mkv"}
	var lines []string
	for _, command := range commands {
		encoders := []encode.Encoder{encode.EncoderX264}
		if command.Stream.Kind == nodeproto.StreamQuality {
			encoders = append(encoders, encode.EncoderQuickSync, encode.EncoderNVENC, encode.EncoderVAAPI)
		}
		for _, encoder := range encoders {
			arguments := Arguments(film, "/prepared/7/chunks-x/k/raw-1", command, encoder)
			lines = append(lines, strings.Join(arguments, "\x00"))
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// TestChunkRecipes_AreRaisedWithAnyChangeToTheirArguments holds that the FFmpeg arguments the node
// builds for each kind of stream are the ones its recipe was raised for (D536): a change to how the
// copied picture, a quality, a sound or a copied sound is made, without raising its recipe, which
// would have the core serve and continue a store the old arguments made as the new's, or a raise
// without its arguments recorded, fails. A change to what the maker does with FFmpeg's raw files is
// the review's.
func TestChunkRecipes_AreRaisedWithAnyChangeToTheirArguments(t *testing.T) {
	t.Parallel()
	for kind, commands := range recipeCommands() {
		recipe, name := recipeOf(kind)
		got := recipeArguments(commands)
		if want, recorded := argumentsByRecipe[kind][recipe]; !recorded || want != got {
			t.Errorf("%s's arguments are %s; %s %d records %q. Changing how it is made raises nodeproto.%s, "+
				"and the new version's arguments are recorded in argumentsByRecipe", kind, got, name, recipe, want,
				name)
		}
	}
}

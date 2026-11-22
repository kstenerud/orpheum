// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// loudnessArgumentsByRecipe is a hash of the FFmpeg arguments the node builds to measure a fixed set
// of tracks' loudness at each nodeproto.LoudnessRecipe (D536). A version is added here with each raise
// of it, and never changed after.
var loudnessArgumentsByRecipe = map[int]string{
	// 1 is the recipe as it was when recipes were first counted (D536).
	1: "17f0f1de913fe592b274c86c551c05eac22c089fb891f1e842bcd09a81ee2b93",
}

// TestLoudnessRecipe_IsRaisedWithAnyChangeToItsArguments holds that the FFmpeg arguments that measure
// a film's loudness are the ones nodeproto.LoudnessRecipe was raised for (D536): a change to how a
// track is measured, its stereo mix or its filter, without raising it, which would level every film
// measured before by another measure, or a raise without its arguments recorded, fails. A change to
// how the summary is read from FFmpeg's log is the review's.
func TestLoudnessRecipe_IsRaisedWithAnyChangeToItsArguments(t *testing.T) {
	t.Parallel()
	var lines []string
	for _, tracks := range [][]nodeproto.LoudnessTrack{
		{{StreamIndex: 1, ChannelCount: 2}},
		{{StreamIndex: 1, ChannelCount: 1}},
		{{StreamIndex: 1, ChannelCount: 6}, {StreamIndex: 2, ChannelCount: 2}, {StreamIndex: 4, ChannelCount: 8}},
	} {
		lines = append(lines, strings.Join(loudnessOutput(tracks), "\x00"))
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	got := hex.EncodeToString(sum[:])
	if want, recorded := loudnessArgumentsByRecipe[nodeproto.LoudnessRecipe]; !recorded || want != got {
		t.Errorf("a loudness measurement's arguments are %s; LoudnessRecipe %d records %q. Changing how "+
			"loudness is measured raises nodeproto.LoudnessRecipe, and the new version's arguments are "+
			"recorded in loudnessArgumentsByRecipe", got, nodeproto.LoudnessRecipe, want)
	}
}

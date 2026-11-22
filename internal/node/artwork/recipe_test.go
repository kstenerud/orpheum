// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// stillArgumentsByRecipe is a hash of the FFmpeg arguments the node builds for a frame still of a
// fixed set of videos at each nodeproto.FrameStillRecipe (D536). A version is added here with each
// raise of it, and never changed after.
var stillArgumentsByRecipe = map[int]string{
	// 1 is the recipe as it was when recipes were first counted (D536).
	1: "fec27885512c37955b638910df0db06ac221d2ea4b87445bff43dce5130be208",
}

// TestFrameStillRecipe_IsRaisedWithAnyChangeToItsArguments holds that the FFmpeg arguments of a frame
// still are the ones nodeproto.FrameStillRecipe was raised for (D536): a change to how a still is
// chosen or written without raising it, which would leave every still taken before shown as this
// recipe's, or a raise without its arguments recorded, fails. A change to how Derive re-encodes the
// still into its sizes is the review's.
func TestFrameStillRecipe_IsRaisedWithAnyChangeToItsArguments(t *testing.T) {
	t.Parallel()
	var lines []string
	for _, still := range []struct {
		streamIndex int
		from        time.Duration
	}{{0, 0}, {2, 61 * time.Second}, {1, 90*time.Minute + 1500*time.Millisecond}} {
		arguments := stillArguments(execx.Film{Path: "/films/a.mkv"}.Input(), still.streamIndex, still.from)
		lines = append(lines, strings.Join(arguments, "\x00"))
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	got := hex.EncodeToString(sum[:])
	if want, recorded := stillArgumentsByRecipe[nodeproto.FrameStillRecipe]; !recorded || want != got {
		t.Errorf("a frame still's arguments are %s; FrameStillRecipe %d records %q. Changing how a still is "+
			"taken raises nodeproto.FrameStillRecipe, and the new version's arguments are recorded in "+
			"stillArgumentsByRecipe", got, nodeproto.FrameStillRecipe, want)
	}
}

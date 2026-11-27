// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

// A start on a ladder tells its player that its node's speed held it only when that bound decided
// it, so the player jumps once to its link's quality (start-quality-by-node-speed.md §4, S4); a
// start the link, the upload or nothing bounded says not.
func TestStartedOutput_SaysTheStartWasHeldOnlyWhenItsNodesSpeedDecidedIt(t *testing.T) {
	t.Parallel()
	for bound, want := range map[chunks.StartBound]bool{
		chunks.StartBoundNodeSpeed:   true,
		chunks.StartBoundStartAtMost: false,
		chunks.StartBoundLink:        false,
		chunks.StartBoundTop:         false,
	} {
		output := startedOutput(playback.Started{Token: "t", OnLadder: &chunks.Offer{StartBound: bound}})
		if got := output.Body.OnLadder.IsStartHeldByNodeSpeed; got != want {
			t.Errorf("a start bounded by %q says held by its node's speed %v; want %v", bound, got, want)
		}
	}
}

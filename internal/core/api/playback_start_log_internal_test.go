// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

// A start on a ladder's log line says whether the speed its node was expected to make it at came
// from its trials or its runs (start-quality-by-node-speed.md §3).
func TestLadderStart_SaysWhereTheExpectedSpeedCameFrom(t *testing.T) {
	t.Parallel()
	logged := ladderStart(playback.Started{OnLadder: &chunks.Offer{
		StreamName: "q3000k720p", ExpectedSpeedPercent: 400, ExpectedSpeedFrom: chunks.SpeedFromRuns,
	}})
	at := slices.Index(logged, any("ladderExpectedSpeedFrom"))
	if at < 0 || at+1 >= len(logged) || logged[at+1] != "runs" {
		t.Errorf("the start's log line is %v; want ladderExpectedSpeedFrom runs", logged)
	}
}

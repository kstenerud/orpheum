// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "github.com/kstenerud/orpheum/internal/core/playback"

// ladderStart is where on its film's ladder a playback starts, for its log line: the quality's stream
// and the sound's it starts with, such as picture and sound0-own (D368 §5), what bounded it, and the
// speed its node is expected to make it at (start-quality-by-node-speed.md §2) and whether that came
// from its trials or its runs (§3); nothing off it.
func ladderStart(started playback.Started) []any {
	if started.OnLadder == nil {
		return nil
	}
	on := started.OnLadder
	return []any{
		"ladderQuality", on.StreamName, "ladderSound", on.SoundStreamName,
		"ladderStartBound", string(on.StartBound), "ladderExpectedSpeedPercent", on.ExpectedSpeedPercent,
		"ladderExpectedSpeedFrom", string(on.ExpectedSpeedFrom),
	}
}

// startsOnLadderKbps is the rate of the picture on its film's ladder a playback starts at, 0 when it
// plays as its decision says (D260).
func startsOnLadderKbps(started playback.Started) int {
	if started.OnLadder == nil {
		return 0
	}
	return started.OnLadder.Quality.Kbps
}

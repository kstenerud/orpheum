// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
)

// cpuCeilingFor is how long a playback moved off a quality its node could not make fast enough
// keeps to the quality it was moved to: long enough for the node's other runs to settle, after which
// it may climb again, and the node's watch moves it again if it is still too slow (D286 §4).
const cpuCeilingFor = 2 * time.Minute

// MoveOffStream moves every playback of fileID whose player wants streamName, a quality its node
// cannot make as fast as it plays, onto the quality below it on its ladder, for cpuCeilingFor (D309
// §6, D286 §4): a run of a lower quality costs the node less, where the run evicted stopped its
// players. It says whether there were any and every one moved; when one is already at the lowest,
// none is, and the run is evicted, its players told why they stop.
func (service *Service) MoveOffStream(fileID int64, streamName string) bool {
	now := service.clock.Now()
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	moves := map[*tokenPlayback]int64{}
	for _, playback := range service.tokens.playbackByToken {
		if playback.fileID != fileID || playback.interest.picture != streamName || !playback.isHeardWithin(now) {
			continue
		}
		kbps, isFound := rungBelow(playback.ladder, streamName)
		if !isFound {
			return false
		}
		moves[playback] = kbps
	}
	for playback, kbps := range moves {
		playback.cpuCeilingKbps, playback.cpuCeilingUntil = kbps, now.Add(cpuCeilingFor)
	}
	return len(moves) > 0
}

// rungBelow is the rate of the rung of ladder below streamName's, and whether there is one.
func rungBelow(ladder []chunks.Rung, streamName string) (int64, bool) {
	at := slices.IndexFunc(ladder, func(rung chunks.Rung) bool { return rung.StreamName == streamName })
	if at < 1 {
		return 0, false
	}
	return ladder[at-1].AverageKbps, true
}

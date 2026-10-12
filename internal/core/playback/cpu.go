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

// MoveOffStream moves every playback of fileID whose player loads streamName, a quality its node
// cannot make as fast as it plays, onto the highest lower quality another playback of the film
// loads, for cpuCeilingFor: one encode then feeds both, rather than the run being evicted and its
// players stopped (D286 §4). It says whether there were any and every one moved; when not, none is.
func (service *Service) MoveOffStream(fileID int64, streamName string) bool {
	now := service.clock.Now()
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	loading := map[*tokenPlayback]bool{}
	others := map[string]bool{}
	for _, playback := range service.tokens.playbackByToken {
		if playback.fileID != fileID || playback.picturesNeed.within(now) == 0 {
			continue
		}
		if playback.pictureStream == streamName {
			loading[playback] = true
		} else {
			others[playback.pictureStream] = true
		}
	}
	moves := map[*tokenPlayback]int64{}
	for playback := range loading {
		kbps, isFound := madeBelow(playback.ladder, streamName, others)
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

// madeBelow is the rate of the highest rung of ladder below streamName's whose stream is among made,
// and whether there is one.
func madeBelow(ladder []chunks.Rung, streamName string, made map[string]bool) (int64, bool) {
	at := slices.IndexFunc(ladder, func(rung chunks.Rung) bool { return rung.StreamName == streamName })
	for index := at - 1; index >= 0; index-- {
		if made[ladder[index].StreamName] {
			return ladder[index].AverageKbps, true
		}
	}
	return 0, false
}

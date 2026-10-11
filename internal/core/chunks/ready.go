// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// readyAheadMs is how far past where a player is a lower quality must be made before it is ready to
// move to: as far as a stalled player loads before it plays again (D247 §5, D253 §1).
const readyAheadMs = 10_000

// IsReadyAt is whether a stall's offer is ready for a player at positionMs (D253 §1): the offered
// quality's chunks, and the starting sound's, are made from there for readyAheadMs, or to the film's
// end. The core knows a chunk made from its node's progress or from serving it; after a restart it
// knows none until then, and the offer is not ready.
func (service *Service) IsReadyAt(ctx context.Context, playback Playback, offer Offer, positionMs int64) (bool, error) {
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return false, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return false, err
	}
	streams := []Stream{{Kind: nodeproto.StreamQuality, Quality: offer.Quality}}
	if playback.HasStartingAudio {
		streams = append(streams, startingSound(playback, offered.soundKbps, offered.isUnlevelled))
	}
	from, through := film.grid.chunkAt(positionMs), film.grid.chunkAt(positionMs+readyAheadMs)
	for _, stream := range streams {
		if !service.runs.isMadeFromThrough(StoreDir(playback.FileID, stream, film.grid.ID()), from, through) {
			return false, nil
		}
	}
	return true, nil
}

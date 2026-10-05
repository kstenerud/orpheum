// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/decide"
)

// ChunksOf is the playback a media token refers to as its film's chunks are served to it (D247
// §10): its file, whether its player decodes the film's own picture, the audio track it started
// with and where its player last said it was. The token is the holder's, as for its other bytes:
// a Watch Together viewer's for the film the session plays.
func (service *Service) ChunksOf(ctx context.Context, token string, holder Holder) (chunks.Playback, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return chunks.Playback{}, err
	}
	playback, found := service.tokens.use(token, holder, service.clock.Now(), idle)
	if !found {
		return chunks.Playback{}, ErrNoToken
	}
	chunked := chunks.Playback{
		FileID: playback.fileID, PositionMs: playback.positionMs,
		// A playback whose picture its decision re-encodes is one whose player cannot decode it.
		IsSourcePictureDecodable: playback.decision.Profile != decide.H264At8M,
	}
	if playback.startingAudio != nil {
		chunked.HasStartingAudio, chunked.StartingAudioTrackIndex = true, playback.startingAudio.AudioTrackIndex
	}
	return chunked, nil
}

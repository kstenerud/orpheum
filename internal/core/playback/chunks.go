// SPDX-License-Identifier: AGPL-3.0-or-later

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
	return chunksPlaybackOf(playback), nil
}

// Keep says what a token's playback keeps, to its end, of the first master playlist of chunks it
// was offered: its film cut on time, its file having no index of its keyframes, though a
// read-through finds them meanwhile (D269 §3); and its sounds unlevelled, though the film's loudness
// is measured meanwhile.
func (service *Service) Keep(token string, holder Holder, kept chunks.Kept) {
	service.tokens.keep(token, holder, kept)
}

// chunksPlaybackOf is a playback as the chunks it is offered see it.
func chunksPlaybackOf(playback tokenPlayback) chunks.Playback {
	chunked := chunks.Playback{
		FileID: playback.fileID, PositionMs: playback.positionMs, StartedAt: playback.startedAt,
		// A playback whose picture its decision re-encodes is one whose player cannot decode it, as
		// is one whose player gave the film's own picture up on an earlier playback (D266).
		IsSourcePictureDecodable: playback.decision.Profile != decide.H264 && !playback.isOwnPictureUnplayable,
		IsOnTime:                 playback.isOnTime,
		IsSoundUnlevelled:        playback.isSoundUnlevelled,
	}
	if playback.startingAudio != nil {
		chunked.HasStartingAudio, chunked.StartingAudioTrackIndex = true, playback.startingAudio.AudioTrackIndex
	}
	return chunked
}

// Fetched records a file of its film's chunks a token's playback asked for: its stream's rate, what
// the playback needs of the server's upload (D286 §5), and of a sound, the sound it plays, which a
// jump's picture is made alongside (SoundOf).
func (service *Service) Fetched(token string, file chunks.StoreFile) {
	now := service.clock.Now()
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	known, found := service.tokens.playbackByToken[token]
	if !found {
		return
	}
	need := streamNeed{kbps: file.AverageKbps(), at: now}
	if file.IsSound() {
		known.sound, known.hasSound, known.soundsNeed = file, true, need
	} else {
		known.picturesNeed = need
	}
}

// SoundOf is the sound a token's playback plays, the last file of a sound its player asked for, and
// whether it asked for one: after a jump, hls.js asks for the sound only once the picture's first
// chunk has come, so the sound was made after the picture, not beside it, and a jump on an encoded
// quality waited 12.5 s, 5 of them for its sound (the owner's test, 2026-10-12).
func (service *Service) SoundOf(token string) (chunks.StoreFile, bool) {
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	known, found := service.tokens.playbackByToken[token]
	if !found || !known.hasSound {
		return chunks.StoreFile{}, false
	}
	return known.sound, true
}

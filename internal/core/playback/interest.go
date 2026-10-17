// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
)

// interestLapsesAfter is how long a playback's interest in its film's chunks lasts with its player
// silent: no word, no chunk asked for, no report (D309 §3). A playing player asks every few seconds
// and reports every ten; a paused one says so and is silent, and its minute ahead is made well within
// this, so a closed tab, a crash or a lost link hold an encoder at most this long.
const interestLapsesAfter = 2 * time.Minute

// playerInterest is what a playback's player wants of its film's chunks (D309 §1): one picture and
// its sound, from a position, paused or not.
type playerInterest struct {
	picture, sound string
	positionMs     int64
	isPaused       bool
	// hasLeft is whether its player left (Leave): it wants nothing, its stall's offer neither, until
	// it asks again, as a player page that takes over its film page's token does.
	hasLeft bool
}

// Interest is what a player says it wants of its film's chunks (SayInterest): a picture's and a
// sound's streams, by the names its master playlist gives them, either empty to keep the one it
// wants; where it plays from; and whether it is paused, wanting only the minute after it.
type Interest struct {
	PictureStream, SoundStream string
	PositionMs                 int64
	IsPaused                   bool
}

// SayInterest takes a player's word on what it wants of its film's chunks, which replaces what it
// wanted (D309 §1): said as it starts, switches quality, seeks, pauses and plays again. The film's
// runs no player wants then stop. A token not the holder's, or gone, is ErrNoToken.
func (service *Service) SayInterest(ctx context.Context, token string, holder Holder, said Interest) error {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return err
	}
	playback, found := service.tokens.use(token, holder, service.clock.Now(), idle)
	if !found {
		return ErrNoToken
	}
	service.tokens.mutex.Lock()
	if known, isKnown := service.tokens.playbackByToken[token]; isKnown {
		interest := &known.interest
		if said.PictureStream != "" {
			interest.picture = said.PictureStream
		}
		if said.SoundStream != "" {
			interest.sound = said.SoundStream
		}
		interest.positionMs, interest.isPaused, interest.hasLeft = said.PositionMs, said.IsPaused, false
	}
	service.tokens.mutex.Unlock()
	return service.stopUnwanted(ctx, playback.fileID)
}

// stopUnwanted stops the film's runs no player wants, where the playbacks have chunks.
func (service *Service) stopUnwanted(ctx context.Context, fileID int64) error {
	if service.chunks == nil {
		return nil
	}
	return service.chunks.StopUnwanted(ctx, fileID)
}

// InterestsIn is every interest the playbacks of fileID hold now (chunks.Interests): each one's
// picture and sound, and the lower quality and sound a stall made ready for it (D247 §2), until its
// player falls silent (interestLapsesAfter).
func (service *Service) InterestsIn(fileID int64) []chunks.Interest {
	now := service.clock.Now()
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	var interests []chunks.Interest
	for _, playback := range service.tokens.playbackByToken {
		if playback.fileID != fileID || !playback.isHeardWithin(now) || playback.interest.hasLeft {
			continue
		}
		interest := playback.interest
		for _, stream := range []string{interest.picture, interest.sound} {
			if stream != "" {
				interests = append(interests, chunks.Interest{
					StreamName: stream, PositionMs: interest.positionMs, IsPaused: interest.isPaused,
				})
			}
		}
		if offer := playback.offer; offer != nil {
			for _, stream := range []string{offer.StreamName, offer.SoundStreamName} {
				if stream != "" {
					interests = append(interests, chunks.Interest{StreamName: stream, PositionMs: offer.FromMs})
				}
			}
		}
	}
	return interests
}

// wantFetched reads a playback's interest off a file of its film's chunks its player asked for (D309
// §1): its picture or its sound is the file's stream, and the file's chunk, starting at startMs,
// unless its player said it is paused, is where it now plays from. An init, isChunk false, says
// nothing of where the player is: its run is made from where the playback last reported, the film's
// start for a guest that has reported nothing (the owner's VQ38 run, 2026-10-12). Locked by its caller.
func (playback *tokenPlayback) wantFetched(isSound bool, stream string, startMs int64, isChunk bool) {
	interest := &playback.interest
	interest.hasLeft = false
	if isSound {
		interest.sound = stream
	} else {
		interest.picture = stream
	}
	if isChunk && !interest.isPaused {
		interest.positionMs = startMs
	}
}

// keepStart keeps how a playback starts, onLadder nil for one not on its film's ladder: whether it is
// on it, what it wants, the picture and sound it starts on from where they are made (D309 §1, D260),
// and what it needs of the server's upload (keepStartsNeed).
func (service *Service) keepStart(playback *tokenPlayback, onLadder *chunks.Offer) {
	playback.isOnLadder = onLadder != nil
	if onLadder != nil {
		playback.interest = playerInterest{
			picture: onLadder.StreamName, sound: onLadder.SoundStreamName, positionMs: onLadder.FromMs,
		}
	}
	service.keepStartsNeed(playback, onLadder)
}

// isHeardWithin is whether the playback's player was heard from within interestLapsesAfter of now:
// a word, a chunk asked for, a report (D309 §3).
func (playback *tokenPlayback) isHeardWithin(now time.Time) bool {
	return now.Sub(playback.lastUsedAt) <= interestLapsesAfter
}

// dropInterest forgets a token's interest: its player left (D309 §1).
func (all *tokens) dropInterest(token string) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if known, found := all.playbackByToken[token]; found {
		known.interest = playerInterest{hasLeft: true}
	}
}

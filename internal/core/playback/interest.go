// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Disposition is where a playback's player stands with its film (D309 §7): playing it, paused,
// gone from its page, or gone to another film.
type Disposition string

// The dispositions.
const (
	// Playing is a player playing, or about to: the zero value, as it starts.
	Playing Disposition = ""
	// Paused is a player paused: likely to play again. Its runs finish chunkKeptAheadSeconds ahead.
	Paused Disposition = "paused"
	// Left is a player that left its page, playing nothing else: maybe back. Its runs finish as a
	// paused player's do.
	Left Disposition = "left"
	// Replaced is a player that went on to play another film: not back. Its runs stop at once.
	Replaced Disposition = "replaced"
)

// playerInterest is what a playback's player wants of its film's chunks (D309 §1): one picture and
// its sound, by stream and by store, each from its own position (D355), and where it stands.
type playerInterest struct {
	picture, sound       string
	pictureDir, soundDir string
	// picturePositionMs and soundPositionMs are where the picture and the sound are wanted from: each
	// moved by its own stream's asks, both by the player's word, a seek's picture and the start.
	// hls.js loads the picture up to its buffer ahead of a late sound, and one position, moved by
	// both, jumped back and forth between them.
	picturePositionMs, soundPositionMs int64
	disposition                        Disposition
}

// Interest is what a player says it wants of its film's chunks (SayInterest): a picture's and a
// sound's streams, by the names its master playlist gives them, either empty to keep the one it
// wants; where it plays from; and whether it is paused.
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
		known.interest.take(said)
	}
	service.tokens.mutex.Unlock()
	return service.stopUnwanted(ctx, playback.fileID)
}

// take replaces what a player wanted with what it said: a stream it names that is not the one it
// wanted is wanted from a store not yet known, until it asks for a file of it. Its word, said as it
// starts, seeks, pauses and plays again, moves both its streams to where it plays (D355 §2).
func (interest *playerInterest) take(said Interest) {
	if said.PictureStream != "" && said.PictureStream != interest.picture {
		interest.picture, interest.pictureDir = said.PictureStream, ""
	}
	if said.SoundStream != "" && said.SoundStream != interest.sound {
		interest.sound, interest.soundDir = said.SoundStream, ""
	}
	interest.moveBoth(said.PositionMs)
	interest.disposition = Playing
	if said.IsPaused {
		interest.disposition = Paused
	}
}

// moveBoth moves the picture's and the sound's positions together to positionMs (D355 §2).
func (interest *playerInterest) moveBoth(positionMs int64) {
	interest.picturePositionMs, interest.soundPositionMs = positionMs, positionMs
}

// stopUnwanted stops the film's runs no player wants, where the playbacks have chunks.
func (service *Service) stopUnwanted(ctx context.Context, fileID int64) error {
	if service.chunks == nil {
		return nil
	}
	return service.chunks.StopUnwanted(ctx, fileID)
}

// InterestsIn is every interest the playbacks of fileID hold now (chunks.Interests): each one's
// picture and sound, each from its own position (D355), stopped once its player paused or left,
// and the lower quality and sound a stall made ready for it (D247 §2); none once its player went to
// another film, or fell silent while playing for chunkInterestLapsesAfterSeconds (D309 §3).
func (service *Service) InterestsIn(fileID int64) []chunks.Interest {
	now := service.clock.Now()
	lapsesAfter := service.timingNow().interestLapsesAfter
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	var interests []chunks.Interest
	for _, playback := range service.tokens.playbackByToken {
		interest := playback.interest
		if !playback.wantsChunksOf(fileID, now, lapsesAfter) {
			continue
		}
		isStopped := interest.disposition != Playing
		for _, wanted := range []struct {
			stream     string
			positionMs int64
		}{{interest.picture, interest.picturePositionMs}, {interest.sound, interest.soundPositionMs}} {
			if wanted.stream != "" {
				interests = append(interests, chunks.Interest{
					StreamName: wanted.stream, PositionMs: wanted.positionMs, IsStopped: isStopped,
				})
			}
		}
		if offer := playback.offer; offer != nil && interest.disposition != Left {
			for _, stream := range []string{offer.StreamName, offer.SoundStreamName} {
				if stream != "" {
					interests = append(interests, chunks.Interest{StreamName: stream, PositionMs: offer.FromMs})
				}
			}
		}
	}
	return interests
}

// askedFile is a file of its film's chunks a player asked for, as its playback's interest reads it
// (wantFetched): its stream's name and store, where its chunk starts, and whether it is a sound's,
// a chunk rather than an init, and asked for loading where a seek took the player
// (chunks.StoreFile.AsSought).
type askedFile struct {
	stream, dir                string
	startMs                    int64
	isSound, isChunk, isSought bool
}

// askedFileOf is file as its playback's interest reads it.
func askedFileOf(file chunks.StoreFile) askedFile {
	startMs, isChunk := file.StartMs()
	return askedFile{
		stream: file.StreamName(), dir: file.StoreDir(), startMs: startMs,
		isSound: file.IsSound(), isChunk: isChunk, isSought: file.IsSought(),
	}
}

// wantFetched reads a playback's interest off a file of its film's chunks its player asked for (D309
// §1): its picture or its sound is the file's stream, and the file's chunk, unless its player said it
// is paused, is where that stream, and only that one, is now wanted from (D355 §1); a picture's chunk
// asked for where a seek took the player moves its sound there too, as a seek moves both (§2): hls.js
// asks for the seek's sound only once the picture's first chunk has come, and the sound's run is made
// alongside the picture from there (chunks.Service.WantAlongside). A chunk asked for where a seek took
// a paused player moves it as it would a playing one, and it stays paused: hls.js, sought onto a
// chunk's very start, steps back to the chunk before when what it loaded starts a moment after, and a
// Watch Together group waiting on the jump, its players paused on that start, wanted nothing behind
// it, planned no run for the chunk every player asked for, and never played (staging, 2026-10-30).
// Its buffer filled after the seek is not sought (the player's markSeeks), and moves nothing, as
// before. An init says nothing of where the
// player is: its run is made from where the playback last reported, the film's start for a guest
// that has reported nothing (the owner's VQ38 run, 2026-10-12). A player that asks after it left, as
// a player page that takes over its film page's token does, plays again. Locked by its caller.
func (playback *tokenPlayback) wantFetched(asked askedFile) {
	interest := &playback.interest
	if asked.isSound {
		interest.sound, interest.soundDir = asked.stream, asked.dir
	} else {
		interest.picture, interest.pictureDir = asked.stream, asked.dir
	}
	if interest.disposition == Paused && (!asked.isChunk || !asked.isSought) {
		return
	}
	if interest.disposition != Paused {
		interest.disposition = Playing
	}
	switch {
	case !asked.isChunk:
	case asked.isSound:
		interest.soundPositionMs = asked.startMs
	case asked.isSought:
		interest.moveBoth(asked.startMs)
	default:
		interest.picturePositionMs = asked.startMs
	}
}

// keepStart keeps how a playback starts, onLadder nil for one not on its film's ladder: whether it is
// on it, what it wants, the picture and sound it starts on from where they are made (D309 §1, D260),
// and what it needs of the server's upload (keepStartsNeed).
func (service *Service) keepStart(playback *tokenPlayback, onLadder *chunks.Offer) {
	playback.isOnLadder = onLadder != nil
	if onLadder != nil {
		playback.isOnTime = onLadder.IsOnTime
		playback.interest = playerInterest{
			picture: onLadder.StreamName, sound: onLadder.SoundStreamName,
			picturePositionMs: onLadder.FromMs, soundPositionMs: onLadder.FromMs,
		}
	}
	service.keepStartsNeed(playback, onLadder)
}

// goneToAnother marks every playback of player's other than fileID's gone to it, and stops their
// films' runs no one else wants (D309 §7): a player that starts a film is not coming back to them.
func (service *Service) goneToAnother(ctx context.Context, player string, fileID int64) error {
	var stopped error
	for _, replaced := range service.tokens.replaceOthers(player, fileID) {
		stopped = errors.Join(stopped, service.stopUnwanted(ctx, replaced))
	}
	return stopped
}

// isWatching is whether the playback's player is playing, heard within lapsesAfter, or paused: one
// whose quality a node cannot keep up with is moved, as its next request will be (D309 §6).
func (playback *tokenPlayback) isWatching(now time.Time, lapsesAfter time.Duration) bool {
	disposition := playback.interest.disposition
	return disposition == Paused || (disposition == Playing && now.Sub(playback.lastUsedAt) <= lapsesAfter)
}

// leave marks a token's player gone, Left or Replaced (D309 §7).
func (all *tokens) leave(token string, gone Disposition) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if known, found := all.playbackByToken[token]; found {
		known.interest.disposition = gone
	}
}

// replaceOthers marks every playback of player's other than fileID's gone to another film: it plays
// that one now, and is not coming back to them (D309 §7). It returns their films.
func (all *tokens) replaceOthers(player string, fileID int64) []int64 {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	var replaced []int64
	for _, known := range all.playbackByToken {
		isOthers := playerOf(known.deviceID, known.sessionKey) == player && known.fileID != fileID
		if isOthers && known.interest.disposition != Replaced {
			known.interest.disposition = Replaced
			replaced = append(replaced, known.fileID)
		}
	}
	return replaced
}

// StoreTiers is how strongly each chunk store a player asked for is kept, by its directory: the
// highest tier any playback gives it (D309 §8), read from the playbacks, which are the list of who
// wants what. A store none gives one is the first a node removes when it needs room.
func (service *Service) StoreTiers(ctx context.Context) (map[string]nodeproto.KeepTier, error) {
	keeping, err := service.keepingTimes(ctx)
	if err != nil {
		return nil, err
	}
	now := service.clock.Now()
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	tiers := map[string]nodeproto.KeepTier{}
	for _, playback := range service.tokens.playbackByToken {
		tier := keeping.tierOf(playback.interest.disposition, now.Sub(playback.lastUsedAt))
		if tier == nodeproto.KeepUnlikely {
			continue
		}
		for _, dir := range []string{playback.interest.pictureDir, playback.interest.soundDir} {
			if dir != "" && tier.IsAbove(tiers[dir]) {
				tiers[dir] = tier
			}
		}
	}
	return tiers, nil
}

// keeping is how long a silent player's stores stay at each tier (D309 §8): likely, paused or silent,
// for likelyFor; maybe to maybeFor; and maybe for leftFor once it left its page.
type keeping struct {
	likelyFor, leftFor, maybeFor time.Duration
	// lapsesAfter is how long a playing player's interest lasts with it silent (playbackTiming).
	lapsesAfter time.Duration
}

// keepingTimes are the settings' keeping.
func (service *Service) keepingTimes(ctx context.Context) (keeping, error) {
	var times [3]time.Duration
	for index, key := range []settings.Key{
		settings.ChunkKeptLikelySeconds, settings.ChunkKeptLeftSeconds, settings.ChunkKeptMaybeSeconds,
	} {
		var err error
		if times[index], err = service.settings.Duration(ctx, key); err != nil {
			return keeping{}, fmt.Errorf("read %s: %w", key, err)
		}
	}
	return keeping{
		likelyFor: times[0], leftFor: times[1], maybeFor: times[2],
		lapsesAfter: service.timingNow().interestLapsesAfter,
	}, nil
}

// tierOf is the tier a player's stores are kept at, by where it stands and how long since it was
// last heard (D309 §8): in use while playing and heard within interestLapsesAfter; likely, paused or
// silent, for likelyFor; maybe, after, to maybeFor, and for leftFor once it left its page; unlikely
// once it went to another film, and after all of them.
func (times keeping) tierOf(disposition Disposition, silentFor time.Duration) nodeproto.KeepTier {
	switch {
	case disposition == Replaced:
		return nodeproto.KeepUnlikely
	case disposition == Left:
		if silentFor < times.leftFor {
			return nodeproto.KeepMaybe
		}
		return nodeproto.KeepUnlikely
	case disposition == Playing && silentFor <= times.lapsesAfter:
		return nodeproto.KeepInUse
	case silentFor < times.likelyFor:
		return nodeproto.KeepLikely
	case silentFor < times.maybeFor:
		return nodeproto.KeepMaybe
	}
	return nodeproto.KeepUnlikely
}

// wantsChunksOf is whether the playback wants any of fileID's chunks at now: not gone to another
// film, nor silent while playing for lapsesAfter (D309 §3), nor playing the file as it is, having
// asked for none of its chunks, whose sound said, a track switched to in place, is the one a stall's
// offer starts (chunks' playingSoundOf).
func (playback *tokenPlayback) wantsChunksOf(fileID int64, now time.Time, lapsesAfter time.Duration) bool {
	disposition := playback.interest.disposition
	isLapsed := disposition == Playing && now.Sub(playback.lastUsedAt) > lapsesAfter
	return playback.fileID == fileID && disposition != Replaced && !isLapsed && !playback.playsTheFileAsItIs()
}

// playsTheFileAsItIs is whether the playback plays its file as it is: not on its film's ladder, offered
// no lower quality, and its player has asked for none of its chunks.
func (playback *tokenPlayback) playsTheFileAsItIs() bool {
	return !playback.isOnLadder && playback.offer == nil && playback.interest.pictureDir == "" &&
		playback.interest.soundDir == ""
}

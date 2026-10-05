// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// tokenBytes is a media token's length before encoding: long enough that one cannot be guessed,
// though a token grants nothing on its own (D217 §2).
const tokenBytes = 24

// Holder is who holds a media token: the person, the session the playback was started under, and
// the libraries the person reaches now (D217 §2, D243 §4). A Watch Together viewer holds one too:
// no person, their membership as the session, the host's libraries, and only the film the session
// plays now (D246 §3).
type Holder struct {
	UserID     int64
	SessionKey string
	Reach      auth.Reach
	// OnlyFileID, when not 0, is the one file the holder may play: a viewer's, the session's film.
	OnlyFileID int64
	// Viewer is a second holder the same request also is: a browser signed in as its person that
	// also views a Watch Together session, its two cookies side by side.
	Viewer *Holder
}

// HolderOf is the holder a signed-in principal is.
func HolderOf(principal auth.Principal) Holder {
	return Holder{UserID: principal.UserID, SessionKey: principal.SessionKey, Reach: principal.Reach}
}

// tokenPlayback is what a media token refers to: whose playback of which file, in which library,
// under which session.
type tokenPlayback struct {
	userID     int64
	sessionKey string
	fileID     int64
	// libraryID is the file's library: a holder who no longer reaches it no longer holds the
	// token, so a library taken back stops its playbacks at their next request (D243 §4).
	libraryID int64
	// playbackID names the playback to the admin's now playing, which never sees the token; deviceID
	// is the device it plays on, startedAt when it started, decision how it plays, and positionMs
	// where its last report said it was.
	playbackID int64
	deviceID   int64
	startedAt  time.Time
	decision   decide.Decision
	positionMs int64
	// stopMessage, once the admin stopped the playback, is what its player is told in the answer to
	// its next report, which ends it (D243 §9).
	stopMessage *string
	// profile is a prepared playback's rendition; empty for direct play.
	profile    nodeproto.PrepareProfile
	lastUsedAt time.Time
	// lastReportSeq is the number of the last play report taken for this playback: the player
	// numbers its reports from 1, and one sent again is taken once.
	lastReportSeq int64
	// used is what the playback started with, which its file keeps once a report says it is started.
	used usedTracks
	// startingAudio is the audio track the playback started with, nil for a file with no sound: its
	// chunks' master playlist starts with that track's standard mix (D247 §10).
	startingAudio *AudioTrack
	// stalls is how many times its player stalled; offer is the lower quality its first stall
	// started, and isOfferTold whether its player was told of it (D247 §2).
	stalls      int
	offer       *chunks.Offer
	isOfferTold bool
}

// usedTracks are the subtitle and audio tracks a playback started with; audio is nil for a file
// with no sound.
type usedTracks struct {
	subtitle preferences.Used
	audio    *preferences.Used
}

// tokens are the playbacks in progress, in memory: a restart ends them, and the client asks for
// a new one with POST /playback, as it would after the idle period.
type tokens struct {
	mutex           sync.Mutex
	playbackByToken map[string]*tokenPlayback // guarded by mutex
	// lastPlaybackID numbers the playbacks issued since the start; guarded by mutex.
	lastPlaybackID int64
}

func newTokens() *tokens {
	return &tokens{playbackByToken: map[string]*tokenPlayback{}}
}

// issue makes a token for one playback, forgetting those idle for longer than idle first.
func (all *tokens) issue(random io.Reader, playback tokenPlayback, now time.Time, idle time.Duration) (string, error) {
	token, err := randbytes.Base64URLSecret(random, tokenBytes)
	if err != nil {
		return "", fmt.Errorf("make a media token: %w", err)
	}
	all.mutex.Lock()
	defer all.mutex.Unlock()
	for key, known := range all.playbackByToken {
		if now.Sub(known.lastUsedAt) > idle {
			delete(all.playbackByToken, key)
		}
	}
	playback.lastUsedAt, playback.startedAt = now, now
	all.lastPlaybackID++
	playback.playbackID = all.lastPlaybackID
	all.playbackByToken[token] = &playback
	return token, nil
}

// use is the playback a token refers to, when holder holds it and it has not been idle for longer
// than idle; using it keeps it alive.
func (all *tokens) use(token string, holder Holder, now time.Time, idle time.Duration) (tokenPlayback, bool) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	known, found := all.playbackByToken[token]
	if !found {
		return tokenPlayback{}, false
	}
	if now.Sub(known.lastUsedAt) > idle {
		delete(all.playbackByToken, token)
		return tokenPlayback{}, false
	}
	// A playback the admin stopped serves nothing more; its next report is told why.
	if !known.isHeldBy(holder) || known.stopMessage != nil {
		return tokenPlayback{}, false
	}
	known.lastUsedAt = now
	return *known, true
}

// isHeldBy is whether holder, or the viewer it also is, holds the playback.
func (playback *tokenPlayback) isHeldBy(holder Holder) bool {
	return playback.isHeldByOne(holder) || (holder.Viewer != nil && playback.isHeldByOne(*holder.Viewer))
}

// isHeldByOne is whether one holder holds the playback: its person, under its session, reaching its
// library now, and for a viewer, the film the session plays now.
func (playback *tokenPlayback) isHeldByOne(holder Holder) bool {
	return holder.SessionKey != "" && playback.userID == holder.UserID &&
		playback.sessionKey == holder.SessionKey && holder.Reach.Reaches(&playback.libraryID) &&
		(holder.OnlyFileID == 0 || holder.OnlyFileID == playback.fileID)
}

// takeReport marks report seq of a token's playback taken, when holder holds it, and says whether
// it is new: a report numbered no higher than the last is a repeat. A
// report that could not be kept is given back with returnReport, so the player's retry is taken.
func (all *tokens) takeReport(
	token string, holder Holder, seq int64, now time.Time, idle time.Duration,
) (playback tokenPlayback, isNew, isFound bool) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	known, isFound := all.playbackByToken[token]
	if !isFound || now.Sub(known.lastUsedAt) > idle || !known.isHeldBy(holder) {
		return tokenPlayback{}, false, false
	}
	known.lastUsedAt = now
	if seq <= known.lastReportSeq {
		return *known, false, true
	}
	before := *known // with the last report's number, for returnReport
	known.lastReportSeq = seq
	return before, true, true
}

// returnReport undoes takeReport for a report that could not be kept, unless a later one was
// taken since.
func (all *tokens) returnReport(token string, seq, previousSeq int64) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if known, found := all.playbackByToken[token]; found && known.lastReportSeq == seq {
		known.lastReportSeq = previousSeq
	}
}

// NowPlaying is one playback heard from lately, as the admin's now playing shows it (cut-a §5).
type NowPlaying struct {
	PlaybackID int64
	UserID     int64
	DeviceID   int64
	FileID     int64
	StartedAt  time.Time
	LastHeard  time.Time
	PositionMs int64
	Decision   decide.Decision
}

// nowPlaying are the playbacks used since heardSince, oldest first.
func (all *tokens) nowPlaying(heardSince time.Time) []NowPlaying {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	var playing []NowPlaying
	for _, known := range all.playbackByToken {
		if known.lastUsedAt.Before(heardSince) || known.stopMessage != nil {
			continue
		}
		playing = append(playing, NowPlaying{
			PlaybackID: known.playbackID, UserID: known.userID, DeviceID: known.deviceID, FileID: known.fileID,
			StartedAt: known.startedAt, LastHeard: known.lastUsedAt, PositionMs: known.positionMs,
			Decision: known.decision,
		})
	}
	slices.SortFunc(playing, func(first, second NowPlaying) int {
		return cmp.Compare(first.PlaybackID, second.PlaybackID)
	})
	return playing
}

// stop marks the playback named playbackID stopped, with message for its player; false when there
// is none.
func (all *tokens) stop(playbackID int64, message string) bool {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	for _, known := range all.playbackByToken {
		if known.playbackID == playbackID && known.stopMessage == nil {
			known.stopMessage = &message
			return true
		}
	}
	return false
}

// end forgets a token, whose playback was stopped and whose player was told.
func (all *tokens) end(token string) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	delete(all.playbackByToken, token)
}

// countStalls adds count stalls to a token's playback, and returns how many it has had.
func (all *tokens) countStalls(token string, count int) int {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	known, found := all.playbackByToken[token]
	if !found {
		return 0
	}
	known.stalls += count
	return known.stalls
}

// forgetStalls forgets a token's stalls, which nothing could be offered for yet.
func (all *tokens) forgetStalls(token string) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if known, found := all.playbackByToken[token]; found {
		known.stalls = 0
	}
}

// offerOf is the lower quality a token's first stall started, and whether there is one.
func (all *tokens) offerOf(token string) (chunks.Offer, bool) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if known, found := all.playbackByToken[token]; found && known.offer != nil {
		return *known.offer, true
	}
	return chunks.Offer{}, false
}

// keepOffer keeps the lower quality a token's first stall started.
func (all *tokens) keepOffer(token string, offer chunks.Offer) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if known, found := all.playbackByToken[token]; found {
		known.offer = &offer
	}
}

// tellOffer marks a token's offer told, and says whether it was not told before.
func (all *tokens) tellOffer(token string) bool {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	known, found := all.playbackByToken[token]
	if !found || known.offer == nil || known.isOfferTold {
		return false
	}
	known.isOfferTold = true
	return true
}

// keepPosition keeps where a report said a token's playback is.
func (all *tokens) keepPosition(token string, positionMs int64) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	if known, found := all.playbackByToken[token]; found {
		known.positionMs = positionMs
	}
}

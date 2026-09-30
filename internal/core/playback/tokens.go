// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// tokenBytes is a media token's length before encoding: long enough that one cannot be guessed,
// though a token grants nothing on its own (D217 §2).
const tokenBytes = 24

// tokenPlayback is what a media token refers to: whose playback of which file, under which session.
type tokenPlayback struct {
	userID     int64
	sessionKey string
	fileID     int64
	// profile is a prepared playback's rendition; empty for direct play.
	profile    nodeproto.PrepareProfile
	lastUsedAt time.Time
	// lastReportSeq is the number of the last play report taken for this playback: the player
	// numbers its reports from 1, and one sent again is taken once.
	lastReportSeq int64
	// used is what the playback started with, which its file keeps once a report says it is started.
	used usedTracks
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
	playback.lastUsedAt = now
	all.playbackByToken[token] = &playback
	return token, nil
}

// use is the playback a token refers to, when it is this user's under this session and has not
// been idle for longer than idle; using it keeps it alive.
func (all *tokens) use(
	token string, userID int64, sessionKey string, now time.Time, idle time.Duration,
) (tokenPlayback, bool) {
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
	if known.userID != userID || known.sessionKey != sessionKey {
		return tokenPlayback{}, false
	}
	known.lastUsedAt = now
	return *known, true
}

// takeReport marks report seq of a token's playback taken, when it is this user's under this
// session, and says whether it is new: a report numbered no higher than the last is a repeat. A
// report that could not be kept is given back with returnReport, so the player's retry is taken.
func (all *tokens) takeReport(
	token string, userID int64, sessionKey string, seq int64, now time.Time, idle time.Duration,
) (playback tokenPlayback, isNew, isFound bool) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	known, isFound := all.playbackByToken[token]
	if !isFound || now.Sub(known.lastUsedAt) > idle || known.userID != userID || known.sessionKey != sessionKey {
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

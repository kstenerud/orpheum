// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// tokenBytes is a media token's length before encoding: long enough that one cannot be guessed,
// though a token grants nothing on its own (D217 §2).
const tokenBytes = 24

// playing is what a media token refers to: whose playback of which file, under which session.
type playing struct {
	userID  int64
	session string
	fileID  int64
	// profile is a prepared playback's rendition; empty for direct play.
	profile  nodeproto.PrepareProfile
	lastUsed time.Time
}

// tokens are the playbacks in progress, in memory: a restart ends them, and the client asks for
// a new one with POST /playback, as it would after the idle period.
type tokens struct {
	mutex   sync.Mutex
	playing map[string]*playing // guarded by mutex
}

func newTokens() *tokens {
	return &tokens{playing: map[string]*playing{}}
}

// issue makes a token for one playback, forgetting those idle for longer than idle first.
func (all *tokens) issue(random io.Reader, what playing, now time.Time, idle time.Duration) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", fmt.Errorf("make a media token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	all.mutex.Lock()
	defer all.mutex.Unlock()
	for key, known := range all.playing {
		if now.Sub(known.lastUsed) > idle {
			delete(all.playing, key)
		}
	}
	what.lastUsed = now
	all.playing[token] = &what
	return token, nil
}

// use is the playback a token refers to, when it is this user's under this session and has not
// been idle for longer than idle; using it keeps it alive.
func (all *tokens) use(token string, userID int64, session string, now time.Time, idle time.Duration) (playing, bool) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	known, found := all.playing[token]
	if !found {
		return playing{}, false
	}
	if now.Sub(known.lastUsed) > idle {
		delete(all.playing, token)
		return playing{}, false
	}
	if known.userID != userID || known.session != session {
		return playing{}, false
	}
	known.lastUsed = now
	return *known, true
}

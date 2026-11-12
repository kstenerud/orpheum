// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// LabAirPlayGrantLife is how long the AirPlay lab's grant serves its playback's media, unless its
// sender revokes it sooner (airplay.md item 3; the owner, 2026-11-12: "We'd need the lease to last
// much longer than the movie, because they could pause it"): the lab's 24 hours, which the spike's
// findings and the D154 §6 amendment, the owner's, replace.
const LabAirPlayGrantLife = 24 * time.Hour

// labAirPlayGrantsPerSession is how many AirPlay lab grants one sending session keeps at most;
// another forgets its oldest, so a page pressing again and again keeps no more.
const labAirPlayGrantsPerSession = 4

// ErrNoLabAirPlayGrant is an AirPlay lab grant unknown, revoked or expired: the three are not told
// apart, as a listener on the network who heard one should learn nothing (D135).
var ErrNoLabAirPlayGrant = errors.New("no such AirPlay grant: it was revoked, or it expired")

// ErrLabAirPlaySender is an AirPlay lab grant asked for by a session that cannot send one: a guest
// receiver's.
var ErrLabAirPlaySender = errors.New("this sign-in cannot AirPlay")

// labAirPlayGrants are the AirPlay lab's grants, by the hex of their secret's SHA-256: the secret
// itself is kept nowhere (D154 §1), as a guest receiver's grant is not (guestGrants). Kept in memory
// alone: a core started again has revoked them all.
type labAirPlayGrants struct {
	mutex  sync.Mutex
	byHash map[string]labAirPlayGrant
}

// labAirPlayGrant is one grant: the playback it serves, by its media token, of which file, and the
// session that started it, which alone revokes it.
type labAirPlayGrant struct {
	LabAirPlayGrant

	madeAt time.Time
}

// LabAirPlayGrant is what an AirPlay lab grant stands for (airplay.md item 3): one playback's media,
// by its token, of the one file FileID, as its sender's session holds them; never anything else.
type LabAirPlayGrant struct {
	UserID     int64
	SessionKey string
	MediaToken string
	FileID     int64
}

// forgetLocked forgets every grant expired at now, and the oldest of sessionKey's while it has as many
// as it may; the mutex is held.
func (kept *labAirPlayGrants) forgetLocked(now time.Time, sessionKey string) {
	oldest, count := "", 0
	for hash, each := range kept.byHash {
		if now.Sub(each.madeAt) >= LabAirPlayGrantLife {
			delete(kept.byHash, hash)
			continue
		}
		if each.SessionKey != sessionKey {
			continue
		}
		count++
		if oldest == "" || each.madeAt.Before(kept.byHash[oldest].madeAt) {
			oldest = hash
		}
	}
	if count >= labAirPlayGrantsPerSession {
		delete(kept.byHash, oldest)
	}
}

// IssueLabAirPlayGrant mints the AirPlay lab's grant for the playback of fileID that mediaToken names,
// which the sender's session holds (the caller has checked): random, kept only as its SHA-256, good
// for LabAirPlayGrantLife unless revoked. It goes in the media's path, the one place an AirPlay
// receiver, which runs none of our code, can carry it (airplay.md item 3), for the lab alone.
// ErrLabAirPlaySender for a guest receiver's session or one of no session key.
func (service *Service) IssueLabAirPlayGrant(sender Principal, mediaToken string, fileID int64) (string, error) {
	if sender.isGuestReceiver || sender.SessionKey == "" {
		return "", ErrLabAirPlaySender
	}
	secret, err := randomSecret(service.random)
	if err != nil {
		return "", err
	}
	now := service.clock.Now()
	service.labAirPlayGrants.mutex.Lock()
	defer service.labAirPlayGrants.mutex.Unlock()
	service.labAirPlayGrants.forgetLocked(now, sender.SessionKey)
	service.labAirPlayGrants.byHash[hex.EncodeToString(tokenHash(secret))] = labAirPlayGrant{
		LabAirPlayGrant: LabAirPlayGrant{
			UserID: sender.UserID, SessionKey: sender.SessionKey, MediaToken: mediaToken, FileID: fileID,
		},
		madeAt: now,
	}
	return secret, nil
}

// LabAirPlayGrantOf is what an AirPlay lab grant stands for while it lasts; ErrNoLabAirPlayGrant for
// one unknown, revoked or expired.
func (service *Service) LabAirPlayGrantOf(grant string) (LabAirPlayGrant, error) {
	now := service.clock.Now()
	hash := hex.EncodeToString(tokenHash(grant))
	service.labAirPlayGrants.mutex.Lock()
	defer service.labAirPlayGrants.mutex.Unlock()
	kept, found := service.labAirPlayGrants.byHash[hash]
	if !found || now.Sub(kept.madeAt) >= LabAirPlayGrantLife {
		delete(service.labAirPlayGrants.byHash, hash)
		return LabAirPlayGrant{}, ErrNoLabAirPlayGrant
	}
	return kept.LabAirPlayGrant, nil
}

// RevokeLabAirPlayGrants revokes every AirPlay lab grant the sender's session made, as its page's
// Stop does (airplay.md item 5), and says how many were still good: a receiver presenting one is
// refused from then on.
func (service *Service) RevokeLabAirPlayGrants(sender Principal) int {
	now := service.clock.Now()
	service.labAirPlayGrants.mutex.Lock()
	defer service.labAirPlayGrants.mutex.Unlock()
	revoked := 0
	for hash, each := range service.labAirPlayGrants.byHash {
		if each.SessionKey != sender.SessionKey || sender.SessionKey == "" {
			continue
		}
		if now.Sub(each.madeAt) < LabAirPlayGrantLife {
			revoked++
		}
		delete(service.labAirPlayGrants.byHash, hash)
	}
	return revoked
}

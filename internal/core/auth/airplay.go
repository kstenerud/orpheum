// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// AirPlayGrantLife is how long an AirPlay grant serves its playback's media, unless its sender
// revokes it sooner (D473 §2; the owner, 2026-11-12: "We'd need the lease to last much longer than
// the movie, because they could pause it").
const AirPlayGrantLife = 24 * time.Hour

// airPlayGrantsPerSession is how many AirPlay grants one sending session keeps at most; another
// forgets its oldest, so a page pressing again and again keeps no more.
const airPlayGrantsPerSession = 4

// airPlayIPv6Prefix is the length of the prefix an IPv6 receiver's grant is bound to (D473 §3): a
// device's own /64, within which its address may change while it plays.
const airPlayIPv6Prefix = 64

// ErrNoAirPlayGrant is an AirPlay grant unknown, revoked or expired, or presented from an address
// other than the one it is bound to: none is told apart, as a listener on the network who heard one
// should learn nothing (D135, D473 §3).
var ErrNoAirPlayGrant = errors.New("no such AirPlay grant: it was revoked, or it expired")

// ErrAirPlaySender is an AirPlay grant asked for by a session that cannot send one: a guest
// receiver's.
var ErrAirPlaySender = errors.New("this sign-in cannot AirPlay")

// airPlayGrants are the AirPlay grants, by the hex of their secret's SHA-256: the secret itself is
// kept nowhere (D154 §1), as a guest receiver's grant is not (guestGrants). Kept in memory alone: a
// core started again has revoked them all.
type airPlayGrants struct {
	mutex  sync.Mutex
	byHash map[string]airPlayGrant
}

// airPlayGrant is one grant: the playback it serves, by its media token, of which file, the session
// that started it, which alone revokes it, and the addresses it is bound to, one of each family, by
// the first request from each (D473 §3).
type airPlayGrant struct {
	AirPlayGrant

	madeAt  time.Time
	boundV4 netip.Addr
	boundV6 netip.Prefix
}

// AirPlayGrant is what an AirPlay grant stands for (D473 §2): one playback's media, by its token, of
// the one file FileID, as its sender's session holds them; never anything else.
type AirPlayGrant struct {
	UserID     int64
	SessionKey string
	MediaToken string
	FileID     int64
}

// admits is whether a request from address may present the grant, binding it to address's family
// on the first (D473 §3): an IPv4 address must be the first IPv4 address's, an IPv6 address within
// the first IPv6 address's /64. An IPv4 address mapped into IPv6 is taken as the IPv4 address it is.
func (grant *airPlayGrant) admits(address netip.Addr) bool {
	address = address.Unmap()
	switch {
	case address.Is4():
		if !grant.boundV4.IsValid() {
			grant.boundV4 = address
		}
		return grant.boundV4 == address
	case address.Is6():
		prefix, err := address.WithZone("").Prefix(airPlayIPv6Prefix)
		if err != nil {
			return false
		}
		if !grant.boundV6.IsValid() {
			grant.boundV6 = prefix
		}
		return grant.boundV6 == prefix
	}
	return false
}

// forgetLocked forgets every grant expired at now, and the oldest of sessionKey's while it has as many
// as it may; the mutex is held.
func (kept *airPlayGrants) forgetLocked(now time.Time, sessionKey string) {
	oldest, count := "", 0
	for hash, each := range kept.byHash {
		if now.Sub(each.madeAt) >= AirPlayGrantLife {
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
	if count >= airPlayGrantsPerSession {
		delete(kept.byHash, oldest)
	}
}

// IssueAirPlayGrant mints an AirPlay grant for the playback of fileID that mediaToken names, which
// the sender's session holds (the caller has checked): random, kept only as its SHA-256, good for
// AirPlayGrantLife unless revoked, and bound to no address until it is first presented. It goes in
// the media's path, the one place an AirPlay receiver, which runs none of our code, can carry it
// (D473 §1). ErrAirPlaySender for a guest receiver's session or one of no session key.
func (service *Service) IssueAirPlayGrant(sender Principal, mediaToken string, fileID int64) (string, error) {
	if sender.isGuestReceiver || sender.SessionKey == "" {
		return "", ErrAirPlaySender
	}
	secret, err := randomSecret(service.random)
	if err != nil {
		return "", err
	}
	now := service.clock.Now()
	service.airPlayGrants.mutex.Lock()
	defer service.airPlayGrants.mutex.Unlock()
	service.airPlayGrants.forgetLocked(now, sender.SessionKey)
	service.airPlayGrants.byHash[hex.EncodeToString(tokenHash(secret))] = airPlayGrant{
		AirPlayGrant: AirPlayGrant{
			UserID: sender.UserID, SessionKey: sender.SessionKey, MediaToken: mediaToken, FileID: fileID,
		},
		madeAt: now,
	}
	return secret, nil
}

// AirPlayGrantOf is what an AirPlay grant stands for, presented by a request from address (the
// connection's own, never a header's), while it lasts and its sender's session does. The first
// request of each address family binds the grant to it (admits). ErrNoAirPlayGrant for one unknown,
// revoked or expired, presented from another address, or whose session ended: signed out, ended by a
// new password or its device revoked, or its person removed. The grant serves that session's
// playback with no credential of its own, so it ends with the session, as the session's own media
// requests do.
func (service *Service) AirPlayGrantOf(ctx context.Context, grant string, address netip.Addr) (AirPlayGrant, error) {
	now := service.clock.Now()
	hash := hex.EncodeToString(tokenHash(grant))
	service.airPlayGrants.mutex.Lock()
	kept, found := service.airPlayGrants.byHash[hash]
	if !found || now.Sub(kept.madeAt) >= AirPlayGrantLife {
		delete(service.airPlayGrants.byHash, hash)
		service.airPlayGrants.mutex.Unlock()
		return AirPlayGrant{}, ErrNoAirPlayGrant
	}
	if !kept.admits(address) {
		service.airPlayGrants.mutex.Unlock()
		return AirPlayGrant{}, ErrNoAirPlayGrant
	}
	service.airPlayGrants.byHash[hash] = kept
	service.airPlayGrants.mutex.Unlock()
	isLive, err := service.isSessionLive(ctx, kept.SessionKey, now)
	if err != nil {
		return AirPlayGrant{}, err
	}
	if !isLive {
		service.airPlayGrants.mutex.Lock()
		delete(service.airPlayGrants.byHash, hash)
		service.airPlayGrants.mutex.Unlock()
		return AirPlayGrant{}, ErrNoAirPlayGrant
	}
	return kept.AirPlayGrant, nil
}

// isSessionLive is whether the session sessionKey names (Principal.SessionKey) has not ended by now
// and its person is not removed. Its idle end is not counted: the receiver plays on while the sending
// page waits, which uses the session not at all.
func (service *Service) isSessionLive(ctx context.Context, sessionKey string, now time.Time) (bool, error) {
	// A principal's key is always its session's hash in hex (sessionPrincipal), so one that is not is
	// a fault, not a session ended.
	session, err := hex.DecodeString(sessionKey)
	if err != nil {
		return false, fmt.Errorf("read an AirPlay grant's session key: %w", err)
	}
	queries := service.database.Read()
	grant, err := store.OrNotFound(queries.GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
		SecretSha256: session, Now: new(now.UnixMilli()),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up an AirPlay grant's session: %w", err)
	}
	if grant.Kind != grantKindSession || grant.UserID == nil {
		return false, nil
	}
	_, err = liveUser(ctx, queries, *grant.UserID)
	if errors.Is(err, ErrNoUser) {
		return false, nil
	}
	return err == nil, err
}

// RevokeAirPlayGrants revokes every AirPlay grant the sender's session made, as its page's Stop and
// the end of its AirPlay do (D473 §2), and says how many were still good: a receiver presenting one
// is refused from then on.
func (service *Service) RevokeAirPlayGrants(sender Principal) int {
	now := service.clock.Now()
	service.airPlayGrants.mutex.Lock()
	defer service.airPlayGrants.mutex.Unlock()
	revoked := 0
	for hash, each := range service.airPlayGrants.byHash {
		if each.SessionKey != sender.SessionKey || sender.SessionKey == "" {
			continue
		}
		if now.Sub(each.madeAt) < AirPlayGrantLife {
			revoked++
		}
		delete(service.airPlayGrants.byHash, hash)
	}
	return revoked
}

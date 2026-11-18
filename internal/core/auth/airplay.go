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
// that started it, which alone revokes it, the address of the sender that asked for it, and the
// addresses it is bound to, one of each family, by the first request from each other than the
// sender's (D473 §3, D474).
type airPlayGrant struct {
	AirPlayGrant

	madeAt   time.Time
	senderV4 netip.Addr
	senderV6 netip.Prefix
	boundV4  netip.Addr
	boundV6  netip.Prefix
}

// AirPlayGrant is what an AirPlay grant stands for (D473 §2): one playback's media, by its token, of
// the one file FileID, as its sender's session holds them; never anything else. A Watch Together
// guest's (D519 §1) has no person: SessionKey is their membership's as their page's playback holds it,
// and HoldCredential is the credential of their membership it holds it by, which the caller checks
// still admits them. Sound is the audio track its master playlist names its default (D521 §2).
type AirPlayGrant struct {
	UserID         int64
	SessionKey     string
	MediaToken     string
	FileID         int64
	HoldCredential string
	Sound          AirPlaySound
}

// AirPlaySound is the audio track the page plays as it asks for an AirPlay grant, by its place among
// the film's audio tracks (D521 §2), which the grant's receiver plays too: IsChosen false where the
// page named none.
type AirPlaySound struct {
	IsChosen        bool
	AudioTrackIndex int
}

// IsForMembership is whether the grant is a Watch Together guest's, held by HoldCredential.
func (grant AirPlayGrant) IsForMembership() bool {
	return grant.HoldCredential != ""
}

// AirPlayMembership is the Watch Together membership a guest by link asks for an AirPlay grant under
// (D519 §1): the key their page's playbacks are held by, as a person's session's is, and the
// credential the core made for the grant to hold the membership by (together's GuestAirPlayHold).
type AirPlayMembership struct {
	SessionKey     string
	HoldCredential string
}

// admits is whether a request from address may present the grant (D473 §3, as D474 amends it). The
// sender's own address is always admitted and binds nothing, since Safari on the sender fetches the
// master and the inits a moment before the receiver does, and plays the grant itself again when the
// receiver lets it go. Any other address binds the grant to its family on the first request from
// it: an IPv4 address must be the first such IPv4 address, an IPv6 address within the first such
// IPv6 address's /64.
func (grant *airPlayGrant) admits(address netip.Addr) bool {
	ipv4, ipv6Prefix, isAddress := addressKey(address)
	switch {
	case !isAddress:
		return false
	case ipv4.IsValid():
		if ipv4 == grant.senderV4 {
			return true
		}
		if !grant.boundV4.IsValid() {
			grant.boundV4 = ipv4
		}
		return grant.boundV4 == ipv4
	default:
		if ipv6Prefix == grant.senderV6 {
			return true
		}
		if !grant.boundV6.IsValid() {
			grant.boundV6 = ipv6Prefix
		}
		return grant.boundV6 == ipv6Prefix
	}
}

// addressKey is address as a grant tells addresses apart: an IPv4 address as itself, one mapped into
// IPv6 as the IPv4 address it maps, an IPv6 address by its /64, within which a device's address may
// change while it plays. False for an address of neither family.
func addressKey(address netip.Addr) (netip.Addr, netip.Prefix, bool) {
	address = address.Unmap()
	switch {
	case address.Is4():
		return address, netip.Prefix{}, true
	case address.Is6():
		prefix, err := address.WithZone("").Prefix(airPlayIPv6Prefix)
		return netip.Addr{}, prefix, err == nil
	}
	return netip.Addr{}, netip.Prefix{}, false
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
// the sender's session holds (the caller has checked), asked for from senderAddress (the
// connection's own): random, kept only as its SHA-256, good for AirPlayGrantLife unless revoked,
// served to the sender's address always, and bound to no other until one first presents it. It goes in
// the media's path, the one place an AirPlay receiver, which runs none of our code, can carry it
// (D473 §1). Its receiver plays sound, the track the page plays (D521 §2). ErrAirPlaySender for a
// guest receiver's session or one of no session key.
func (service *Service) IssueAirPlayGrant(
	sender Principal, senderAddress netip.Addr, mediaToken string, fileID int64, sound AirPlaySound,
) (string, error) {
	if sender.isGuestReceiver || sender.SessionKey == "" {
		return "", ErrAirPlaySender
	}
	secret, err := randomSecret(service.random)
	if err != nil {
		return "", err
	}
	service.keepAirPlayGrant(secret, senderAddress, AirPlayGrant{
		UserID: sender.UserID, SessionKey: sender.SessionKey, MediaToken: mediaToken, FileID: fileID, Sound: sound,
	})
	return secret, nil
}

// IssueMembershipAirPlayGrant is IssueAirPlayGrant for a Watch Together guest who joined by their
// link, signed in to nothing (D519 §1): the grant is for the playback of fileID that mediaToken names,
// which their membership holds (the caller has checked), and bound to the membership, not to a person
// or a session. Its life, its addresses, its film alone, its sound and its revocation
// (RevokeAirPlayGrantsOf) are a person's grant's; it serves while membership.HoldCredential admits
// the guest, which the caller checks at each request (AirPlayGrantOf). ErrAirPlaySender for no membership.
func (service *Service) IssueMembershipAirPlayGrant(
	membership AirPlayMembership, senderAddress netip.Addr, mediaToken string, fileID int64, sound AirPlaySound,
) (string, error) {
	if membership.SessionKey == "" || membership.HoldCredential == "" {
		return "", ErrAirPlaySender
	}
	secret, err := randomSecret(service.random)
	if err != nil {
		return "", err
	}
	service.keepAirPlayGrant(secret, senderAddress, AirPlayGrant{
		SessionKey: membership.SessionKey, MediaToken: mediaToken, FileID: fileID,
		HoldCredential: membership.HoldCredential, Sound: sound,
	})
	return secret, nil
}

// keepAirPlayGrant keeps the grant whose secret is secret, standing for granted, asked for from
// senderAddress, forgetting its sender's oldest while it has as many as it may.
func (service *Service) keepAirPlayGrant(secret string, senderAddress netip.Addr, granted AirPlayGrant) {
	now := service.clock.Now()
	service.airPlayGrants.mutex.Lock()
	defer service.airPlayGrants.mutex.Unlock()
	service.airPlayGrants.forgetLocked(now, granted.SessionKey)
	senderV4, senderV6, _ := addressKey(senderAddress)
	service.airPlayGrants.byHash[hex.EncodeToString(tokenHash(secret))] = airPlayGrant{
		AirPlayGrant: granted, madeAt: now, senderV4: senderV4, senderV6: senderV6,
	}
}

// AirPlayGrantOf is what an AirPlay grant stands for, presented by a request from address (the
// connection's own, never a header's), while it lasts and its sender's session does. The sender's
// address is always served; the first request of each address family from any other binds the grant
// to it (admits). ErrNoAirPlayGrant for one unknown,
// revoked or expired, presented from another address, or whose session ended: signed out, ended by a
// new password or its device revoked, or its person removed. The grant serves that session's
// playback with no credential of its own, so it ends with the session, as the session's own media
// requests do. A Watch Together guest's grant (IsForMembership) has no session here: the caller
// checks its HoldCredential still admits the guest, and serves nothing once it does not.
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
	if kept.IsForMembership() {
		return kept.AirPlayGrant, nil
	}
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
	return service.RevokeAirPlayGrantsOf(sender.SessionKey)
}

// RevokeAirPlayGrantsOf revokes every AirPlay grant made under sessionKey, a person's session's or a
// Watch Together guest's membership's (AirPlayMembership.SessionKey), as RevokeAirPlayGrants does.
func (service *Service) RevokeAirPlayGrantsOf(sessionKey string) int {
	now := service.clock.Now()
	service.airPlayGrants.mutex.Lock()
	defer service.airPlayGrants.mutex.Unlock()
	revoked := 0
	for hash, each := range service.airPlayGrants.byHash {
		if each.SessionKey != sessionKey || sessionKey == "" {
			continue
		}
		if now.Sub(each.madeAt) < AirPlayGrantLife {
			revoked++
		}
		delete(service.airPlayGrants.byHash, hash)
	}
	return revoked
}

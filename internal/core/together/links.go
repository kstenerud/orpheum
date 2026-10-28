// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"time"

	discoverylink "github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// link is one person's way in (D246 §3): single use, with the host's note of who it is for. Several
// links may give one place: a guest given a new link keeps their place until it is used, and the
// first used of a place's links ends the others (D318; the owner, 2026-10-19).
type link struct {
	id int64
	// placeID is the place it gives: its own id for a new person's, or forMember's.
	placeID int64
	// forMember is the member whose place it gives, moved to whoever joins by it, or 0 for a new
	// person's place.
	forMember  int64
	secretHash [sha256.Size]byte
	// locateHash is what a discovery finds the core by while the link is unused (link.LocateHash).
	locateHash string
	// secret is the link itself, kept for the host to pass on again: a page reloaded before it was
	// shared lost it, and a guest comes back by it on the browser that joined (D267 §1, §5).
	secret   string
	hostNote string
	// rights are what the person who joins by it may do to playback (D271 §4).
	rights Rights
	// usedBy is the member who joined by it, or 0 while it waits.
	usedBy int64
	// expiresAt is when it stops admitting anyone, unused: its lifetime after it was made (D346).
	expiresAt time.Time
}

// AddLink makes a link for one person to join by, once, with the host's note of who it is for,
// which only the host sees (D246 §3), and what they may do to playback (D271 §4).
func (hub *Hub) AddLink(sessionID, hostUserID int64, hostNote string, rights Rights) (LinkMade, error) {
	if hostNote == "" {
		return LinkMade{}, ErrNoNote
	}
	secret, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return LinkMade{}, fmt.Errorf("make a link: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return LinkMade{}, err
	}
	hub.lastID++
	now := hub.clock.Now()
	hosted.links[hub.lastID] = &link{
		id: hub.lastID, placeID: hub.lastID, secretHash: sha256.Sum256([]byte(secret)),
		locateHash: discoverylink.LocateHash(secret), secret: secret, hostNote: hostNote, rights: rights,
		expiresAt: now.Add(hub.lifetimes.Link),
	}
	hosted.touch(now)
	hosted.lastRights = rights
	hub.linksChanged()
	return LinkMade{LinkID: hub.lastID, Secret: secret}, nil
}

// AnotherLinkFor makes a new link for a guest's place, with their note and rights: they keep their
// place, on every device they came in on, until it is used, and whoever joins by it then takes the
// place over, those devices cut off (D311 §5, D312 §3; the owner, 2026-10-19). Only the host makes
// one, never for themselves.
func (hub *Hub) AnotherLinkFor(sessionID, hostUserID, memberID int64) (LinkMade, error) {
	secret, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return LinkMade{}, fmt.Errorf("make a link: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return LinkMade{}, err
	}
	guest, found := hosted.members[memberID]
	if !found || guest.isHost {
		return LinkMade{}, ErrNoMember
	}
	hub.lastID++
	now := hub.clock.Now()
	hosted.links[hub.lastID] = &link{
		id: hub.lastID, placeID: guest.placeID, forMember: guest.id, secretHash: sha256.Sum256([]byte(secret)),
		locateHash: discoverylink.LocateHash(secret), secret: secret, hostNote: guest.hostNote, rights: guest.rights,
		expiresAt: now.Add(hub.lifetimes.Link),
	}
	hosted.touch(now)
	hub.linksChanged()
	return LinkMade{LinkID: hub.lastID, Secret: secret}, nil
}

// RemoveLink withdraws a link: it admits no one from now on, and a member who joined by it stays.
func (hub *Hub) RemoveLink(sessionID, hostUserID, linkID int64) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return err
	}
	if _, found := hosted.links[linkID]; !found {
		return ErrNoLink
	}
	delete(hosted.links, linkID)
	hosted.touch(hub.clock.Now())
	return nil
}

// Join admits the person a link was made for, once, as a member named name, the name the others
// see (D246 §3); signed in, as person, whose devices have the membership back without the link
// (D267 §2). Not signed in, person is nil. A person is in one session at most (D372 §8): one in
// another, signed in or by the membership their browser keeps, leaving.HeldCredential, is refused
// with InAnotherSessionError until they say to leave it, leaving.ShouldLeave, and then leaves it.
func (hub *Hub) Join(secret, name string, person *Person, leaving Leaving) (Joined, error) {
	made, err := newCredential(hub.random)
	if err != nil {
		return Joined{}, err
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	now := hub.clock.Now()
	joined, used := hub.sessionByLink(secret)
	if used == nil || !used.waits(now) {
		return Joined{}, ErrNoSession // used, withdrawn, past its lifetime (D346), or its session over
	}
	if others := hub.placesOf(person, leaving.HeldCredential, joined.id); len(others) > 0 {
		if !leaving.ShouldLeave {
			return Joined{}, refusalFor(others[0])
		}
		for _, other := range others {
			hub.leavePlace(other)
		}
	}
	joined.touch(now)
	var taking *member
	if used.forMember != 0 {
		// A new link for a guest's place: whoever joins by it takes the place over, and the devices
		// the guest came in on are cut off; one ejected or gone since admits no one.
		taking = joined.members[used.forMember]
		if taking == nil {
			return Joined{}, ErrNoSession
		}
		taking.moveTo(name, person)
		joined.leaderStillHere(now, false, goneRemoved)
	} else {
		hub.lastID++
		taking = &member{
			id: hub.lastID, name: name, hostNote: used.hostNote, placeID: used.placeID, rights: used.rights,
			person: person, devices: map[int64]*device{},
		}
		joined.members[taking.id] = taking
	}
	used.usedBy = taking.id
	// The first of a place's links used ends the others (D318 §2).
	for id, other := range joined.links {
		if other != used && other.placeID == used.placeID {
			delete(joined.links, id)
		}
	}
	taking.addCredential(made)
	joined.sendRoster()
	hub.linksChanged()
	return Joined{
		SessionID: joined.id, MemberID: taking.id, MembershipCredential: made.credential, Locator: made.locator,
	}, nil
}

// moveTo hands the member's place to whoever joined by a new link for it, as name: every device
// they came in on is told and cut off, and none of their credentials admits anything more.
func (each *member) moveTo(name string, person *Person) {
	for _, attached := range each.devices {
		attached.send(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: "movedAway"})
	}
	each.name, each.person = name, person
	each.devices = map[int64]*device{}
	each.credentials = nil
}

// endLinksOf ends every link of a member's place, theirs and any new one made for it, as they leave
// the session: revoking a place ends every link to it (D318 §3).
func (current *session) endLinksOf(gone *member) {
	for id, each := range current.links {
		if each.placeID == gone.placeID || each.forMember == gone.id {
			delete(current.links, id)
		}
	}
}

// sessionByLink is the session and link a link's secret names, or nils.
func (hub *Hub) sessionByLink(secret string) (*session, *link) {
	hash := sha256.Sum256([]byte(secret))
	for _, each := range hub.sessions {
		for _, candidate := range each.links {
			if subtle.ConstantTimeCompare(candidate.secretHash[:], hash[:]) == 1 {
				return each, candidate
			}
		}
	}
	return nil, nil
}

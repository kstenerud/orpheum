// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"crypto/sha256"
	"fmt"
	"slices"
	"time"
)

// A person is in one session at most, as host or as guest (D372 §8). Starting a session ends the
// one they host and takes them out of one they joined (Open); joining another asks first: the join
// is refused with InAnotherSessionError until the person says to leave the other (Leaving). A person
// is known by their sign-in, and a guest by link alone by the membership their browser keeps, which
// the join is given. A cast is not a session of the person's, and is left alone.

// place is a person's place in a session: the session, and their member in it.
type place struct {
	in *session
	as *member
}

// Leaving is what a join is told of the joining person's other session: a membership the browser
// keeps, of one it may be in by link alone, and whether the person said to leave the other to join
// this one, asked "Leave ‹session› to join this one?" (D372 §8).
type Leaving struct {
	HeldCredential string
	ShouldLeave    bool
}

// InAnotherSessionError is a join refused because its person is in another session, which they
// have not said to leave: whose it is, its host's name, and whether it is their own, for the
// question the client asks (D372 §8).
type InAnotherSessionError struct {
	HostName string
	IsHost   bool
}

func (refused *InAnotherSessionError) Error() string {
	if refused.IsHost {
		return "you host another Watch Together session; leave it to join this one"
	}
	return fmt.Sprintf("you are in %s's Watch Together session; leave it to join this one", refused.HostName)
}

// placesOf is every place person, signed in, holds, and the place held names by its credential,
// in a Watch Together session other than except: the host's first, then by session.
func (hub *Hub) placesOf(person *Person, held string, except int64) []place {
	var heldHash [sha256.Size]byte
	if held != "" {
		heldHash = sha256.Sum256([]byte(held))
	}
	var places []place
	for _, each := range hub.sessions {
		if each.isCast || each.id == except {
			continue
		}
		for _, candidate := range each.members {
			if each.isPersons(candidate, person) || (held != "" && candidate.admits(heldHash)) {
				places = append(places, place{in: each, as: candidate})
			}
		}
	}
	slices.SortFunc(places, func(one, other place) int {
		if one.as.isHost != other.as.isHost {
			if one.as.isHost {
				return -1
			}
			return 1
		}
		return cmp.Compare(one.in.id, other.in.id)
	})
	return places
}

// isPersons is whether candidate is person's place in the session: they host it, or joined it
// signed in; false for nobody.
func (current *session) isPersons(candidate *member, person *Person) bool {
	if person == nil {
		return false
	}
	return (candidate.isHost && current.hostUserID == person.UserID) ||
		(candidate.person != nil && candidate.person.UserID == person.UserID)
}

// refusalFor is the join's refusal for the person's other place.
func refusalFor(other place) *InAnotherSessionError {
	refused := &InAnotherSessionError{IsHost: other.as.isHost}
	for _, each := range other.in.members {
		if each.isHost {
			refused.HostName = each.name
		}
	}
	return refused
}

// leavePlace takes its person out of a session for another: one they host ends, each device told
// the host ended it; from one they joined, they leave, each of their devices told so.
func (hub *Hub) leavePlace(held place) {
	now := hub.clock.Now()
	hub.logger.Info("watch together: a person left a session for another",
		sessionIDAttribute, held.in.id, "memberId", held.as.id, "wasHost", held.as.isHost)
	if held.as.isHost {
		held.in.broadcast(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: "hostEnded"})
		delete(hub.sessions, held.in.id)
		return
	}
	held.in.remove(now, held.as, "left")
}

// remove takes a guest out of the session: each of their devices told why, their place's links
// ended, a lead of theirs passed on (D372 §5), and everyone told the roster.
func (current *session) remove(now time.Time, gone *member, reason string) {
	for _, each := range gone.devices {
		each.send(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: reason})
	}
	current.endLinksOf(gone)
	delete(current.members, gone.id)
	current.touch(now)
	current.leaderStillHere(now, false, goneRemoved)
	current.sendRoster()
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package together

// A Watch Together guest who joined by their link, signed in to nothing, casts to a Google Cast
// device as another device of their membership (D518 §1): the device trades a grant bound to the
// membership (internal/core/auth's IssueMembershipGuestGrant) for a credential of the member's own,
// and with it follows the group as just another follower, as a signed-in guest's receiver does
// (TogetherPart, D371 §4), playing what the session plays by the membership (D246 §3) with exactly
// the guest's rights (D372 §4). It is no cast: no session of one, no person, no device of anyone's.
// It ends as the membership ends, the guest removed or the session ended, its device told why as each
// of the member's is.

// GuestCastPart is a new credential, and its device's locator, for a Google Cast device to take the
// part of member memberID in session sessionID, a guest's, while they are in (D518 §1). ErrNoSession
// for a session ended, or a cast; ErrNoMember for a member gone, removed or left, or the host, whose
// cast is a signed-in person's (TogetherPart).
func (hub *Hub) GuestCastPart(sessionID, memberID int64) (Joined, error) {
	made, err := newCredential(hub.random)
	if err != nil {
		return Joined{}, err
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	in, found := hub.sessions[sessionID]
	if !found || in.isCast {
		return Joined{}, ErrNoSession
	}
	guest, found := in.members[memberID]
	if !found || guest.isHost {
		return Joined{}, ErrNoMember
	}
	guest.addCredential(made)
	hub.linksChanged()
	return Joined{
		SessionID: sessionID, MemberID: memberID, MembershipCredential: made.credential, Locator: made.locator,
	}, nil
}

// MemberOfLocator is the session and member one of whose devices finds the core by the locator whose
// hash is hash (link.LocatorHash), while the membership lasts: whose waiting grants a discovery's offer
// under it may be proven by (D518 §1). False for any other hash, the host's included, whose device
// finds the core by their own session's locator.
func (hub *Hub) MemberOfLocator(hash string) (sessionID, memberID int64, isFound bool) {
	if hash == "" {
		return 0, 0, false
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	for _, each := range hub.sessions {
		for _, candidate := range each.members {
			for _, held := range candidate.credentials {
				if held.locatorHash == hash {
					return each.id, candidate.id, true
				}
			}
		}
	}
	return 0, 0, false
}

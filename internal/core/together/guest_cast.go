// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"crypto/sha256"
	"slices"
)

// A Watch Together guest who joined by their link, signed in to nothing, casts to a Google Cast
// device as another device of their membership (D518 §1): the device trades a grant bound to the
// membership (internal/core/auth's IssueMembershipGuestGrant) for a credential of the member's own,
// and with it follows the group as just another follower, as a signed-in guest's receiver does
// (TogetherPart, D371 §4), playing what the session plays by the membership (D246 §3) with exactly
// the guest's rights (D372 §4). It is no cast: no session of one, no person, no device of anyone's.
// It ends as the membership ends, the guest removed or the session ended, its device told why as each
// of the member's is. A guest has one cast's credential at a time (D519 §2): each new cast's replaces
// the last's, whose receiver is cut off as a device whose place moved away is, so casting again and
// again never pushes the guest's own browser out of the newest credentials the member keeps.

// GuestCastPart is a new credential, and its device's locator, for a Google Cast device to take the
// part of member memberID in session sessionID, a guest's, while they are in (D518 §1). It replaces
// the guest's last cast's credential, that receiver told its place moved away and cut off (D519 §2).
// ErrNoSession for a session ended, or a cast; ErrNoMember for a member gone, removed or left, or the
// host, whose cast is a signed-in person's (TogetherPart).
func (hub *Hub) GuestCastPart(sessionID, memberID int64) (Joined, error) {
	made, err := newCredential(hub.random)
	if err != nil {
		return Joined{}, err
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if replaceErr := hub.replaceGuestsCastLocked(sessionID, memberID, made); replaceErr != nil {
		return Joined{}, replaceErr
	}
	hub.linksChanged()
	return Joined{
		SessionID: sessionID, MemberID: memberID, MembershipCredential: made.credential, Locator: made.locator,
	}, nil
}

// GuestAirPlayHold is a new credential of member memberID in session sessionID, a guest's, for an
// AirPlay grant they asked for to hold their membership by while they are in (D519 §1): it is held by
// the core alone, never handed to a device, and has no locator, as no device finds the core by it. As
// a Chromecast's part does, it replaces the guest's last cast's credential (D519 §2). ErrNoSession and
// ErrNoMember as GuestCastPart.
func (hub *Hub) GuestAirPlayHold(sessionID, memberID int64) (string, error) {
	made, err := newCredential(hub.random)
	if err != nil {
		return "", err
	}
	made.locator = ""
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if replaceErr := hub.replaceGuestsCastLocked(sessionID, memberID, made); replaceErr != nil {
		return "", replaceErr
	}
	hub.linksChanged()
	return made.credential, nil
}

// replaceGuestsCastLocked gives guest memberID of session sessionID made as their cast's credential,
// in place of their last cast's (replaceCastCredential), and hands the lead on and tells the roster
// where that receiver was cut off; the mutex is held. ErrNoSession and ErrNoMember as GuestCastPart.
func (hub *Hub) replaceGuestsCastLocked(sessionID, memberID int64, made madeCredential) error {
	casting, found := hub.sessions[sessionID]
	if !found || casting.isCast {
		return ErrNoSession
	}
	guest, found := casting.members[memberID]
	if !found || guest.isHost {
		return ErrNoMember
	}
	if guest.replaceCastCredential(made) {
		casting.leaderStillHere(hub.clock.Now(), false, goneReplaced)
		casting.sendRoster()
	}
	return nil
}

// replaceCastCredential makes made the member's cast's credential (D519 §2): the last cast's admits
// nothing more, and each device that said hello with it is told its place moved away and let go, as
// cutOff tells a device whose place a new link took. Whether any device was let go.
func (each *member) replaceCastCredential(made madeCredential) bool {
	isCutOff := false
	if replaced := each.castCredential; replaced != ([sha256.Size]byte{}) {
		each.credentials = slices.DeleteFunc(each.credentials, func(kept credential) bool {
			return kept.hash == replaced
		})
		for id, attached := range each.devices {
			if attached.credentialHash == replaced {
				attached.send(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: "movedAway"})
				delete(each.devices, id)
				isCutOff = true
			}
		}
	}
	each.addCredential(made)
	each.castCredential = sha256.Sum256([]byte(made.credential))
	return isCutOff
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

// ViewerHeldBy is the viewer an AirPlay grant of a guest's holds their membership by (D519 §1), hold
// being the credential GuestAirPlayHold made for it, while it admits them: as ViewerOf, but of a
// credential the core keeps for a grant, which no caller presents.
func (hub *Hub) ViewerHeldBy(hold string) (Viewer, bool) {
	return hub.viewerByCredential(hold)
}

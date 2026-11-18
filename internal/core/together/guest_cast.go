// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"crypto/sha256"
)

// A Watch Together guest who joined by their link, signed in to nothing, casts to a Google Cast
// device as another device of their membership (D518 §1): the device trades a grant bound to the
// membership (internal/core/auth's IssueMembershipGuestGrant) for a credential of the member's own,
// and with it follows the group as just another follower, as a signed-in guest's receiver does
// (TogetherPart, D371 §4), playing what the session plays by the membership (D246 §3) with exactly
// the guest's rights (D372 §4). It is no cast: no session of one, no person, no device of anyone's.
// It ends as the membership ends, the guest removed or the session ended, its device told why as each
// of the member's is. One device casts to one other at a time (D520 §1, amending D519 §2): a cast's
// credential is the sending device's, the one whose membership credential asked, and that device's
// new cast replaces its last alone, whose receiver is cut off as a device whose place moved away is;
// it ends as the sending device's credential goes; and a device being cast to casts nothing itself.
// So casting again and again never pushes the guest's own browser out of the newest credentials the
// member keeps.

// GuestCastPart is a new credential, and its device's locator, for a Google Cast device to take the
// part of member memberID in session sessionID, a guest's, while they are in (D518 §1), cast by the
// guest's device whose membership credential's hash is sender. It replaces that device's last cast's
// credential, that receiver told its place moved away and cut off (D520 §1). ErrNoSession for a
// session ended, or a cast; ErrNoMember for a member gone, removed or left, or the host, whose cast
// is a signed-in person's (TogetherPart), and for a sender whose credential no longer admits them;
// ErrBeingCastTo for a sender that is itself a cast's receiver.
func (hub *Hub) GuestCastPart(sessionID, memberID int64, sender [sha256.Size]byte) (Joined, error) {
	made, err := newCredential(hub.random)
	if err != nil {
		return Joined{}, err
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if replaceErr := hub.replaceGuestsCastLocked(sessionID, memberID, sender, made); replaceErr != nil {
		return Joined{}, replaceErr
	}
	hub.linksChanged()
	return Joined{
		SessionID: sessionID, MemberID: memberID, MembershipCredential: made.credential, Locator: made.locator,
	}, nil
}

// GuestAirPlayHold is a new credential of member memberID in session sessionID, a guest's, for an
// AirPlay grant their device whose membership credential's hash is sender asked for, to hold their
// membership by while they are in (D519 §1): it is held by the core alone, never handed to a device,
// and has no locator, as no device finds the core by it. As a Chromecast's part does, it replaces that
// device's last cast's credential (D520 §1). Errors as GuestCastPart's.
func (hub *Hub) GuestAirPlayHold(sessionID, memberID int64, sender [sha256.Size]byte) (string, error) {
	made, err := newCredential(hub.random)
	if err != nil {
		return "", err
	}
	made.locator = ""
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if replaceErr := hub.replaceGuestsCastLocked(sessionID, memberID, sender, made); replaceErr != nil {
		return "", replaceErr
	}
	hub.linksChanged()
	return made.credential, nil
}

// IsBeingCastTo is whether credential is a cast's receiver's, a Google Cast device's part a guest gave
// it (GuestCastPart), which casts nothing itself (D520 §1).
func (hub *Hub) IsBeingCastTo(credential string) bool {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	_, holder := hub.memberByCredential(credential)
	if holder == nil {
		return false
	}
	return holder.isCastCredential(sha256.Sum256([]byte(credential)))
}

// isCastCredential is whether the member's credential whose hash is hash is a cast's, given its
// receiver; false too for a credential not kept.
func (each *member) isCastCredential(hash [sha256.Size]byte) bool {
	for _, kept := range each.credentials {
		if kept.hash == hash {
			return kept.castOf != ([sha256.Size]byte{})
		}
	}
	return false
}

// replaceGuestsCastLocked gives guest memberID of session sessionID made as the cast's credential of
// their device whose credential's hash is sender, in place of that device's last cast's
// (replaceCastCredential), and hands the lead on and tells the roster where that receiver was cut
// off; the mutex is held. Errors as GuestCastPart's.
func (hub *Hub) replaceGuestsCastLocked(
	sessionID, memberID int64, sender [sha256.Size]byte, made madeCredential,
) error {
	casting, found := hub.sessions[sessionID]
	if !found || casting.isCast {
		return ErrNoSession
	}
	guest, found := casting.members[memberID]
	if !found || guest.isHost || !guest.admits(sender) {
		return ErrNoMember
	}
	if guest.isCastCredential(sender) {
		return ErrBeingCastTo
	}
	if guest.replaceCastCredential(made, sender) {
		casting.leaderStillHere(hub.clock.Now(), false, goneReplaced)
		casting.sendRoster()
	}
	return nil
}

// replaceCastCredential makes made the cast's credential of the member's device whose credential's
// hash is sender (D520 §1): that device's last cast's admits nothing more, and each device that said
// hello with it is told its place moved away and let go (endCastsWhere); another device's cast plays
// on. Whether any device was let go.
func (each *member) replaceCastCredential(made madeCredential, sender [sha256.Size]byte) bool {
	isCutOff := each.endCastsWhere(func(cast credential) bool { return cast.castOf == sender })
	return each.addCastCredential(made, sender) || isCutOff
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

// SPDX-License-Identifier: AGPL-3.0-or-later

package together

// What the hub tells the server's upload of its sessions (D286 §5): how many players a session's
// film starts for, and the newest member sent away when the upload cannot carry everyone.

// SendAwayNewest sends away the member who joined most recently of those with a player, in any
// session, never a host: each of their devices is told the session ended for them with reason, and
// their membership is kept, so they may come back at once (D286 §5: the server's upload cannot carry
// every player at the lowest qualities). It says whom, and whether there was anyone.
func (hub *Hub) SendAwayNewest(reason string) (sessionID, memberID int64, isSent bool) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	var newest *member
	for _, each := range hub.sessions {
		for _, candidate := range each.members {
			if !candidate.isHost && candidate.hasPlayer() && (newest == nil || candidate.id > newest.id) {
				newest, sessionID = candidate, each.id
			}
		}
	}
	if newest == nil {
		return 0, 0, false
	}
	for _, each := range newest.devices {
		each.send(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: reason})
	}
	return sessionID, newest.id, true
}

// PlayersHostedBy is how many players a film put on in the session hostUserID hosts starts for: its
// devices that play the film, or, where more, its members with any device, each of whom will play;
// zero when they host none. The upload is shared evenly among them (D286 §5). A host's own player
// joins the session only after its playback has started, so counting the players alone, the host's
// start saw the guest's alone, took the share whole, and left an iPhone guest the lowest quality (the
// owner's test, 2026-10-12).
func (hub *Hub) PlayersHostedBy(hostUserID int64) int {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	players, members := 0, 0
	for _, each := range hub.sessions {
		if each.hostUserID != hostUserID {
			continue
		}
		for _, member := range each.members {
			if len(member.devices) > 0 {
				members++
			}
			for _, attached := range member.devices {
				if attached.role.plays() {
					players++
				}
			}
		}
	}
	return max(players, members)
}

// hasPlayer is whether any of the member's devices plays the film.
func (each *member) hasPlayer() bool {
	for _, attached := range each.devices {
		if attached.role.plays() {
			return true
		}
	}
	return false
}

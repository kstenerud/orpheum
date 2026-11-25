// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"maps"
	"slices"
)

// A signed-in person's part in a Watch Together session is what Mine lists of it; each page of
// theirs that is open shows it (the offer to start a session, or to move one here). Whatever changes
// it, a start, a join, a leave, a move to another device, a device gone, the session's end, is told
// once the hub's lock is let go (OnPartChanged), so the person's pages can read it again; the hub
// finds what changed by comparing each session's parts with those last told, rather than by each
// path saying so.

// part is a signed-in person's part in a session as Mine lists it.
type part struct {
	memberID int64
	isHost   bool
	// isOnADevice is whether it holds a credential of a device of the person's own (holdsOwnCredential).
	isOnADevice bool
	// devices are the person's signed-in devices it is on now (signedInDevices).
	devices []int64
}

// OnPartChanged has changed called with each signed-in person whose part in a Watch Together
// session changed: they started or joined one, left it or were taken out, it moved to another device
// of theirs or a device of theirs attached to it or went from it, or it ended. A cast's session of
// one is none. Called before the hub is used; changed is called once the hub's lock is let go, after
// any cast moved or session ended is told, so it may call the hub.
func (hub *Hub) OnPartChanged(changed func(userID int64)) {
	hub.onPartChanged = changed
}

// parts are the session's signed-in people's parts now, by user.
func (current *session) parts() map[int64]part {
	parts := map[int64]part{}
	for _, each := range current.members {
		if each.person == nil {
			continue
		}
		parts[each.person.UserID] = part{
			memberID: each.id, isHost: each.isHost, isOnADevice: each.holdsOwnCredential(),
			devices: each.signedInDevices(),
		}
	}
	return parts
}

// partsGone are the people whose part ends with the session: those in it now, and those last told
// of it.
func (current *session) partsGone() []int64 {
	gone := slices.Collect(maps.Keys(current.parts()))
	for userID := range current.toldParts {
		if !slices.Contains(gone, userID) {
			gone = append(gone, userID)
		}
	}
	return gone
}

// partsChanged are the people, each once, whose part in a session changed since onPartChanged was
// last told: of a session ended (partsEnded), or not as last told (toldParts), which is told them
// now. A cast's session of one has no parts. The caller holds the mutex.
func (hub *Hub) partsChanged() []int64 {
	changed := hub.partsEnded
	hub.partsEnded = nil
	for _, each := range hub.sessions {
		if each.isCast {
			continue
		}
		now := each.parts()
		for userID, was := range each.toldParts {
			if is, found := now[userID]; !found || !samePart(was, is) {
				changed = append(changed, userID)
			}
		}
		for userID := range now {
			if _, found := each.toldParts[userID]; !found {
				changed = append(changed, userID)
			}
		}
		each.toldParts = now
	}
	slices.Sort(changed)
	return slices.Compact(changed)
}

// samePart is whether two parts are the same, as Mine lists them.
func samePart(was, is part) bool {
	return was.memberID == is.memberID && was.isHost == is.isHost && was.isOnADevice == is.isOnADevice &&
		slices.Equal(was.devices, is.devices)
}

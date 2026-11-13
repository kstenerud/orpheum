// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"reflect"
	"slices"
)

// roster is every member and their devices, as everyone sees them: never the host's notes.
func (current *session) roster() []MemberView {
	views := make([]MemberView, 0, len(current.members))
	for _, each := range current.members {
		view := MemberView{ID: each.id, Name: each.name, IsHost: each.isHost, Devices: []DeviceView{}}
		for _, attached := range each.devices {
			isLeader := attached.id == current.leaderID
			leadsBecause := ""
			if isLeader {
				leadsBecause = current.leadsBecause
			}
			view.Devices = append(view.Devices, DeviceView{
				ID: attached.id, Role: attached.role, IsBuffering: attached.buffering,
				Suspended: attached.suspended, RTTMs: attached.rttMs, CannotKeepUp: attached.cannotKeepUp,
				Status: attached.status(current.group.fileID != 0, isLeader), IsLeader: isLeader,
				Playback: attached.playback, LeadsBecause: leadsBecause,
			})
		}
		slices.SortFunc(view.Devices, func(one, other DeviceView) int { return cmp.Compare(one.ID, other.ID) })
		views = append(views, view)
	}
	// In the order they joined, the host first: the members are kept by id, and a map's order is
	// chance, which swapped the names on everyone's screen (the owner's test, 2026-10-05).
	slices.SortFunc(views, func(one, other MemberView) int { return cmp.Compare(one.ID, other.ID) })
	return views
}

// sendRoster tells every device the roster.
func (current *session) sendRoster() {
	current.lastRoster = current.roster()
	current.broadcast(Outbound{V: ProtocolVersion, Type: MessageRoster, Roster: current.lastRoster})
}

// sendRosterIfChanged tells every device the roster when it is not the one they were last told: a
// device's state report comes each second while its group waits, from each device, and each sent
// every device the roster, the same one most of the time (the audit of 2026-10-25, R2).
func (current *session) sendRosterIfChanged() {
	if roster := current.roster(); !reflect.DeepEqual(roster, current.lastRoster) {
		current.sendRoster()
	}
}

// broadcast sends message to every device of every member, encoded once for them all.
func (current *session) broadcast(message Outbound) {
	message.encoded = &sharedEncoding{}
	for _, each := range current.members {
		for _, attached := range each.devices {
			attached.send(message)
		}
	}
}

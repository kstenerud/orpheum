// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"testing"
)

// A guest whose link gives no controls is refused every command the group takes, from their own
// browser and from the Chromecast taking their part alike, and the group stays as it was: so AirPlay,
// offered whatever the link's controls, changes nothing by its remote or the lock screen, whatever a
// page sends for it (D520 §2).
func TestHub_AGuestWhoseLinkGivesNoControlsChangesNothing(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	guest := stage.join(t)
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
	if err != nil {
		t.Fatal(err)
	}
	commands := []Inbound{
		{Type: MessageCommand, Command: CommandPlay},
		{Type: MessageCommand, Command: CommandPause},
		{Type: MessageCommand, Command: CommandSeek, PositionMs: 60_000},
		{Type: MessageCommand, Command: CommandRate, Rate: 2},
		{Type: MessageCommand, Command: CommandChangeItem, FileID: 9},
		{Type: MessageCommand, Command: CommandStop},
		{Type: MessageCommand, Command: CommandNext},
		{Type: MessageCommand, Command: CommandPrevious},
	}
	for name, credential := range map[string]string{
		"the guest's browser": guest.MembershipCredential, "the guest's Chromecast": part.MembershipCredential,
	} {
		device, box := stage.attach(t, credential, RoleVideo)
		groupsBefore := hostBox.count(MessageGroup)
		for _, command := range commands {
			refusedBefore := box.count(MessageRejected)
			stage.hub.Handle(t.Context(), device, command)
			if refused := box.last(MessageRejected); box.count(MessageRejected) != refusedBefore+1 ||
				refused.Code != codeForbidden {
				t.Errorf("%s's %s = %+v; want it refused as forbidden", name, command.Command, refused)
			}
		}
		if groups := hostBox.count(MessageGroup); groups != groupsBefore {
			t.Errorf("%s's refused commands sent the group %d changes; want none", name, groups-groupsBefore)
		}
		if film := stage.filmOn(stage.opened.SessionID); film != 7 {
			t.Errorf("%s's refused commands left film %d on; want 7", name, film)
		}
	}
}

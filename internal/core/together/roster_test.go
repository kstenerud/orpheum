// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"bytes"
	"maps"
	"testing"
)

// A device's state report that changes nothing anyone sees sends no roster: while a group waits each
// device reports every second, and each report sent every device the roster, the same one (the
// audit of 2026-10-25, R2). One that changes it is sent as before.
func TestHub_AStateThatChangesNothingSendsNoRoster(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guestDevice, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	stage.ready(t, guestDevice, true)
	before := hostBox.count(MessageRoster)
	for range 5 {
		stage.ready(t, guestDevice, true)
	}
	if sent := hostBox.count(MessageRoster) - before; sent != 0 {
		t.Errorf("five reports that changed nothing sent the host %d rosters; want none", sent)
	}
	stage.ready(t, guestDevice, false)
	if sent := hostBox.count(MessageRoster) - before; sent != 1 {
		t.Errorf("a report that changed the guest's buffering sent the host %d rosters; want one", sent)
	}
}

// A message sent to every device is encoded once, its bytes shared by every device's writer, and
// encodes as the same message sent to one device would.
func TestBroadcast_IsEncodedOnceForEveryDevice(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	_, guestBox := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	toHost, toGuest := hostBox.last(MessageRoster), guestBox.last(MessageRoster)
	hostBytes, hostErr := toHost.MarshalJSON()
	guestBytes, guestErr := toGuest.MarshalJSON()
	if hostErr != nil || guestErr != nil || len(hostBytes) == 0 || &hostBytes[0] != &guestBytes[0] {
		t.Fatalf("the roster to the host and to the guest encoded apart (%v, %v); want one encoding", hostErr, guestErr)
	}
	alone := toHost
	alone.encoded = nil
	if aloneBytes, err := alone.MarshalJSON(); err != nil || !bytes.Equal(aloneBytes, hostBytes) {
		t.Errorf("the roster sent to one device encodes as %s (%v); want %s, the broadcast's", aloneBytes, err,
			hostBytes)
	}
}

// The roster says why its leader leads, as the leaderChanged line says it, a leader gone's reason
// after a colon, and of no other device: a person's debug view shows it (plans: Show debug tools).
func TestHub_TheRosterSaysWhyItsLeaderLeads(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 2)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	want := map[int64]string{stage.host.DeviceID: leadPutOn}
	if why := rosterLeadsBecause(watchBox); !maps.Equal(why, want) {
		t.Errorf("the roster's reasons: %v; want %v, the host's player's alone", why, want)
	}
	stage.hide(t, stage.host, true)
	want = map[int64]string{stage.guests[0].DeviceID: "leaderGone:hidden"}
	if why := rosterLeadsBecause(watchBox); !maps.Equal(why, want) {
		t.Errorf("the roster's reasons, the host's page hidden: %v; want %v", why, want)
	}
}

// rosterLeadsBecause is why each device of the last roster box was told leads, of those it says.
func rosterLeadsBecause(box *inbox) map[int64]string {
	reasons := map[int64]string{}
	for _, each := range box.last(MessageRoster).Roster {
		for _, attached := range each.Devices {
			if attached.LeadsBecause != "" {
				reasons[attached.ID] = attached.LeadsBecause
			}
		}
	}
	return reasons
}

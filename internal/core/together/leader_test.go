// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// guestsStage is a stage with film 7 playing, the host's player leading and guests' players following,
// each attached a second after the last: the first guest's the most senior.
type guestsStage struct {
	sessionStage
	host    Attachment
	hostBox *inbox
	guests  []Attachment
}

// withGuests is film 7 playing with count guests' players, the host's leading.
func withGuests(t *testing.T, count int) guestsStage {
	t.Helper()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	guests := make([]Attachment, 0, count)
	for range count {
		stage.clock.Advance(time.Second)
		guestDevice, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
		stage.ready(t, guestDevice, false)
		guests = append(guests, guestDevice)
	}
	return guestsStage{sessionStage: stage, host: host, hostBox: hostBox, guests: guests}
}

// While none of the host's devices can lead, the player present longest leads, a guest's, by the
// moment the core saw it arrive, at microsecond resolution; arrived the same microsecond, the lower
// id. The log says why and the new leader's seniority (D372 §1).
func TestHub_TheLeadPassesToThePlayerPresentLongest(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 2)
	stage.clock.Advance(time.Microsecond / 2) // the next two arrive in the same microsecond
	sameFirst, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	sameSecond, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	stage.clock.Advance(90 * time.Second)
	stage.hub.Detach(stage.host, GoneClosed)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	if leader := rosterLeader(watchBox); leader != stage.guests[0].DeviceID {
		t.Errorf("device %d leads, the host's player gone; want the first guest's, %d, present longest",
			leader, stage.guests[0].DeviceID)
	}
	if line := logLineWith(stage.logs.String(), "why=leaderGone"); !strings.Contains(line,
		"leaderSeniority=2026-10-05T20:00:01.000000Z leaderPresentMs=91000") ||
		!strings.Contains(line, "goneBecause=closed goneLeaderWasHost=true") {
		t.Errorf("the lead's passing logged as %q; want the new leader's seniority and why", line)
	}
	stage.hub.Detach(stage.guests[0], GoneClosed)
	stage.hub.Detach(stage.guests[1], GoneClosed)
	if leader := rosterLeader(watchBox); leader != sameFirst.DeviceID {
		t.Errorf("device %d leads; want %d, the lower id of two arrived the same microsecond",
			leader, sameFirst.DeviceID)
	}
	stage.hub.Detach(sameFirst, GoneClosed)
	if leader := rosterLeader(watchBox); leader != sameSecond.DeviceID {
		t.Errorf("device %d leads; want %d, the one left", leader, sameSecond.DeviceID)
	}
}

// A device that comes back, its page shown again after its browser hid it or its connection made
// again, takes the moment it came back as its seniority: the most senior no longer (D372 §2).
func TestHub_ADeviceThatComesBackIsPresentOnlySinceItCameBack(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 3)
	stage.hide(t, stage.guests[0], true)
	stage.clock.Advance(time.Second)
	stage.hide(t, stage.guests[0], false)
	stage.hub.Detach(stage.guests[1], GoneClosed)
	again, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	stage.hub.Detach(stage.host, GoneClosed)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	if leader := rosterLeader(watchBox); leader != stage.guests[2].DeviceID {
		t.Errorf("device %d leads; want the third guest's, %d: the first's page came back after it, "+
			"the second's connection too", leader, stage.guests[2].DeviceID)
	}
	stage.hub.Detach(stage.guests[2], GoneClosed)
	if leader := rosterLeader(watchBox); leader != stage.guests[0].DeviceID {
		t.Errorf("device %d leads; want the first guest's, %d, back before %d", leader,
			stage.guests[0].DeviceID, again.DeviceID)
	}
}

// The host's player coming back while a guest's leads a film follows first, and takes the lead back
// only once it plays in step, within the drift a follower leaves alone, so the group never jumps for
// its return; the log says it came back in step (D372 §3).
func TestHub_TheHostsPlayerComingBackFollowsUntilInStepThenLeads(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	stage.hub.Detach(stage.host, GoneClosed)
	stage.clock.Advance(10 * time.Second)
	back, backBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	if leader := rosterLeader(backBox); leader != stage.guests[0].DeviceID {
		t.Fatalf("device %d leads as the host's player comes back; want the guest's, %d, until it is in step",
			leader, stage.guests[0].DeviceID)
	}
	seq := backBox.last(MessageGroup).Seq
	at := stage.groupPositionMs(stage.opened.SessionID)
	stage.report(t, back, at, false, true) // loading
	stage.report(t, back, at-2_000, true, false)
	if leader := rosterLeader(backBox); leader == back.DeviceID {
		t.Error("the host's player leads playing 2 s behind the group; want it to follow until in step")
	}
	stage.report(t, back, at-300, true, false) // 280 ms behind, its round trip 40 ms: in step
	if leader := rosterLeader(backBox); leader != back.DeviceID {
		t.Errorf("device %d leads, the host's player playing in step; want it, %d", leader, back.DeviceID)
	}
	if backBox.last(MessageGroup).Seq != seq {
		t.Errorf("the group changed as the host's player took the lead back: %+v", backBox.last(MessageGroup).Group)
	}
	if line := logLineWith(stage.logs.String(), "why=hostInStep"); !strings.Contains(line, "offGroupMs=280") {
		t.Errorf("the lead's return logged as %q; want how far off the group it was", line)
	}
}

// A guest's player leading carries the film's clock and nothing else: its state places the group,
// while the commands its link withholds are refused it as before (D372 §4).
func TestHub_AGuestsLeadingPlayerCarriesTheClockOnly(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	guest := stage.guests[0]
	stage.hub.Detach(stage.host, GoneClosed)
	watchAttachment, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.clock.Advance(5 * time.Second)
	stage.report(t, guest, 3_000, true, false)
	if at := stage.groupPositionMs(stage.opened.SessionID); at != 3_020 {
		t.Errorf("the group is at %d ms; want 3.02 s, where the guest's leading player is", at)
	}
	for _, command := range []CommandKind{CommandPause, CommandSeek, CommandStop, CommandChangeItem} {
		box := &inbox{}
		attachment, err := stage.hub.Attach(stage.guestCredential(t, guest), RolePanel, false, nil, testTuning,
			box.send)
		if err != nil {
			t.Fatal(err)
		}
		stage.command(t, attachment, command, 8)
		if refused := box.last(MessageRejected); refused.Code != codeForbidden {
			t.Errorf("the leading guest's %s: %+v; want refused, forbidden", command, refused)
		}
	}
	if group := stage.pongGroup(t, watchAttachment, watchBox); group.State != GroupPlaying || group.FileID != 7 {
		t.Errorf("the group after the guest's refused commands: %+v; want film 7 playing on", group)
	}
}

// logLineWith is the first line logged that holds fragment, or "".
func logLineWith(logged, fragment string) string {
	for line := range strings.Lines(logged) {
		if strings.Contains(line, fragment) {
			return line
		}
	}
	return ""
}

// guestCredential is a credential of the guest whose device attachment is, for another device.
func (stage sessionStage) guestCredential(t *testing.T, attachment Attachment) string {
	t.Helper()
	stage.hub.mutex.Lock()
	defer stage.hub.mutex.Unlock()
	made := madeCredential{credential: fmt.Sprintf("another-%d", attachment.DeviceID)}
	stage.hub.sessions[attachment.SessionID].members[attachment.MemberID].addCredential(made)
	return made.credential
}

// A leader whose page its browser hides, or whose connection goes silent, is gone, and the lead
// passes at once; a leader's stall is not: the group stands for it (D372 §5). A device whose page is
// hidden, or that plays nothing, cannot lead (§6).
func TestHub_ALeaderHiddenOrSilentIsGoneAndAStalledOneIsNot(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 3)
	stage.hub.Detach(stage.host, GoneClosed)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	first, second, third := stage.guests[0], stage.guests[1], stage.guests[2]
	stage.report(t, first, stage.groupPositionMs(stage.opened.SessionID), true, true)
	if leader := rosterLeader(watchBox); leader != first.DeviceID {
		t.Errorf("device %d leads as the leader stalls; want the stalled leader still, %d", leader, first.DeviceID)
	}
	if group := watchBox.last(MessageGroup).Group; group.State != GroupWaiting {
		t.Errorf("the group as its leader stalls: %+v; want Waiting for it", group)
	}
	stage.hide(t, second, true)
	stage.hide(t, first, true)
	if leader := rosterLeader(watchBox); leader != third.DeviceID {
		t.Errorf("device %d leads, the leader's page hidden and the next's too; want %d", leader, third.DeviceID)
	}
	if line := logLineWith(stage.logs.String(), "goneBecause=hidden"); !strings.Contains(line,
		fmt.Sprintf("leaderDeviceId=%d", third.DeviceID)) {
		t.Errorf("the hidden leader's passing logged as %q", line)
	}
	stage.hide(t, second, false)
	stage.hub.Detach(third, GoneSilent)
	if leader := rosterLeader(watchBox); leader != second.DeviceID {
		t.Errorf("device %d leads, the leader silent; want the one shown again, %d", leader, second.DeviceID)
	}
	if line := logLineWith(stage.logs.String(), "goneBecause=silent"); line == "" {
		t.Error("no log line says the leader went silent")
	}
}

// With everyone gone, no player left who can lead, a Watch Together group pauses where it stands,
// and the session ends by its idle end as before (D372 §7, D346); a cast, a session of one, is left
// to its receiver's return.
func TestHub_WithEveryoneGoneTheGroupPausesWhereItStandsThenEndsIdle(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.clock.Advance(4 * time.Second)
	at := stage.groupPositionMs(stage.opened.SessionID)
	stage.hide(t, stage.guests[0], true)
	stage.hub.Detach(stage.host, GoneClosed)
	group := watchBox.last(MessageGroup).Group
	if group.State != GroupPaused || group.Event != EventEveryoneLeft || group.PositionMs != at {
		t.Errorf("the group as everyone went: %+v; want Paused where it stood, %d ms, everyone gone", group, at)
	}
	stage.clock.Advance(StartingIdleEnd)
	if ended := stage.hub.EndStale(startingLifetimes()); len(ended) != 1 {
		t.Errorf("everyone gone for the idle end: EndStale ended %v; want the session", ended)
	}

	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	receiver, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	_, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.report(t, receiver, 0, true, false)
	stage.hub.Detach(receiver, GoneClosed)
	if group := remoteBox.last(MessageGroup).Group; group.State != GroupPlaying {
		t.Errorf("a cast as its receiver went: %+v; want Playing on, for its return", group)
	}
}

// A guest's leading player that undoes its person's move their link withholds says so, and the
// server logs it as a note it knows (D372 §4).
func TestHub_LogsALeadingGuestsUndoneMove(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	stage.hub.Detach(stage.host, GoneClosed)
	guestBox := &inbox{}
	guest, err := stage.hub.Attach(stage.guestCredential(t, stage.guests[0]), RoleVideo, true, nil, testTuning,
		guestBox.send)
	if err != nil {
		t.Fatal(err)
	}
	stage.hub.Handle(t.Context(), guest, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSought, PositionMs: 600_000, ToMs: 60_000, Why: "undone",
	}})
	if refused := guestBox.count(MessageRejected); refused != 0 {
		t.Errorf("the undone move's note was refused %d times; want it logged", refused)
	}
	if line := logLineWith(stage.logs.String(), "why=undone"); line == "" {
		t.Errorf("no log line says the move was undone:\n%s", stage.logs)
	}
}

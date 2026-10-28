// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"testing"
)

// A cast is a session of one that nothing else finds: it has no links, is not among the person's
// sessions, and their party is opened and kept beside it (plans/cast-to-device.md §3, §8 step 4).
func TestCast_IsASessionOfOneNoOneElseFindsAndAPartyStaysBesideIt(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	if cast.SessionID == stage.opened.SessionID {
		t.Fatalf("the cast is the party, session %d", cast.SessionID)
	}
	mine, err := stage.hub.Mine(stage.owner.UserID)
	if err != nil || len(mine) != 1 || mine[0].SessionID != stage.opened.SessionID {
		t.Errorf("the person's sessions are %+v, %v; want their party alone", mine, err)
	}
	again, err := stage.hub.Open(stage.owner)
	if err != nil || again.SessionID == cast.SessionID {
		t.Errorf("starting a party again gave session %d, %v; want a new party, not the cast", again.SessionID, err)
	}
	if _, _, err = stage.hub.CastPosition(cast.SessionID, stage.owner.UserID); err != nil {
		t.Errorf("the cast after a party was started again: %v; want it on, beside it (D372 §8)", err)
	}
	_, err = stage.hub.AddLink(cast.SessionID, stage.owner.UserID, "someone", Rights{})
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("a link in the cast was %v, want ErrNoSession", err)
	}
	if _, err := stage.hub.DetailsOf(cast.SessionID, stage.owner.UserID); !errors.Is(err, ErrNoSession) {
		t.Errorf("the cast's details were %v, want ErrNoSession", err)
	}
	if err := stage.hub.End(cast.SessionID, stage.owner.UserID); !errors.Is(err, ErrNoSession) {
		t.Errorf("ending the cast as a party was %v, want ErrNoSession", err)
	}
	if _, err := stage.hub.CastCredential(stage.opened.SessionID, stage.owner.UserID); !errors.Is(err, ErrNoSession) {
		t.Errorf("a cast's credential for the party was %v, want ErrNoSession", err)
	}
	if _, err := stage.hub.CastCredential(cast.SessionID, 99); !errors.Is(err, ErrNoSession) {
		t.Errorf("a cast's credential for someone else was %v, want ErrNoSession", err)
	}
}

// The receiver leads and the remote commands it: the remote's pause pauses the group the receiver
// follows, and ending the cast tells both why and admits neither again.
func TestCast_TheRemoteCommandsTheReceiverAndEndingTellsBothWhy(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	receiver, receiverBox := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	remote, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.ready(t, receiver, false)
	if state := receiverBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Fatalf("with its one player ready the cast is %s, want Playing", state)
	}
	stage.command(t, remote, CommandPause, 0)
	if state := receiverBox.last(MessageGroup).Group.State; state != GroupPaused {
		t.Errorf("after the remote's pause the receiver's group is %s, want Paused", state)
	}
	if group := remoteBox.last(MessageGroup).Group; group.State != GroupPaused || group.FileID != 12 {
		t.Errorf("the remote sees %+v, want file 12 paused", group)
	}
	if err = stage.hub.EndCast(cast.SessionID, stage.owner.UserID, CastTakenBack); err != nil {
		t.Fatal(err)
	}
	for name, box := range map[string]*inbox{"receiver": receiverBox, "remote": remoteBox} {
		if reason := box.last(MessageEnded).Reason; reason != CastTakenBack {
			t.Errorf("the %s was told the cast ended for %q, want %q", name, reason, CastTakenBack)
		}
	}
	_, err = stage.hub.Attach(cast.SenderCredential, RolePanel, false, nil, testTuning, (&inbox{}).send)
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("the ended cast's credential attached: %v", err)
	}
}

// A device of the person coming back to their cast has a credential of its own, which cuts off none.
func TestCast_ADeviceComingBackHasACredentialOfItsOwn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	again, err := stage.hub.CastCredential(cast.SessionID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	for name, credential := range map[string]string{
		"sender's": cast.SenderCredential, "receiver's": cast.ReceiverCredential, "new": again,
	} {
		if sessionID, _, isMember := stage.hub.MemberOf(credential); !isMember || sessionID != cast.SessionID {
			t.Errorf("the %s credential is of session %d (%v), want the cast, %d",
				name, sessionID, isMember, cast.SessionID)
		}
	}
}

// A cast made ready between videos plays nothing, every device told, so the next video, even the one
// before, is put on afresh rather than followed where the group had gone on to (D364 §5).
func TestCast_ReadyPlaysNothingSoTheNextIsPutOnAfresh(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	receiver, receiverBox := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	_, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.ready(t, receiver, false)
	if err = stage.hub.CastReady(cast.SessionID, stage.owner.UserID); err != nil {
		t.Fatal(err)
	}
	for name, box := range map[string]*inbox{"receiver": receiverBox, "remote": remoteBox} {
		if group := box.last(MessageGroup).Group; group.State != GroupIdle || group.FileID != 0 {
			t.Errorf("the %s sees %+v once ready, want nothing on", name, group)
		}
	}
	if err := stage.hub.CastReady(cast.SessionID, 99); !errors.Is(err, ErrNoSession) {
		t.Errorf("someone else made the cast ready: %v; want ErrNoSession", err)
	}
	if err := stage.hub.CastReady(stage.opened.SessionID, stage.owner.UserID); !errors.Is(err, ErrNoSession) {
		t.Errorf("the party was made ready as a cast: %v; want ErrNoSession", err)
	}
}

// A cast's receiver is given a credential of the host's own for the Watch Together session its person
// hosts, to play it as their player; a person hosting none, or only a cast, is given none (D364 §11).
func TestCast_AHostsReceiverHasACredentialForTheirSession(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	if _, err := stage.hub.OpenCast(Person{UserID: 77, Name: "Caster"}); err != nil {
		t.Fatal(err)
	}
	if credential, _ := stage.hub.TogetherPart(77); credential != "" {
		t.Errorf("a person with only a cast was given %q; want none", credential)
	}
	credential, isHost := stage.hub.TogetherPart(stage.owner.UserID)
	sessionID, memberID, isMember := stage.hub.MemberOf(credential)
	if !isMember || !isHost || sessionID != stage.opened.SessionID ||
		stage.hub.hostOf(sessionID) != stage.owner.UserID {
		t.Fatalf("the host's credential is of session %d, member %d (%v, host %v); want the party's host",
			sessionID, memberID, isMember, isHost)
	}
}

// A guest who joined signed in and casts has their receiver given a credential of their own
// membership, to take their part in the session as just another follower; it never leads, and the
// group never waits for it (D371 §4).
func TestCast_AGuestsReceiverTakesTheirPartAsAFollower(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "my partner", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	guestPerson := Person{UserID: 42, Name: "Anna"}
	joined, err := stage.hub.Join(made.Secret, "Anna", &guestPerson, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	credential, isHost := stage.hub.TogetherPart(guestPerson.UserID)
	if sessionID, memberID, isMember := stage.hub.MemberOf(credential); !isMember || isHost ||
		sessionID != stage.opened.SessionID || memberID != joined.MemberID {
		t.Fatalf("the guest's receiver was given session %d, member %d (%v, host %v); want the guest's own part",
			sessionID, memberID, isMember, isHost)
	}
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	receiver, _ := stage.attach(t, credential, RoleVideo)
	seq := hostBox.last(MessageGroup).Seq
	stage.report(t, receiver, 0, false, true)
	for _, each := range hostBox.last(MessageRoster).Roster {
		for _, attached := range each.Devices {
			if attached.ID == receiver.DeviceID && attached.IsLeader {
				t.Error("the guest's receiver leads the session; want it a follower")
			}
		}
	}
	if hostBox.last(MessageGroup).Seq != seq {
		t.Errorf("the guest's receiver loading changed the group: %+v", hostBox.last(MessageGroup).Group)
	}
}

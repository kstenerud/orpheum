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
	if err != nil || again.SessionID != stage.opened.SessionID {
		t.Errorf("opening again gave session %d, %v; want the party, %d", again.SessionID, err, stage.opened.SessionID)
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
	receiver, receiverBox := stage.attach(t, cast.ReceiverCredential, RolePlayback)
	remote, remoteBox := stage.attach(t, cast.SenderCredential, RoleVoice)
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
	_, err = stage.hub.Attach(cast.SenderCredential, RoleVoice, false, nil, testTuning, (&inbox{}).send)
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

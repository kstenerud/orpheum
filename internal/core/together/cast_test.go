// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"reflect"
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

// A device saying hello with a credential of a cast that just ended is told why it ended, as its
// devices attached then were, for endedCastKept; after it, and for a credential no cast had, only
// that there is no session (the browser tier, 2026-10-29: a remote opened as the receiver took
// control back said only that the cast was over).
func TestCast_AHelloJustAfterTheEndIsToldWhyForAMinute(t *testing.T) {
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
	if err = stage.hub.EndCast(cast.SessionID, stage.owner.UserID, CastTakenBack); err != nil {
		t.Fatal(err)
	}
	attach := func(credential string) error {
		_, err := stage.hub.Attach(credential, RolePanel, false, nil, testTuning, (&inbox{}).send)
		return err
	}
	isOnlyNoSession := func(err error) bool {
		ended, isCastEnded := errors.AsType[*CastEndedError](err)
		return !isCastEnded && ended == nil && errors.Is(err, ErrNoSession)
	}
	for _, credential := range []string{cast.SenderCredential, again} {
		err := attach(credential)
		if ended, isCastEnded := errors.AsType[*CastEndedError](err); !isCastEnded || ended.Reason != CastTakenBack ||
			!errors.Is(err, ErrNoSession) {
			t.Errorf("a hello just after the end = %v, want the cast ended for %q, no session", err, CastTakenBack)
		}
	}
	if err := attach("never a cast's"); !isOnlyNoSession(err) {
		t.Errorf("a hello with no cast's credential = %v, want only no session", err)
	}
	stage.clock.Advance(endedCastKept)
	if err := attach(again); !isOnlyNoSession(err) {
		t.Errorf("a hello a minute after the end = %v, want only no session", err)
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
	if credential, _ := stage.hub.TogetherPart(77, ""); credential != "" {
		t.Errorf("a person with only a cast was given %q; want none", credential)
	}
	credential, isHost := stage.hub.TogetherPart(stage.owner.UserID, "")
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
	credential, isHost := stage.hub.TogetherPart(guestPerson.UserID, "")
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

// A person who hosts a session and joined it as a guest from another device casts from that device as
// its guest: the membership the sending page holds is the part its receiver takes, a follower of the
// group, never their host's player (VQ193); without it, the session they host is the part, as before.
func TestCast_AHostsGuestDeviceCastsAsTheirGuest(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "my phone", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	owner := stage.owner
	joined, err := stage.hub.Join(made.Secret, "Phone", &owner, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	credential, isHost := stage.hub.TogetherPart(owner.UserID, joined.MembershipCredential)
	if _, memberID, isMember := stage.hub.MemberOf(credential); !isMember || isHost || memberID != joined.MemberID {
		t.Fatalf("the guest's device's receiver was given member %d (%v, host %v); want the guest's part %d",
			memberID, isMember, isHost, joined.MemberID)
	}
	if _, isHost := stage.hub.TogetherPart(owner.UserID, ""); !isHost {
		t.Error("a cast from a page holding no guest's membership took a guest's part; want the host's")
	}
}

// A remote's choice of its film's sound, the dialogue boost, levelling or the audio track, reaches the
// person's own devices that play the film, a cast's receiver, which carries it out as its own press;
// it reaches no other member's, nor the remote itself, and a choice no remote may make is refused. A
// Fire TV's own controls are hidden while it is controlled, so its sound could only be changed after
// taking control back (the owner, VQ116's run).
func TestCast_TheRemoteChoosesTheReceiversSound(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, playerBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	remote, remoteBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	guest := stage.join(t)
	_, guestBox := stage.attach(t, guest.MembershipCredential, RoleVideo)
	for _, choice := range []SoundChoice{
		{Kind: SoundBoost, IsOn: true},
		{Kind: SoundLevelling},
		{Kind: SoundAudioTrack, StreamID: 3},
	} {
		stage.hub.Handle(t.Context(), remote, Inbound{Type: MessageSound, Sound: &choice})
		relayed := playerBox.last(MessageSound)
		if relayed.Sound == nil || *relayed.Sound != choice || relayed.FromDeviceID != remote.DeviceID {
			t.Errorf("the player was relayed %+v (%+v); want %+v from the remote", relayed, relayed.Sound, choice)
		}
	}
	if count := guestBox.count(MessageSound); count != 0 {
		t.Errorf("another member's player was told %d sound choices; want none", count)
	}
	if count := remoteBox.count(MessageSound); count != 0 {
		t.Errorf("the remote was told %d of its own choices; want none", count)
	}
	for _, refused := range []*SoundChoice{nil, {Kind: "louder"}, {Kind: SoundAudioTrack}} {
		stage.hub.Handle(t.Context(), remote, Inbound{Type: MessageSound, Sound: refused})
	}
	if count := playerBox.count(MessageSound); count != 3 {
		t.Errorf("the player was told %d sound choices; want the 3 a remote may make", count)
	}
	if count := remoteBox.count(MessageRejected); count != 3 {
		t.Errorf("the remote's unknown choices were refused %d times; want 3", count)
	}
}

// A player's state may say how its film sounds, which the roster shows its remote; a sound past the
// bounds, or of a kind no player says, is not taken, the last known kept.
func TestHub_TheRosterSaysHowEachPlayersFilmSounds(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	receiver, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	_, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	soundInRoster := func() *PlaybackSound {
		for _, each := range remoteBox.last(MessageRoster).Roster {
			for _, attached := range each.Devices {
				if attached.ID == receiver.DeviceID && attached.Playback != nil {
					return attached.Playback.Sound
				}
			}
		}
		return nil
	}
	isOn := true
	sound := PlaybackSound{
		IsBoostOn: &isOn, IsLevellingOn: true, AudioStreamID: 2,
		AudioTracks: []SoundTrack{
			{StreamID: 1, Language: "en", Kind: "plain"},
			{StreamID: 2, Language: "de", ContainerTitle: "Director", Kind: "commentary"},
		},
	}
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.hub.Handle(t.Context(), receiver, Inbound{
		Type: MessageState, Playing: true, FileID: 12,
		Playback: &Playback{Delivery: DeliveryConverted, Sound: &sound},
	})
	if got := soundInRoster(); got == nil || !reflect.DeepEqual(*got, sound) {
		t.Fatalf("the remote's roster says the receiver sounds %+v; want %+v", got, sound)
	}
	stage.hub.Handle(t.Context(), receiver, Inbound{
		Type: MessageState, Playing: true, FileID: 12,
		Playback: &Playback{Delivery: DeliveryConverted, Sound: &PlaybackSound{
			AudioTracks: []SoundTrack{{StreamID: 1, Kind: "whispered"}},
		}},
	})
	if got := soundInRoster(); got == nil || !reflect.DeepEqual(*got, sound) {
		t.Errorf("after a track of a kind no player says, the receiver sounds %+v; want the last known", got)
	}
}

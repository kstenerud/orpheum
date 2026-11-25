// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// alone is a person in no Watch Together session, whose cast is a session of one (D88).
var alone = Person{UserID: 77, Name: "Caster"}

// A cast by a person in no Watch Together session is a session of one that nothing else finds: it has
// no links and is not among the person's sessions (plans/cast-to-device.md §3). A party its person
// starts then takes the cast in as their part, no second session beside it (D555 §2).
func TestCast_AloneIsASessionOfOneAndAPartyTakesItIn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	if cast.TogetherSessionID != 0 {
		t.Fatalf("a cast alone is in session %d; want its own", cast.TogetherSessionID)
	}
	if mine, mineErr := stage.hub.Mine(alone.UserID); mineErr != nil || len(mine) != 0 {
		t.Errorf("the person's sessions are %+v; want none, the cast found by no one", mine)
	}
	stage.isFoundAsNoSession(t, cast.CastID)
	party, err := stage.hub.Open(alone)
	if err != nil {
		t.Fatal(err)
	}
	again, err := stage.hub.CastCredential(cast.CastID, alone.UserID)
	if err != nil {
		t.Fatalf("the cast after its person started a party: %v; want it on, in the party", err)
	}
	if sessionID, _, isMember := stage.hub.MemberOf(again); !isMember || sessionID != party.SessionID {
		t.Errorf("the cast's credential is of session %d (%v); want the party, %d", sessionID, isMember,
			party.SessionID)
	}
	if sessionID, isHost := stage.hub.CastPart(cast.CastID, alone.UserID); sessionID != party.SessionID || !isHost {
		t.Errorf("the cast is the part in session %d (host %v); want the party's host's", sessionID, isHost)
	}
}

// isFoundAsNoSession checks the cast castID, alone's, is no party: no link is made in it, it has no
// details and is not ended as one, and no cast's credential is had for the party or for someone else.
func (stage sessionStage) isFoundAsNoSession(t *testing.T, castID int64) {
	t.Helper()
	_, err := stage.hub.AddLink(castID, alone.UserID, "someone", Rights{})
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("a link in the cast was %v, want ErrNoSession", err)
	}
	if _, err = stage.hub.DetailsOf(castID, alone.UserID); !errors.Is(err, ErrNoSession) {
		t.Errorf("the cast's details were %v, want ErrNoSession", err)
	}
	if err = stage.hub.End(castID, alone.UserID); !errors.Is(err, ErrNoSession) {
		t.Errorf("ending the cast as a party was %v, want ErrNoSession", err)
	}
	_, err = stage.hub.CastCredential(stage.opened.SessionID, stage.owner.UserID)
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("a cast's credential for the party was %v, want ErrNoSession", err)
	}
	if _, err = stage.hub.CastCredential(castID, 99); !errors.Is(err, ErrNoSession) {
		t.Errorf("a cast's credential for someone else was %v, want ErrNoSession", err)
	}
}

// The receiver leads and the remote commands it: the remote's pause pauses the group the receiver
// follows, and ending the cast tells both why and admits neither again.
func TestCast_TheRemoteCommandsTheReceiverAndEndingTellsBothWhy(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner, "")
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
	if err = stage.hub.EndCast(cast.CastID, stage.owner.UserID, CastTakenBack); err != nil {
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
	cast, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := stage.hub.CastCredential(cast.CastID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.EndCast(cast.CastID, stage.owner.UserID, CastTakenBack); err != nil {
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
	cast, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := stage.hub.CastCredential(cast.CastID, alone.UserID)
	if err != nil {
		t.Fatal(err)
	}
	for name, credential := range map[string]string{
		"sender's": cast.SenderCredential, "receiver's": cast.ReceiverCredential, "new": again,
	} {
		if sessionID, _, isMember := stage.hub.MemberOf(credential); !isMember || sessionID != cast.CastID {
			t.Errorf("the %s credential is of session %d (%v), want the cast, %d",
				name, sessionID, isMember, cast.CastID)
		}
	}
}

// A cast made ready between videos plays nothing, every device told, so the next video, even the one
// before, is put on afresh rather than followed where the group had gone on to (D364 §5). Nothing
// on is no title either: the title its film was put on as goes with it, so the next file put on is
// not taken as that title (B0 question 20).
func TestCast_ReadyPlaysNothingSoTheNextIsPutOnAfresh(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	receiver, receiverBox := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	_, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	const titleOf12 = 34
	stage.hub.Handle(t.Context(), receiver, Inbound{
		Type: MessageCommand, Command: CommandChangeItem, FileID: 12, ItemID: titleOf12,
	})
	if group := receiverBox.last(MessageGroup).Group; group.FileID != 12 || group.ItemID != titleOf12 {
		t.Fatalf("the cast put on %+v; want file 12 as title %d", group, titleOf12)
	}
	stage.ready(t, receiver, false)
	if err = stage.hub.CastReady(cast.CastID, alone.UserID); err != nil {
		t.Fatal(err)
	}
	for name, box := range map[string]*inbox{"receiver": receiverBox, "remote": remoteBox} {
		if group := box.last(MessageGroup).Group; group.State != GroupIdle || group.FileID != 0 || group.ItemID != 0 {
			t.Errorf("the %s sees %+v once ready, want nothing on, as no title", name, group)
		}
	}
	if played := stage.hub.playedOf(cast.CastID); played != (Played{}) {
		t.Errorf("the cast once ready plays %+v; want nothing, as no title", played)
	}
	if err := stage.hub.CastReady(cast.CastID, 99); !errors.Is(err, ErrNoSession) {
		t.Errorf("someone else made the cast ready: %v; want ErrNoSession", err)
	}
	if err := stage.hub.CastReady(stage.opened.SessionID, alone.UserID); !errors.Is(err, ErrNoSession) {
		t.Errorf("the party was made ready as a cast: %v; want ErrNoSession", err)
	}
}

// A cast's receiver is given a credential of the host's own for the Watch Together session its person
// hosts, to play it as their player; a person hosting none, or only a cast, is given none (D364 §11).
func TestCast_AHostsReceiverHasACredentialForTheirSession(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	if _, err := stage.hub.OpenCast(Person{UserID: 77, Name: "Caster"}, ""); err != nil {
		t.Fatal(err)
	}
	if credential, _, _ := stage.castPart(t, 77, ""); credential != "" {
		t.Errorf("a person with only a cast was given %q; want none", credential)
	}
	credential, isHost, _ := stage.castPart(t, stage.owner.UserID, "")
	sessionID, memberID, isMember := stage.hub.MemberOf(credential)
	if !isMember || !isHost || sessionID != stage.opened.SessionID ||
		stage.hub.hostOf(sessionID) != stage.owner.UserID {
		t.Fatalf("the host's credential is of session %d, member %d (%v, host %v); want the party's host",
			sessionID, memberID, isMember, isHost)
	}
}

// Every way a Watch Together session ends is told, once the hub's lock is let go, with the session
// the casts that take a part in it end with (D389 §1): its host starting another, its host joining
// another, its host's end and its idle end. A cast's own end is no Watch Together session's.
func TestHub_EveryWayASessionEndsIsToldWithThePartsCastsEndWith(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	var told []int64
	stage.hub.OnSessionEnded(func(sessionID int64) {
		if _, err := stage.hub.Mine(stage.owner.UserID); err != nil { // the lock is let go, or this waits
			t.Error(err)
		}
		told = append(told, sessionID)
	})
	_, _, partIn := stage.castPart(t, stage.owner.UserID, "")
	if partIn != stage.opened.SessionID {
		t.Fatalf("the host's part is in session %d; want %d", partIn, stage.opened.SessionID)
	}
	cast, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.EndCast(cast.CastID, stage.owner.UserID, CastStopped); err != nil {
		t.Fatal(err)
	}
	again, err := stage.hub.Open(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	friend := Person{UserID: 2, Name: "friend"}
	friends, err := stage.hub.Open(friend)
	if err != nil {
		t.Fatal(err)
	}
	made, err := stage.hub.AddLink(friends.SessionID, friend.UserID, "the owner", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(made.Secret, "owner", &stage.owner, Leaving{ShouldLeave: true}); err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.End(friends.SessionID, friend.UserID); err != nil {
		t.Fatal(err)
	}
	idle, err := stage.hub.Open(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	stage.clock.Advance(StartingIdleEnd)
	stage.hub.EndStale(Lifetimes{Link: StartingLinkLifetime, Idle: StartingIdleEnd})
	want := []int64{stage.opened.SessionID, again.SessionID, friends.SessionID, idle.SessionID}
	if !reflect.DeepEqual(told, want) {
		t.Errorf("the sessions told ended = %v; want %v", told, want)
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
	credential, isHost, _ := stage.castPart(t, guestPerson.UserID, "")
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
	credential, isHost, _ := stage.castPart(t, owner.UserID, joined.MembershipCredential)
	if _, memberID, isMember := stage.hub.MemberOf(credential); !isMember || isHost || memberID != joined.MemberID {
		t.Fatalf("the guest's device's receiver was given member %d (%v, host %v); want the guest's part %d",
			memberID, isMember, isHost, joined.MemberID)
	}
	if _, isHost, _ := stage.castPart(t, owner.UserID, ""); !isHost {
		t.Error("a cast from a page holding no guest's membership took a guest's part; want the host's")
	}
}

// A remote's choice of its film's sound, the dialogue boost, levelling, the audio track, its timing or
// the film's level and mute on the receiver (D440 §2), reaches the
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
		{Kind: SoundOffset, Ms: -200}, // the owner, 2026-11-06: "Put a control in ... cast"
		{Kind: SoundChannelLevel, Channel: ChannelFilm, Level: 0.4},
		{Kind: SoundChannelLevel, Channel: ChannelFilm, Level: 1, IsMuted: true},
		{Kind: SoundFileOffset, Ms: 300},                     // the file's own (D537)
		{Kind: SoundFileOffset, Ms: -150, IsEveryones: true}, // made every person's
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
	refusedChoices := []*SoundChoice{
		nil,
		{Kind: "louder"},
		{Kind: SoundAudioTrack},
		{Kind: SoundOffset, Ms: 2050},
		{Kind: SoundOffset, Ms: -2050},
		{Kind: SoundChannelLevel, Level: 0.5},
		{Kind: SoundChannelLevel, Channel: "voices", Level: 0.5},
		{Kind: SoundChannelLevel, Channel: ChannelFilm, Level: 1.5},
		{Kind: SoundChannelLevel, Channel: ChannelFilm, Level: -0.1},
		{Kind: SoundFileOffset, Ms: 2050},
		{Kind: SoundOffset, Ms: 100, IsEveryones: true},
	}
	for _, refused := range refusedChoices {
		stage.hub.Handle(t.Context(), remote, Inbound{Type: MessageSound, Sound: refused})
	}
	if count := playerBox.count(MessageSound); count != 8 {
		t.Errorf("the player was told %d sound choices; want the 8 a remote may make", count)
	}
	if count := remoteBox.count(MessageRejected); count != len(refusedChoices) {
		t.Errorf("the remote's unknown choices were refused %d times; want %d", count, len(refusedChoices))
	}
}

// While its person hosts Watch Together, a cast's receiver leads that session and is only a panel in
// the cast's (D364 §11); it says its playback there alone, and its remote's sound choices reach it
// there (the owner, 2026-11-06: "It shouldn't be watch together's business at all which TV has what
// offset or playback characteristics").
func TestCast_TheRemoteWorksTheReceiverInTheCastWhileItLeadsWatchTogether(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	receiver, receiverBox := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	remote, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	earlier := int64(-400)
	said := &Playback{Delivery: DeliveryConverted, QualityKbps: 4000, Sound: &PlaybackSound{SoundOffsetMs: &earlier}}
	stage.hub.Handle(t.Context(), receiver, Inbound{Type: MessagePlayback, Playback: said})
	var shown *Playback
	for _, each := range remoteBox.last(MessageRoster).Roster {
		for _, attached := range each.Devices {
			if attached.ID == receiver.DeviceID {
				shown = attached.Playback
			}
		}
	}
	if shown == nil || shown.QualityKbps != 4000 || shown.Sound == nil || shown.Sound.SoundOffsetMs == nil ||
		*shown.Sound.SoundOffsetMs != earlier {
		t.Errorf("the remote's roster shows the receiver playing %+v; want %+v", shown, said)
	}
	choice := SoundChoice{Kind: SoundOffset, Ms: -450}
	stage.hub.Handle(t.Context(), remote, Inbound{Type: MessageSound, Sound: &choice})
	if relayed := receiverBox.last(MessageSound); relayed.Sound == nil || *relayed.Sound != choice {
		t.Errorf("the receiver was relayed %+v; want %+v", relayed.Sound, choice)
	}
}

// A player's state may say how its film sounds, which the roster shows its remote; a sound past the
// bounds, or of a kind no player says, is not taken, the last known kept.
func TestHub_TheRosterSaysHowEachPlayersFilmSounds(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner, "")
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
	earlier, pastTheBound := int64(-200), int64(2050)
	sound := PlaybackSound{
		IsBoostOn: &isOn, IsLevellingOn: true, AudioStreamID: 2, SoundOffsetMs: &earlier,
		AudioTracks: []SoundTrack{
			{StreamID: 1, Language: "en", Kind: "plain"},
			{StreamID: 2, Language: "de", ContainerTitle: "Director", Kind: "commentary"},
		},
		Channels:        []SoundChannel{{Channel: ChannelFilm, Level: 0.35, IsMuted: true}},
		FileSoundOffset: &FileSoundOffset{EveryonesMs: 0, OwnMs: &earlier},
	}
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.hub.Handle(t.Context(), receiver, Inbound{
		Type: MessageState, IsPlaying: true, FileID: 12,
		Playback: &Playback{Delivery: DeliveryConverted, Sound: &sound},
	})
	if got := soundInRoster(); got == nil || !reflect.DeepEqual(*got, sound) {
		t.Fatalf("the remote's roster says the receiver sounds %+v; want %+v", got, sound)
	}
	refused := map[string]PlaybackSound{
		"a track of a kind no player says": {AudioTracks: []SoundTrack{{StreamID: 1, Kind: "whispered"}}},
		"a sound offset past the bound":    {SoundOffsetMs: &pastTheBound},
		"a film louder than full":          {Channels: []SoundChannel{{Channel: ChannelFilm, Level: 1.2}}},
		"a channel no player mixes":        {Channels: []SoundChannel{{Channel: "louder", Level: 0.5}}},
		"a file's offset past the bound":   {FileSoundOffset: &FileSoundOffset{EveryonesMs: 2050}},
		"a file's own past the bound":      {FileSoundOffset: &FileSoundOffset{OwnMs: &pastTheBound}},
	}
	for name, each := range refused {
		stage.hub.Handle(t.Context(), receiver, Inbound{
			Type: MessageState, IsPlaying: true, FileID: 12,
			Playback: &Playback{Delivery: DeliveryConverted, Sound: &each},
		})
		if got := soundInRoster(); got == nil || !reflect.DeepEqual(*got, sound) {
			t.Errorf("after %s, the receiver sounds %+v; want the last known", name, got)
		}
	}
}

// How long nothing has played in a cast (D469 §3) is none while it plays, and counts from its pause,
// as from its making while nothing was put on.
func TestCast_IdleForCountsFromWhenItLastStoppedPlaying(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	userID := stage.owner.UserID
	stage.clock.Advance(time.Minute)
	if idle, idleErr := stage.hub.CastIdleFor(cast.CastID, userID); idleErr != nil || idle != time.Minute {
		t.Errorf("nothing put on for a minute: idle %v, %v; want a minute", idle, idleErr)
	}
	receiver, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	remote, _ := stage.attach(t, cast.SenderCredential, RolePanel)
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.ready(t, receiver, false)
	stage.clock.Advance(time.Hour)
	if idle, idleErr := stage.hub.CastIdleFor(cast.CastID, userID); idleErr != nil || idle != 0 {
		t.Errorf("playing for an hour: idle %v, %v; want none", idle, idleErr)
	}
	stage.command(t, remote, CommandPause, 0)
	stage.clock.Advance(2 * time.Minute)
	if idle, idleErr := stage.hub.CastIdleFor(cast.CastID, userID); idleErr != nil || idle != 2*time.Minute {
		t.Errorf("paused two minutes: idle %v, %v; want two minutes", idle, idleErr)
	}
	if _, idleErr := stage.hub.CastIdleFor(cast.CastID+1, userID); idleErr == nil {
		t.Error("a cast that is not there has an idle time")
	}
}

// castPart is a cast's receiver credential for the person userID's part in the Watch Together session
// they are in, the page holding held, whether they host it, and the session: a cast's castee is
// their part (D555 §2). "" and 0 when they are in none, the cast alone.
func (stage sessionStage) castPart(t *testing.T, userID int64, held string) (string, bool, int64) {
	t.Helper()
	cast, err := stage.hub.OpenCast(Person{UserID: userID, Name: "caster"}, held)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, isHost := stage.hub.CastPart(cast.CastID, userID)
	if sessionID == 0 {
		return "", false, 0
	}
	return cast.ReceiverCredential, isHost, sessionID
}

// A signed-in guest leaving the session takes their cast out with them, to play on alone (D555 §7):
// its part goes, the hub tells it moved once its lock is let go, and the cast is theirs still, in a
// session of one; a guest whose cast came by a link alone ends instead (TestGuestCastPart_*).
func TestHub_LeavingASessionMovesTheGuestsCastToPlayAlone(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	var moved []EndedCast
	stage.hub.OnCastMoved(func(each EndedCast) {
		if _, err := stage.hub.Mine(stage.owner.UserID); err != nil { // the lock is let go, or this waits
			t.Error(err)
		}
		moved = append(moved, each)
	})
	friend := Person{UserID: 2, Name: "friend"}
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a friend", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := stage.hub.Join(made.Secret, "friend", &friend, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	cast, err := stage.hub.OpenCast(friend, joined.MembershipCredential)
	if err != nil {
		t.Fatal(err)
	}
	if sessionID, _ := stage.hub.CastPart(cast.CastID, friend.UserID); sessionID != stage.opened.SessionID {
		t.Fatalf("the friend's cast is a part in session %d; want %d", sessionID, stage.opened.SessionID)
	}
	if err = stage.hub.Leave(joined.MembershipCredential); err != nil {
		t.Fatal(err)
	}
	if want := []EndedCast{{CastID: cast.CastID, UserID: friend.UserID}}; !reflect.DeepEqual(moved, want) {
		t.Errorf("the casts told moved = %v; want %v", moved, want)
	}
	if sessionID, _ := stage.hub.CastPart(cast.CastID, friend.UserID); sessionID != 0 {
		t.Errorf("the friend's cast is a part in session %d; want none, alone", sessionID)
	}
	if _, err = stage.hub.CastCredential(cast.CastID, friend.UserID); err != nil {
		t.Errorf("the friend's cast has no credential: %v; want it going on", err)
	}
}

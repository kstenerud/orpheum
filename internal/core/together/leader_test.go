// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/testutil"
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

// While none of the host's devices can lead, the player whose run started earliest leads, a guest's,
// by the moment the core first heard it ready, at microsecond resolution; ready the same microsecond,
// the lower id. The log says why and when the new leader's run started (D372 §1, D377 §2).
func TestHub_TheLeadPassesToThePlayerPresentLongest(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 2)
	stage.clock.Advance(time.Microsecond / 2) // the next two arrive in the same microsecond
	sameFirst, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	sameSecond, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	stage.ready(t, sameSecond, false)
	stage.ready(t, sameFirst, false)
	stage.clock.Advance(90 * time.Second)
	stage.hub.Detach(stage.host, GoneClosed)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	if leader := rosterLeader(watchBox); leader != stage.guests[0].DeviceID {
		t.Errorf("device %d leads, the host's player gone; want the first guest's, %d, present longest",
			leader, stage.guests[0].DeviceID)
	}
	if line := logLineWith(stage.logs.String(), "why=leaderGone"); !strings.Contains(line,
		"leaderRunSince=2026-10-05T20:00:01.000000Z leaderRunMs=91000") ||
		!strings.Contains(line, "goneBecause=closed goneLeaderWasHost=true") {
		t.Errorf("the lead's passing logged as %q; want the start of the new leader's run and why", line)
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
// passes at once; a leader's stall, within its grace, is not (D372 §5, D373 §2). A device whose page
// is hidden, or that plays nothing, cannot lead (§6).
func TestHub_ALeaderHiddenOrSilentIsGoneAndAStalledOneIsNot(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 3)
	stage.hub.Detach(stage.host, GoneClosed)
	watchAttachment, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	first, second, third := stage.guests[0], stage.guests[1], stage.guests[2]
	stage.report(t, first, stage.groupPositionMs(stage.opened.SessionID), true, true)
	if leader := rosterLeader(watchBox); leader != first.DeviceID {
		t.Errorf("device %d leads as the leader stalls; want the stalled leader still, %d", leader, first.DeviceID)
	}
	if group := stage.pongGroup(t, watchAttachment, watchBox); group.State != GroupPlaying {
		t.Errorf("the group as its leader stalls: %+v; want Playing on, waiting for no one's stall (D373)", group)
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

// stall says attachment's player stalls where the group is now, playing but unable to play on, as a
// stalled player tells its state each second.
func (stage sessionStage) stall(t *testing.T, attachment Attachment) {
	t.Helper()
	stage.report(t, attachment, stage.groupPositionMs(attachment.SessionID), true, true)
}

// No one's stall holds a Watch Together group (D373 §1): a leader stalled past stalledLeaderGrace is
// gone for the lead, which passes down the line, the host's player to the longest-present guest's
// still playing, a stalled one passed over, and the group plays on throughout, telling no one; the
// log says the lead passed for a stall and how long it lasted, and that the group played on through
// it (§2). A follower's stall moves nothing.
func TestHub_ALeaderStalledPastItsGracePassesTheLeadAndTheGroupPlaysOn(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 2)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stalledGuest, playingGuest := stage.guests[0], stage.guests[1]
	seq := stage.hostBox.last(MessageGroup).Seq
	stage.clock.Advance(10 * time.Second)
	stage.stall(t, stalledGuest)
	stage.stall(t, stage.host)
	for elapsed := time.Second; elapsed < stalledLeaderGrace; elapsed += time.Second {
		stage.clock.Advance(time.Second)
		stage.stall(t, stage.host)
		if leader := rosterLeader(watchBox); leader != stage.host.DeviceID {
			t.Fatalf("device %d leads, the host's player stalled %s, within its grace; want the host's still",
				leader, elapsed)
		}
	}
	stage.clock.Advance(time.Second)
	stage.stall(t, stage.host)
	if leader := rosterLeader(watchBox); leader != playingGuest.DeviceID {
		t.Errorf("device %d leads, the host's player stalled past its grace; want %d, the guest's that plays, "+
			"the more senior guest's stalled too", leader, playingGuest.DeviceID)
	}
	group := stage.pongGroup(t, stage.host, stage.hostBox)
	if group.State != GroupPlaying || group.PositionMs != 15_000 || stage.hostBox.last(MessageGroup).Seq != seq {
		t.Errorf("the group, 3 s into its leader's stall: %+v (seq %d, was %d); want Playing on at 15 s, untold",
			group, stage.hostBox.last(MessageGroup).Seq, seq)
	}
	if line := logLineWith(stage.logs.String(), "goneBecause=stalled"); !strings.Contains(line,
		"why=leaderGone") || !strings.Contains(line, "goneLeaderWasHost=true stalledMs=3000") {
		t.Errorf("the stalled leader's passing logged as %q; want why, whose and how long", line)
	}
	stage.report(t, stalledGuest, 15_000, true, false)
	if line := testutil.LineWith(t, stage.logs.String(), eventStallNotWaitedFor); !strings.Contains(line,
		fmt.Sprintf("deviceId=%d logEvent=stallNotWaitedFor isLeader=false stalledMs=3000", stalledGuest.DeviceID)) {
		t.Errorf("the guest's stall logged as %q; want who stalled, and for how long, the group playing on", line)
	}
}

// A leader's stall shorter than stalledLeaderGrace, as a quality switch's, keeps the lead, so the
// lead is not handed back and forth; the group plays on through it (D373 §2), and the leader playing
// again behind it catches up to it, the group never stepping back for the stall, and leads from where
// it caught up; the log says it catches up, how far behind, its catch-up's move, and once in step
// (D374).
func TestHub_ALeadersStallWithinItsGraceKeepsTheLead(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.clock.Advance(10 * time.Second)
	stage.stall(t, stage.host)
	stage.clock.Advance(stalledLeaderGrace - time.Millisecond)
	stage.stall(t, stage.guests[0]) // a follower's state, as the grace nearly ends
	seq := stage.hostBox.last(MessageGroup).Seq
	stage.report(t, stage.host, 11_000, true, false) // plays again where it stalled
	if leader := rosterLeader(watchBox); leader != stage.host.DeviceID {
		t.Errorf("device %d leads, the host's player stalled just under its grace; want the host's still", leader)
	}
	if last := stage.hostBox.last(MessageGroup); last.Group.State != GroupPlaying || last.Seq != seq {
		t.Errorf("the group through its leader's short stall: %+v; want Playing, untold", last.Group)
	}
	if at := stage.groupPositionMs(stage.opened.SessionID); at != 13_999 {
		t.Errorf("the group is at %d ms as its leader plays again at 11 s; want 13.999 s, playing on, "+
			"not stepped back to the leader (D374)", at)
	}
	if line := testutil.LineWith(t, stage.logs.String(), eventStallNotWaitedFor); !strings.Contains(line,
		"isLeader=true stalledMs=2999 positionMs=11000 catchesUp=true behindGroupMs=2979") {
		t.Errorf("the leader's stall logged as %q; want it catching up, 2.979 s behind", line)
	}
	stage.hub.Handle(t.Context(), stage.host, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSought, PositionMs: 11_000, ToMs: 14_500, Why: "catchUp",
	}})
	if line := testutil.LineWith(t, stage.logs.String(), eventFollowerCaughtUp); !strings.Contains(line,
		"isLeaderCatchingUp=true") {
		t.Errorf("the leader's catch-up seek logged as %q; want it said, the leader catching up", line)
	}
	stage.report(t, stage.host, 14_000, false, false) // waiting ahead, paused, for the group
	if at := stage.groupPositionMs(stage.opened.SessionID); at != 13_999 ||
		stage.hostBox.last(MessageGroup).Seq != seq {
		t.Errorf("the leader waits ahead for its group: the group at %d ms, %+v; want it playing on, untold",
			at, stage.hostBox.last(MessageGroup).Group)
	}
	stage.clock.Advance(time.Millisecond)
	stage.report(t, stage.host, 14_100, true, false) // in step
	if at := stage.groupPositionMs(stage.opened.SessionID); at != 14_120 {
		t.Errorf("the group is at %d ms as its leader plays in step at 14.1 s; want 14.12 s, the leader's", at)
	}
	if line := testutil.LineWith(t, stage.logs.String(), eventLeaderCaughtUp); !strings.Contains(line,
		"offGroupMs=120 catchingUpMs=1") {
		t.Errorf("the leader's catch-up logged as %q; want how far off and after how long", line)
	}
	stage.clock.Advance(time.Hour)
	stage.ready(t, stage.guests[0], false)
	if leader := rosterLeader(watchBox); leader != stage.host.DeviceID {
		t.Errorf("device %d leads, the host's player's stall over; want the host's still", leader)
	}
}

// Starting still waits (D373 §4): a host's seek stands the group for its leader, however long the
// leader loads the new place, and hands on no lead; the group plays as the leader plays.
func TestHub_AHostsSeekStillWaitsForItsLeaderAndPassesNoLead(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 2)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.clock.Advance(10 * time.Second)
	loading := stage.guests[0]
	stage.stall(t, loading) // stalled as the host seeks, and loading the new place after
	stage.hub.Handle(t.Context(), stage.host, Inbound{
		Type: MessageCommand, Command: CommandSeek, PositionMs: 600_000,
		BasedOnSeq: stage.hostBox.last(MessageGroup).Seq,
	})
	stage.ready(t, stage.guests[1], false)
	for range 10 {
		stage.clock.Advance(time.Second)
		stage.report(t, stage.host, 600_000, true, true)
	}
	if group := stage.hostBox.last(MessageGroup).Group; group.State != GroupWaiting || group.Event != EventSeeked {
		t.Errorf("the group 10 s after the host's seek, its leader loading: %+v; want Waiting for it", group)
	}
	if leader := rosterLeader(watchBox); leader != stage.host.DeviceID {
		t.Errorf("device %d leads as the host's player loads its seek; want the host's", leader)
	}
	stage.report(t, stage.host, 600_000, true, false)
	if group := stage.hostBox.last(MessageGroup).Group; group.State != GroupPlaying || group.Event != EventLeaderPlays {
		t.Errorf("the group as its leader plays: %+v; want Playing, the leader playing", group)
	}
	stage.hub.Detach(stage.host, GoneClosed)
	stage.clock.Advance(time.Second)
	stage.report(t, loading, 600_000, true, true)
	if leader := rosterLeader(watchBox); leader != loading.DeviceID {
		t.Errorf("device %d leads, the senior guest's player loading 1 s into the group's playing; want it, %d: "+
			"its loading before the group played is the seek's, not a stall", leader, loading.DeviceID)
	}
}

// The host's player stalled past its grace hands the lead to a guest's, catches up as a follower,
// and takes the lead back once it plays in step (D373 §3); a guest's stalled former leader just
// follows.
func TestHub_TheHostsStalledPlayerTakesTheLeadBackInStepAndAGuestsFollows(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 2)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.clock.Advance(10 * time.Second)
	stage.stall(t, stage.host)
	stage.clock.Advance(stalledLeaderGrace)
	stage.stall(t, stage.host)
	if leader := rosterLeader(watchBox); leader != stage.guests[0].DeviceID {
		t.Fatalf("device %d leads, the host's player stalled past its grace; want the senior guest's, %d",
			leader, stage.guests[0].DeviceID)
	}
	stage.report(t, stage.host, 10_000, true, false) // plays again, 3 s behind
	if leader := rosterLeader(watchBox); leader != stage.guests[0].DeviceID {
		t.Errorf("device %d leads, the host's player playing 3 s behind; want it to follow until in step", leader)
	}
	at := stage.groupPositionMs(stage.opened.SessionID)
	stage.report(t, stage.host, at, true, false)
	if leader := rosterLeader(watchBox); leader != stage.host.DeviceID {
		t.Errorf("device %d leads, the host's player caught up in step; want it back, %d", leader, stage.host.DeviceID)
	}
	if !strings.Contains(stage.logs.String(), "why=hostInStep") {
		t.Error("no log line says the host's player took the lead back in step")
	}

	stage.hub.Detach(stage.host, GoneClosed)
	guest := stage.guests[0]
	stage.stall(t, guest)
	stage.clock.Advance(stalledLeaderGrace)
	stage.stall(t, guest)
	if leader := rosterLeader(watchBox); leader != stage.guests[1].DeviceID {
		t.Fatalf("device %d leads, the guest's leader stalled past its grace; want the other guest's, %d",
			leader, stage.guests[1].DeviceID)
	}
	stage.report(t, guest, stage.groupPositionMs(stage.opened.SessionID), true, false)
	if leader := rosterLeader(watchBox); leader != stage.guests[1].DeviceID {
		t.Errorf("device %d leads, the stalled guest's player in step again; want it to follow", leader)
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

// A cast is a session of one, which holds no one up: its group stands where its receiver stalls,
// however long, and plays from where the receiver plays again. No one's stall holds a Watch
// Together group (D373 §1); a cast's receiver is its only player.
func TestCast_AReceiversStallStandsItsGroupForIt(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	receiver, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	_, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.report(t, receiver, 0, true, false)
	stage.clock.Advance(10 * time.Second)
	stage.report(t, receiver, 9_000, true, true)
	stage.clock.Advance(2 * stalledLeaderGrace)
	stage.report(t, receiver, 9_000, true, true)
	if group := remoteBox.last(MessageGroup).Group; group.State != GroupWaiting || group.Event != EventWaitingFor ||
		group.PositionMs != 9_000 {
		t.Errorf("the cast's receiver stalled at 9 s: %+v; want its group Waiting there for it", group)
	}
	stage.report(t, receiver, 9_000, true, false)
	if group := remoteBox.last(MessageGroup).Group; group.State != GroupPlaying || group.Event != EventLeaderPlays {
		t.Errorf("the cast's receiver plays again: %+v; want its group Playing from there", group)
	}
}

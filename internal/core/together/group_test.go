// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"testing"
	"time"
)

// A player that stalls stops no one, the first time or any other: the group plays on, naming it, and
// it catches up alone, its reports while it does changing nothing (D250 §2).
func TestHub_AStalledPlayerStopsNoOneAndCatchesUpAlone(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	hostBox, guestDevice := stage.hostBox, stage.guestDevice
	guest := stage.guest
	for stall := range 3 {
		stage.ready(t, guestDevice, true)
		group := hostBox.last(MessageGroup).Group
		if group.State != GroupPlaying || group.Event != EventContinuedWithout || group.ByMemberID != guest.MemberID {
			t.Errorf("the guest's stall %d: %+v; want Playing on without the guest", stall+1, group)
		}
		seq := hostBox.last(MessageGroup).Seq
		stage.ready(t, guestDevice, true)
		stage.ready(t, guestDevice, false)
		if hostBox.last(MessageGroup).Seq != seq {
			t.Errorf("the guest catching up changed the group: %+v", hostBox.last(MessageGroup).Group)
		}
		stage.clock.Advance(time.Minute)
	}
}

// A player that stalls with no other player playing along holds the group where it stalled until it
// is ready, past the sync wait: played on, the group played to no one, and the one watching jumped
// 75 s ahead to catch up (the owner's Bram, staging, 2026-10-12; D250 §2's amendment).
func TestHub_TheOnlyPlayerStallingHoldsTheGroupUntilItIsReady(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.ready(t, host, false)
	stage.command(t, host, CommandChangeItem, 7)
	stage.clock.Advance(time.Minute)
	stage.ready(t, host, true)
	held := hostBox.last(MessageGroup).Group
	if held.State != GroupWaiting || held.Event != EventWaitingFor || held.ByMemberID != host.MemberID {
		t.Fatalf("the only player stalled: %+v; want Waiting for it", held)
	}
	stage.clock.Advance(testSyncWait + time.Minute)
	stage.tick(t, host)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting || group.PositionMs != held.PositionMs {
		t.Fatalf("past the sync wait, the player still loading: %+v; want Waiting at %d ms", group, held.PositionMs)
	}
	stage.ready(t, host, false)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying || group.PositionMs != held.PositionMs {
		t.Errorf("the player ready again: %+v; want Playing from %d ms, where it stalled", group, held.PositionMs)
	}
}

// Once every player has stalled or dropped, one after another, the group holds where the last
// left it until one of them is ready again: it never moves on while no one watches (the owner,
// 2026-10-12; D250 §2's amendment).
func TestHub_EveryPlayerStalledOrGoneHoldsTheGroupUntilOneIsReady(t *testing.T) {
	t.Parallel()
	for _, lastDrops := range []bool{false, true} {
		stage := playing(t)
		hostBox, guestDevice := stage.hostBox, stage.guestDevice
		stage.clock.Advance(time.Minute)
		stage.ready(t, guestDevice, true)
		if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying {
			t.Fatalf("the guest stalled, the host's player along: %+v; want Playing on", group)
		}
		stage.clock.Advance(10 * time.Second)
		if lastDrops {
			stage.hub.Detach(stage.host)
		} else {
			stage.ready(t, stage.host, true)
		}
		_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RoleVoice)
		held := watchBox.last(MessageWelcome).Group
		if held.State != GroupWaiting || held.PositionMs != 70_000 {
			t.Fatalf("host dropped %t, every player stalled or gone: %+v; want Waiting at 70 s", lastDrops, held)
		}
		stage.clock.Advance(testSyncWait + time.Minute)
		stage.tick(t, guestDevice)
		if changed := watchBox.last(MessageGroup).Group; changed != nil {
			t.Fatalf("host dropped %t, past the sync wait with no one ready: %+v; want it held, unchanged",
				lastDrops, changed)
		}
		stage.ready(t, guestDevice, false)
		stage.clock.Advance(testSyncWait)
		stage.tick(t, guestDevice)
		group := watchBox.last(MessageGroup).Group
		if group.State != GroupPlaying || group.PositionMs != 70_000 {
			t.Errorf("host dropped %t, the guest ready again: %+v; want Playing from 70 s", lastDrops, group)
		}
	}
}

// The last player playing along that drops, stalled first or not, holds the group until a player is
// ready, the same one back on a new connection: the group started playing to no one as the owner's
// stalled player reconnected (staging, 2026-10-12; D250 §2's amendment).
func TestHub_TheLastPlayerDroppingHoldsTheGroupUntilOneIsBack(t *testing.T) {
	t.Parallel()
	for _, stallsFirst := range []bool{false, true} {
		stage := newStage(t)
		host, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
		_, voiceBox := stage.attach(t, stage.opened.MembershipCredential, RoleVoice)
		stage.ready(t, host, false)
		stage.command(t, host, CommandChangeItem, 7)
		stage.clock.Advance(time.Minute)
		if stallsFirst {
			stage.ready(t, host, true)
		}
		stage.hub.Detach(host)
		held := voiceBox.last(MessageGroup).Group
		if held.State != GroupWaiting || held.Event != EventWaitingFor {
			t.Fatalf("stalled first %t, the only player dropped: %+v; want Waiting", stallsFirst, held)
		}
		back, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
		stage.ready(t, back, true)
		stage.clock.Advance(10 * time.Second)
		if group := voiceBox.last(MessageGroup).Group; group.State != GroupWaiting {
			t.Fatalf("stalled first %t, the player back and loading: %+v; want still Waiting", stallsFirst, group)
		}
		stage.ready(t, back, false)
		group := voiceBox.last(MessageGroup).Group
		if group.State != GroupPlaying || group.PositionMs != held.PositionMs {
			t.Errorf("stalled first %t, the player back and ready: %+v; want Playing from %d ms",
				stallsFirst, group, held.PositionMs)
		}
	}
}

// A sync point waits at most the sync wait its devices were welcomed with, once a player is ready:
// then the players not ready, the host's as anyone's, catch up alone (D250 §3).
func TestHub_ASyncPointWaitsAtMostTheSyncWaitThenPlaysWithoutWhoeverIsNotReady(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	host, hostBox, guestDevice := stage.host, stage.hostBox, stage.guestDevice
	guest := stage.guest
	seek := func(positionMs int64) {
		stage.hub.Handle(t.Context(), host, Inbound{
			Type: MessageCommand, Command: CommandSeek, PositionMs: positionMs,
			BasedOnSeq: hostBox.last(MessageGroup).Seq,
		})
	}
	seek(600_000)
	stage.ready(t, host, false) // the host has loaded the new place; the guest has not
	stage.clock.Advance(testSyncWait - time.Second)
	stage.tick(t, host)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupWaiting {
		t.Fatalf("before the sync wait's time: %s, want Waiting", state)
	}
	stage.clock.Advance(time.Second)
	stage.tick(t, host)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying || group.Event != EventContinuedWithout ||
		group.ByMemberID != guest.MemberID {
		t.Errorf("at the sync wait's time: %+v; want Playing on without the guest", group)
	}
	stage.ready(t, guestDevice, false)
	seek(1_200_000)
	stage.ready(t, guestDevice, false) // now the guest has loaded it, and the host has not
	stage.clock.Advance(testSyncWait)
	stage.tick(t, guestDevice)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying ||
		group.ByMemberID != host.MemberID {
		t.Errorf("at the sync wait's time, the host still loading: %+v; want Playing on without the host", group)
	}
	short := Tuning{SyncWaitMs: 5000}
	_, err := stage.hub.Attach(guest.MembershipCredential, RoleVoice, true, nil, short, (&inbox{}).send)
	if err != nil {
		t.Fatal(err)
	}
	stage.ready(t, host, false)
	seek(1_800_000)
	stage.ready(t, host, false)
	stage.clock.Advance(5 * time.Second)
	stage.tick(t, host)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Errorf("5 s after a seek, the last device welcomed with a 5 s sync wait: %s; want Playing", state)
	}
}

// After a seek the group waits for every player to say it is ready from the new place, not from
// what it said of the old one, and loading there is no stall: a stall after it is (D250 §2, §3).
func TestHub_AfterASeekEachPlayerLoadsTheNewPlaceWithoutStalling(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	host, hostBox, guestDevice := stage.host, stage.hostBox, stage.guestDevice
	stage.hub.Handle(t.Context(), host, Inbound{
		Type: MessageCommand, Command: CommandSeek, PositionMs: 600_000, BasedOnSeq: hostBox.last(MessageGroup).Seq,
	})
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting {
		t.Fatalf("just after the host's seek: %+v; want Waiting for the players to load there", group)
	}
	stage.ready(t, guestDevice, true)
	stage.ready(t, host, false)
	stage.ready(t, guestDevice, false)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying {
		t.Fatalf("both loaded where the host jumped to: %+v; want Playing", group)
	}
	stage.ready(t, guestDevice, true)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying || group.Event != EventContinuedWithout {
		t.Errorf("the guest's first stall after loading the new place: %+v; want Playing on without it", group)
	}
}

// The host's player that puts a film on is waited for at the sync point, though it has not yet said
// it is ready: it connected as the host's player page opened.
func TestHub_TheHostsPlayerIsWaitedForAsItLoadsWhatTheHostPutOn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.ready(t, guestDevice, false)
	stage.command(t, host, CommandChangeItem, 7)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting {
		t.Fatalf("film 7 put on, the host's player not yet loaded: %+v; want Waiting for it", group)
	}
	stage.clock.Advance(testSyncWait - time.Second)
	stage.tick(t, guestDevice)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting {
		t.Fatalf("within the sync wait after the host put film 7 on: %+v; want still Waiting", group)
	}
	stage.ready(t, host, false)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying {
		t.Errorf("the host's player loaded: %+v; want Playing", group)
	}
}

// While no player is ready a sync point waits on past its time, so the group never plays to no one,
// and a player catching up is waited for again by what the host puts on next (D250 §3).
func TestHub_WithNoPlayerReadyASyncPointWaitsOnPastItsTime(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	host, hostBox, guestDevice := stage.host, stage.hostBox, stage.guestDevice
	guest := stage.guest
	stage.ready(t, guestDevice, true) // it catches up alone
	stage.command(t, host, CommandChangeItem, 8)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting {
		t.Fatalf("film 8 put on with the guest, catching up, still loading: %+v, want Waiting for it", group)
	}
	stage.ready(t, host, true)
	stage.clock.Advance(testSyncWait)
	stage.tick(t, host)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupWaiting {
		t.Errorf("past the sync wait with no player ready: %s, want still Waiting", state)
	}
	stage.ready(t, host, false)
	stage.tick(t, host)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying || group.ByMemberID != guest.MemberID {
		t.Errorf("past its time with the host ready: %+v, want Playing on without the guest", group)
	}
}

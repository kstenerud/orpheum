// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// A link admits its person for its lifetime from when it was made, and then no one: it leaves the
// host's view and the discovery's links; a link made after EndStale was told a lifetime keeps that
// one; who joined in time stays in (D346).
func TestHub_ALinkAdmitsNoOneOnceItsLifetimeHasPassed(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	inTime, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a cousin", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	late, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "an aunt", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	stage.clock.Advance(StartingLinkLifetime - time.Minute)
	cousin, err := stage.hub.Join(inTime.Secret, "Cousin", nil)
	if err != nil {
		t.Fatalf("a join within the link's hour = %v; want admitted", err)
	}
	if live := stage.hub.LiveLinks(); len(live) != 1 {
		t.Errorf("before its hour is up the discovery finds %d links; want the aunt's", len(live))
	}
	stage.clock.Advance(time.Minute)
	if _, err = stage.hub.Join(late.Secret, "Aunt", nil); !errors.Is(err, ErrNoSession) {
		t.Errorf("a join an hour after the link was made = %v; want ErrNoSession", err)
	}
	details, err := stage.hub.DetailsOf(stage.opened.SessionID, stage.owner.UserID)
	if err != nil || len(details.Links) != 1 || details.Links[0].UsedBy != cousin.MemberID {
		t.Errorf("the host's links = %+v (%v); want only the cousin's, used", details.Links, err)
	}
	if live := stage.hub.LiveLinks(); len(live) != 0 {
		t.Errorf("an expired link is among the discovery's links: %+v", live)
	}
	if _, found := stage.hub.ViewerOf(cousin.MembershipCredential); !found {
		t.Error("the cousin, who joined in time, is no longer in")
	}
	stage.hub.EndStale(Lifetimes{Link: 10 * time.Minute, Idle: StartingIdleEnd})
	short, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a neighbour", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	stage.clock.Advance(10 * time.Minute)
	if _, err = stage.hub.Join(short.Secret, "Neighbour", nil); !errors.Is(err, ErrNoSession) {
		t.Errorf("a join past the lifetime the setting gave = %v; want ErrNoSession", err)
	}
}

// A session nothing plays in and no one does anything in for the idle end is ended as the host's end
// would: each device is told "idle", and no credential nor link admits anyone. Playing to a player
// keeps it on, and a member's command starts its idle time again; what a page sends by itself, its
// state and pings, does not (D346).
func TestHub_ASessionIdleForItsIdleEndIsEnded(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	lifetimes := Lifetimes{Link: time.Hour, Idle: 6 * time.Hour}
	stage.clock.Advance(7 * time.Hour)
	if ended := stage.hub.EndStale(lifetimes); len(ended) != 0 {
		t.Fatalf("a session playing for 7 hours was ended: %v", ended)
	}
	stage.command(t, stage.host, CommandPause, 0)
	stage.clock.Advance(5 * time.Hour)
	stage.ready(t, stage.guestDevice, false)
	stage.hub.Handle(t.Context(), stage.host, Inbound{Type: MessagePing})
	if ended := stage.hub.EndStale(lifetimes); len(ended) != 0 {
		t.Fatalf("a session paused for 5 hours was ended: %v", ended)
	}
	waiting, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a friend", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	stage.clock.Advance(6 * time.Hour)
	stage.ready(t, stage.guestDevice, false)
	if ended := stage.hub.EndStale(lifetimes); !slices.Equal(ended, []int64{stage.opened.SessionID}) {
		t.Fatalf("after 6 hours with only the pages' own reports, EndStale ended %v; want the session", ended)
	}
	if told := stage.hostBox.last(MessageEnded); told.Reason != "idle" {
		t.Errorf("the host's device was told %+v; want ended, idle", told)
	}
	if _, found := stage.hub.ViewerOf(stage.guest.MembershipCredential); found {
		t.Error("the guest's credential still admits them to an ended session")
	}
	if _, err = stage.hub.Join(waiting.Secret, "Friend", nil); !errors.Is(err, ErrNoSession) {
		t.Errorf("a join by the ended session's link = %v; want ErrNoSession", err)
	}
}

// A group playing with no player attached is not played to: its players all gone, the session is
// idle (D346).
func TestHub_AGroupPlayingToNoPlayerIsIdle(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	stage.hub.Detach(stage.host)
	stage.hub.Detach(stage.guestDevice)
	stage.clock.Advance(StartingIdleEnd)
	if ended := stage.hub.EndStale(startingLifetimes()); len(ended) != 1 {
		t.Errorf("a group with every player gone for the idle end: EndStale ended %v; want it", ended)
	}
}

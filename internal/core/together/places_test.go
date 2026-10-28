// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"testing"
)

// A person is in one session at most: starting one ends the one they host, its guests told the host
// ended it, and takes them out of one they joined, its other members told the roster; its new
// credential admits the host, who plays as themselves (D246 §5, D372 §8).
func TestHub_StartingASessionEndsTheOneThePersonHostsAndLeavesTheOneTheyJoined(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, guestBox := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	friend := Person{UserID: 2, Name: "friend"}
	friends, err := stage.hub.Open(friend)
	if err != nil {
		t.Fatal(err)
	}
	made, err := stage.hub.AddLink(friends.SessionID, friend.UserID, "the owner", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := stage.hub.Join(made.Secret, "owner", &stage.owner, Leaving{ShouldLeave: true})
	if err != nil {
		t.Fatal(err)
	}
	if told := guestBox.last(MessageEnded); told.Reason != "hostEnded" {
		t.Errorf("the owner joining the friend's session, saying to leave theirs: their guest was told %+v; "+
			"want hostEnded", told)
	}
	_, joinedBox := stage.attach(t, joined.MembershipCredential, RoleVideo)
	again, err := stage.hub.Open(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	if again.SessionID == stage.opened.SessionID || again.SessionID == friends.SessionID {
		t.Errorf("starting again = %+v; want a new session", again)
	}
	if told := joinedBox.last(MessageEnded); told.Reason != "left" {
		t.Errorf("the owner's device in the friend's session was told %+v; want left", told)
	}
	if _, found := stage.hub.ViewerOf(joined.MembershipCredential); found {
		t.Error("the owner is still a guest in the friend's session")
	}
	_, err = stage.hub.Attach(again.MembershipCredential, RoleVideo, true, nil, testTuning, (&inbox{}).send)
	if err != nil {
		t.Errorf("the new credential = %v; want it to admit the host", err)
	}
	if _, isViewer := stage.hub.ViewerOf(again.MembershipCredential); isViewer {
		t.Error("the host's credential reads as a viewer's; the host plays as themselves")
	}
	if _, err = stage.hub.Attach(stage.opened.MembershipCredential, RoleVideo, true, nil, testTuning,
		(&inbox{}).send); !errors.Is(err, ErrNoSession) {
		t.Errorf("the old session's credential = %v; want ErrNoSession, it ended", err)
	}
}

// Joining another session asks first: a person in one, signed in or by the membership their
// browser keeps, is refused, told whose it is, and only their yes takes them out of it (D372 §8).
func TestHub_JoiningAnotherSessionIsRefusedUntilThePersonSaysToLeaveTheirs(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	friend := Person{UserID: 2, Name: "friend"}
	friends, err := stage.hub.Open(friend)
	if err != nil {
		t.Fatal(err)
	}
	link := func(note string) string {
		made, linkErr := stage.hub.AddLink(friends.SessionID, friend.UserID, note, Rights{})
		if linkErr != nil {
			t.Fatal(linkErr)
		}
		return made.Secret
	}
	_, err = stage.hub.Join(link("the owner"), "owner", &stage.owner, Leaving{})
	if refused, isRefused := errors.AsType[*InAnotherSessionError](err); !isRefused || !refused.IsHost {
		t.Errorf("the owner, hosting, joining the friend's = %v; want refused, their own session", err)
	}
	byLink := stage.join(t)
	secret := link("a cousin")
	_, err = stage.hub.Join(secret, "cousin", nil, Leaving{HeldCredential: byLink.MembershipCredential})
	if refused, isRefused := errors.AsType[*InAnotherSessionError](err); !isRefused || refused.IsHost ||
		refused.HostName != "owner" {
		t.Errorf("a guest by link alone joining another = %v; want refused, the owner's session", err)
	}
	if _, found := stage.hub.ViewerOf(byLink.MembershipCredential); !found {
		t.Error("a refused join took the guest out of their session")
	}
	if _, err = stage.hub.Join(secret, "cousin", nil, Leaving{
		HeldCredential: byLink.MembershipCredential, ShouldLeave: true,
	}); err != nil {
		t.Fatalf("joining, saying to leave = %v", err)
	}
	if _, found := stage.hub.ViewerOf(byLink.MembershipCredential); found {
		t.Error("the guest is still in the session they said to leave")
	}
	if _, err = stage.hub.Join(link("someone"), "someone", nil, Leaving{}); err != nil {
		t.Errorf("someone in no session joining = %v; want admitted", err)
	}
}

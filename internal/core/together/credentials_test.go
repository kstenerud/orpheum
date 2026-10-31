// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"strings"
	"testing"
)

// flipped is credential with its last character changed: the same length and alphabet, but not it.
func flipped(credential string) string {
	last := credential[len(credential)-1]
	replacement := "A"
	if last == 'A' {
		replacement = "B"
	}
	return credential[:len(credential)-1] + replacement
}

// A credential admits by what it is, never by being there: no membership is found, viewed, attached
// to, left or joined by one that is empty, well formed but no one's, one character off a real one,
// another kind of secret (a link's for a membership's, and a membership's for a link's), an ejected
// member's or an ended session's. Plex2Shell's first step was a token checked only for being present
// (security-week-2026-10-30 §5).
func TestHub_NoCredentialButTheLiveOneItIsAdmitsAnyone(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	waiting, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a cousin", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	ejected := stage.join(t)
	if err = stage.hub.Eject(stage.opened.SessionID, stage.owner.UserID, ejected.MemberID); err != nil {
		t.Fatal(err)
	}
	other, err := stage.hub.Open(Person{UserID: 2, Name: "neighbour"})
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.End(other.SessionID, 2); err != nil {
		t.Fatal(err)
	}
	wrongMemberships := map[string]string{
		"empty":                          "",
		"well formed but no one's":       strings.Repeat("A", len(guest.MembershipCredential)),
		"a guest's, one character off":   flipped(guest.MembershipCredential),
		"the host's, one character off":  flipped(stage.opened.MembershipCredential),
		"a link's secret":                waiting.Secret,
		"an ejected member's":            ejected.MembershipCredential,
		"an ended session's host's":      other.MembershipCredential,
		"a guest's, with a space before": " " + guest.MembershipCredential,
	}
	for name, credential := range wrongMemberships {
		if _, isViewer := stage.hub.ViewerOf(credential); isViewer {
			t.Errorf("%s: ViewerOf admitted a viewer", name)
		}
		if _, _, isMember := stage.hub.MemberOf(credential); isMember {
			t.Errorf("%s: MemberOf named a member", name)
		}
		box := &inbox{}
		if _, err := stage.hub.Attach(credential, RoleVideo, true, nil, testTuning, box.send); err == nil {
			t.Errorf("%s: a device attached", name)
		}
		if err := stage.hub.Leave(credential); !errors.Is(err, ErrNoSession) {
			t.Errorf("%s: Leave = %v, want ErrNoSession", name, err)
		}
	}
	wrongLinks := map[string]string{
		"empty":                       "",
		"well formed but no one's":    strings.Repeat("A", len(waiting.Secret)),
		"a link's, one character off": flipped(waiting.Secret),
		"a membership's credential":   guest.MembershipCredential,
		"the host's credential":       stage.opened.MembershipCredential,
	}
	for name, secret := range wrongLinks {
		if _, err := stage.hub.Join(secret, "stranger", nil, Leaving{}); !errors.Is(err, ErrNoSession) {
			t.Errorf("%s: Join = %v, want ErrNoSession", name, err)
		}
	}
	// What was refused above took nothing from those it was not: each real one still admits.
	viewer, isViewer := stage.hub.ViewerOf(guest.MembershipCredential)
	if !isViewer || viewer.MemberID != guest.MemberID {
		t.Errorf("the guest's own credential = %+v, %v; want them", viewer, isViewer)
	}
	if _, err := stage.hub.Join(waiting.Secret, "cousin", nil, Leaving{}); err != nil {
		t.Errorf("the waiting link's own secret: %v", err)
	}
}

// A host's management of a session is theirs alone: another person, a guest among them, or the
// host of another session, naming the session, is refused, as is the host naming a session that
// is not theirs.
func TestHub_OnlyItsHostManagesASessionByItsID(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	other, err := stage.hub.Open(Person{UserID: 2, Name: "neighbour"})
	if err != nil {
		t.Fatal(err)
	}
	const guestUserID, strangerUserID = 3, 4
	for name, ask := range map[string]struct{ sessionID, userID int64 }{
		"another session's host":        {stage.opened.SessionID, 2},
		"someone in no session":         {stage.opened.SessionID, strangerUserID},
		"a guest's person":              {stage.opened.SessionID, guestUserID},
		"the host, of another session":  {other.SessionID, stage.owner.UserID},
		"the host, of no such session":  {other.SessionID + 100, stage.owner.UserID},
		"the zero person, of this one":  {stage.opened.SessionID, 0},
		"the host, of the zero session": {0, stage.owner.UserID},
	} {
		if _, err := stage.hub.DetailsOf(ask.sessionID, ask.userID); err == nil {
			t.Errorf("%s: DetailsOf answered", name)
		}
		if _, err := stage.hub.AddLink(ask.sessionID, ask.userID, "a note", Rights{}); err == nil {
			t.Errorf("%s: AddLink made a link", name)
		}
		if _, err := stage.hub.AnotherLinkFor(ask.sessionID, ask.userID, guest.MemberID); err == nil {
			t.Errorf("%s: AnotherLinkFor made a link", name)
		}
		if err := stage.hub.Eject(ask.sessionID, ask.userID, guest.MemberID); err == nil {
			t.Errorf("%s: Eject ejected the guest", name)
		}
		if err := stage.hub.End(ask.sessionID, ask.userID); err == nil {
			t.Errorf("%s: End ended a session", name)
		}
	}
	if _, isViewer := stage.hub.ViewerOf(guest.MembershipCredential); !isViewer {
		t.Error("the guest was taken out by a refused ask")
	}
	if _, err := stage.hub.DetailsOf(other.SessionID, 2); err != nil {
		t.Errorf("the other session was ended by a refused ask: %v", err)
	}
}

// A cast is its sender's: another person naming it gets no credential for it and cannot end it.
func TestCast_OnlyItsSenderHasACredentialForItOrEndsIt(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	for name, ask := range map[string]struct{ sessionID, userID int64 }{
		"another person":            {cast.SessionID, 2},
		"the sender, of a session":  {stage.opened.SessionID, stage.owner.UserID},
		"the sender, of no session": {cast.SessionID + 100, stage.owner.UserID},
	} {
		if credential, err := stage.hub.CastCredential(ask.sessionID, ask.userID); err == nil {
			t.Errorf("%s: CastCredential gave %q", name, credential)
		}
		if err := stage.hub.EndCast(ask.sessionID, ask.userID, "ended"); err == nil {
			t.Errorf("%s: EndCast ended it", name)
		}
	}
	box := &inbox{}
	if _, err := stage.hub.Attach(cast.ReceiverCredential, RoleVideo, true, nil, testTuning, box.send); err != nil {
		t.Errorf("the cast was ended by a refused ask: %v", err)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"testing"
	"time"
)

// An AirPlay lab grant stands for its playback alone, for 24 hours and not a second more
// (airplay.md item 3: the lab's length, the owner's to change after the spike).
func TestLabAirPlayGrant_StandsForItsPlaybackForADayThenExpires(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	sender, _ := guestSender(t, fixture.service)
	grant, err := fixture.service.IssueLabAirPlayGrant(sender, "media-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(LabAirPlayGrantLife - time.Second)
	kept, err := fixture.service.LabAirPlayGrantOf(grant)
	if err != nil {
		t.Fatalf("a second before its day ends: %v", err)
	}
	if kept.UserID != sender.UserID || kept.SessionKey != sender.SessionKey || kept.MediaToken != "media-token" ||
		kept.FileID != 7 {
		t.Errorf("the grant stands for %+v; want the sender's playback of file 7", kept)
	}
	fixture.clock.Advance(time.Second)
	if _, err := fixture.service.LabAirPlayGrantOf(grant); !errors.Is(err, ErrNoLabAirPlayGrant) {
		t.Errorf("once its day is over: %v, want ErrNoLabAirPlayGrant", err)
	}
	if _, err := fixture.service.LabAirPlayGrantOf("not-a-grant"); !errors.Is(err, ErrNoLabAirPlayGrant) {
		t.Errorf("an unknown grant: %v, want ErrNoLabAirPlayGrant", err)
	}
}

// Stop revokes the sending session's AirPlay lab grants, and only its own: another session's stay,
// and the count says how many were revoked, for the page to show it was answered.
func TestLabAirPlayGrant_IsRevokedByItsSenderAlone(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	sender, _ := guestSender(t, fixture.service)
	grant, err := fixture.service.IssueLabAirPlayGrant(sender, "media-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	other := sender
	other.SessionKey = "another session"
	otherGrant, err := fixture.service.IssueLabAirPlayGrant(other, "other-token", 8)
	if err != nil {
		t.Fatal(err)
	}
	if revoked := fixture.service.RevokeLabAirPlayGrants(sender); revoked != 1 {
		t.Errorf("revoked %d; want the sender's one grant", revoked)
	}
	if _, err := fixture.service.LabAirPlayGrantOf(grant); !errors.Is(err, ErrNoLabAirPlayGrant) {
		t.Errorf("a revoked grant: %v, want ErrNoLabAirPlayGrant", err)
	}
	if _, err := fixture.service.LabAirPlayGrantOf(otherGrant); err != nil {
		t.Errorf("another session's grant, after the sender's Stop: %v; want it kept", err)
	}
	if revoked := fixture.service.RevokeLabAirPlayGrants(sender); revoked != 0 {
		t.Errorf("a second Stop revoked %d; want none", revoked)
	}
}

// A guest receiver's session cannot AirPlay: it holds its cast's film alone (D465 §2).
func TestLabAirPlayGrant_RefusesAGuestReceiver(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	sender, _ := guestSender(t, fixture.service)
	guest := sender
	guest.isGuestReceiver = true
	if _, err := fixture.service.IssueLabAirPlayGrant(guest, "media-token", 7); !errors.Is(err, ErrLabAirPlaySender) {
		t.Errorf("a guest receiver's grant: %v, want ErrLabAirPlaySender", err)
	}
}

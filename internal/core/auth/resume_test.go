// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"testing"
)

func TestDevices_ASignInResumesTheDeviceTheClientRemembersWhenItIsTheSamePersons(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "partner", "pw", RoleUser)
	fixture.createUser(t, "owner", "pw", RoleAdmin)
	first, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	before, err := fixture.service.Authenticate(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.SignOut(ctx, first); err != nil {
		t.Fatal(err)
	}
	resumed := NewDevice{
		Name: "Firefox on Linux", Platform: "web", PreviousDeviceID: before.DeviceID, PreviousDeviceProof: first,
	}
	again, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", resumed)
	if err != nil {
		t.Fatal(err)
	}
	after, err := fixture.service.Authenticate(ctx, again)
	if err != nil || after.DeviceID != before.DeviceID {
		t.Errorf("signing in again resumed device %d, %v; want %d (D221 §1)", after.DeviceID, err, before.DeviceID)
	}
	someoneElses, err := fixture.service.SignIn(ctx, "owner", "pw", "192.0.2.1", resumed)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := fixture.service.Authenticate(ctx, someoneElses)
	if err != nil || owner.DeviceID == before.DeviceID {
		t.Errorf("another person naming the device got device %d, %v; want a new one", owner.DeviceID, err)
	}
}

// A client that names a device it cannot prove it was, by a session token that device was issued, gets
// a new device: a stranger naming someone's device id sat on it, unseen in their list and kept by
// "sign out everywhere else" (the second audit's K1 and W4, 2026-10-13).
func TestDevices_ADeviceIsResumedOnlyByAClientThatProvesItWasIt(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "partner", "pw", RoleUser)
	phone, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	victim, err := fixture.service.Authenticate(ctx, phone)
	if err != nil {
		t.Fatal(err)
	}
	other, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	for proof, why := range map[string]string{
		"":        "no proof",
		"guessed": "a token never issued",
		other:     "a token of another of the person's devices",
	} {
		named := NewDevice{
			Name: "Intruder", Platform: "web", PreviousDeviceID: victim.DeviceID, PreviousDeviceProof: proof,
		}
		token, signInErr := fixture.service.SignIn(ctx, "partner", "pw", "203.0.113.9", named)
		if signInErr != nil {
			t.Fatal(signInErr)
		}
		principal, authErr := fixture.service.Authenticate(ctx, token)
		if authErr != nil || principal.DeviceID == victim.DeviceID {
			t.Errorf("with %s, the sign-in sat on device %d, %v; want a new device", why, principal.DeviceID, authErr)
		}
	}
}

// A new password set by a registration link ends every session the person had; one changed by the
// person ends every other session of theirs, the asking one alone staying (the second audit's K1;
// D218 §6 as amended 2026-10-13).
func TestPassword_ANewPasswordEndsTheOtherSessions(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "old", RoleUser)
	asking, err := fixture.service.SignIn(ctx, "partner", "old", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	stolen, err := fixture.service.SignIn(ctx, "partner", "old", "203.0.113.9", laptop)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.ChangePassword(ctx, id, "old", "new", asking); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Authenticate(ctx, stolen); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("another session after a changed password: %v, want signed out", err)
	}
	if _, err = fixture.service.Authenticate(ctx, asking); err != nil {
		t.Errorf("the session that changed it: %v, want still signed in", err)
	}
	link, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, link.Secret, "", "newer", laptop); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Authenticate(ctx, asking); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("a session after a registration link set a new password: %v, want signed out", err)
	}
}

func TestRegistration_ALinkIsNotHeldBackByTheLimitAndMakingOneClearsThePerson(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "forgotten", RoleUser)
	var err error
	for range 20 {
		if _, err = fixture.service.SignIn(ctx, "partner", "guess", "198.51.100.9", laptop); errors.Is(err,
			ErrRateLimited) {
			break
		}
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("after wrong passwords: %v, want limited", err)
	}
	invitation, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, invitation.Secret, "", "remembered", laptop); err != nil {
		t.Errorf("registering from a limited address: %v, want it to work (D221 §4)", err)
	}
	if _, err = fixture.service.SignIn(ctx, "partner", "remembered", "192.0.2.50", laptop); err != nil {
		t.Errorf("signing in after the new link: %v, want the account no longer limited", err)
	}
}

func TestAuthenticate_SaysASessionWasEndedRatherThanUnknown(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	_, television := phoneAndTelevision(t, fixture.service, id)
	listed, err := fixture.service.Authenticate(ctx, television)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.RemoveDevice(ctx, id, listed.DeviceID); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.Authenticate(ctx, television)
	if !errors.Is(err, ErrSessionEnded) || !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("a removed device's session: %v, want ErrSessionEnded", err)
	}
	if _, err = fixture.service.Authenticate(ctx, "not-a-token"); errors.Is(err, ErrSessionEnded) ||
		!errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("an unknown token: %v, want only ErrNotAuthenticated", err)
	}
}

func TestRenameUser_KeepsNamesUnique(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	fixture.createUser(t, "owner", "pw", RoleAdmin)
	if err := fixture.service.RenameUser(ctx, id, "Owner"); !errors.Is(err, ErrNameTaken) {
		t.Errorf("renaming to another's name: %v, want ErrNameTaken", err)
	}
	if err := fixture.service.RenameUser(ctx, id, "Sam"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.SignIn(ctx, "Sam", "pw", "192.0.2.1", laptop); err != nil {
		t.Errorf("signing in by the new name: %v", err)
	}
}

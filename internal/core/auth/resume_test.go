// SPDX-License-Identifier: GPL-3.0-or-later

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
	resumed := NewDevice{Name: "Firefox on Linux", Platform: "web", PreviousDeviceID: before.DeviceID}
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

func TestDeviceCode_OneWaitsAtATimeAndACodeForAListedDeviceSignsItBackIn(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	phone, television := phoneAndTelevision(t, fixture.service, id)
	listed, err := fixture.service.Authenticate(ctx, television)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.SignOut(ctx, television); err != nil {
		t.Fatal(err)
	}
	older, err := fixture.service.IssueDeviceCode(ctx, id, "Kitchen tablet", 0)
	if err != nil {
		t.Fatal(err)
	}
	back, err := fixture.service.IssueDeviceCode(ctx, id, "", listed.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := fixture.service.WaitingCodeOf(ctx, id)
	if err != nil || waiting == nil || waiting.DeviceName != "Living room TV" {
		t.Errorf("the waiting code = %+v, %v; want the television's", waiting, err)
	}
	_, err = fixture.service.SignInWithCode(ctx, older.Secret, "web", "192.0.2.3", 0)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("an older code after a newer: %v, want withdrawn (D221 §3)", err)
	}
	again, err := fixture.service.SignInWithCode(ctx, back.Secret, "web", "192.0.2.3", 0)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := fixture.service.Authenticate(ctx, again)
	if err != nil || resumed.DeviceID != listed.DeviceID {
		t.Errorf("the code for the television signed in device %d, %v; want %d (D221 §2)", resumed.DeviceID, err,
			listed.DeviceID)
	}
	if waiting, err = fixture.service.WaitingCodeOf(ctx, id); err != nil || waiting != nil {
		t.Errorf("after it was used, the waiting code = %+v, %v; want none", waiting, err)
	}
	if _, err = fixture.service.Authenticate(ctx, phone); err != nil {
		t.Errorf("the phone: %v", err)
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
		_, _ = fixture.service.SignInWithCode(ctx, "zzzz-zzzz", "web", "198.51.100.9", 0) //nolint:errcheck // failing
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

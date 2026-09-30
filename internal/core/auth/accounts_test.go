// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

func TestIssueRegistrationInvitation_LastsAsLongAsTheSettingSays(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	settingstest.Set(t, fixture.settings, settings.RegistrationInvitationLifeDays, "30")
	_, invitation, err := fixture.service.CreateAccount(t.Context(), "traveller", RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if life := invitation.ExpiresAt.Sub(fixture.clock.Now()); life != 30*24*time.Hour {
		t.Errorf("with registration_invitation_life_days at 30, the invitation lasts %v, want 30 days", life)
	}
}

func TestRegistration_ALinkSetsAPasswordOnceAndSignsTheDeviceIn(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id, invitation, err := fixture.service.CreateAccount(ctx, "partner", RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if invitation.ExpiresAt.Sub(fixture.clock.Now()) != 7*24*time.Hour || len(invitation.Secret) < 40 {
		t.Errorf("the invitation = %+v, want 256 bits lasting seven days (D218 §1)", invitation)
	}
	_, err = fixture.service.SignIn(ctx, "partner", "", "192.0.2.1", laptop)
	if err == nil {
		t.Error("a person who has not registered signed in")
	}
	token, err := fixture.service.Register(ctx, invitation.Secret, "partner-password", laptop)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	principal, err := fixture.service.Authenticate(ctx, token)
	if err != nil || principal.UserID != id || principal.DeviceID == 0 {
		t.Errorf("after registering: %+v, %v; want the person, signed in on a device", principal, err)
	}
	_, err = fixture.service.Register(ctx, invitation.Secret, "other", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("an invitation used twice: %v, want ErrBadInvitation", err)
	}
	_, err = fixture.service.SignIn(ctx, "partner", "partner-password", "192.0.2.1", laptop)
	if err != nil {
		t.Errorf("the password set by the invitation does not sign in: %v", err)
	}
}

func TestRegistration_ANewLinkWithdrawsTheOldAndTheOldPasswordWorksUntilANewOneIsSet(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "forgotten", RoleUser)
	old, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.Register(ctx, old.Secret, "x", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("an older invitation: %v, want withdrawn", err)
	}
	_, err = fixture.service.SignIn(ctx, "partner", "forgotten", "192.0.2.1", laptop)
	if err != nil {
		t.Errorf("the old password before the new one is set: %v, want it to work (D218 §5)", err)
	}
	_, err = fixture.service.Register(ctx, current.Secret, "remembered", laptop)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.SignIn(ctx, "partner", "forgotten", "192.0.2.1", laptop)
	if err == nil {
		t.Error("the old password still works after the new one is set")
	}
	fixture.clock.Advance(time.Second)
	expiring, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(7*24*time.Hour + time.Second)
	_, err = fixture.service.Register(ctx, expiring.Secret, "late", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("an invitation past seven days: %v, want expired", err)
	}
}

func TestDeviceCode_SignsOneDeviceInOnceWithinTenMinutesAsTyped(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	code, err := fixture.service.IssueDeviceCode(ctx, id, "Living room TV", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(code.Secret) != 9 || code.Secret[4] != '-' || code.ExpiresAt.Sub(fixture.clock.Now()) != 10*time.Minute {
		t.Errorf("the code = %+v, want xxxx-xxxx for ten minutes (D218 §3)", code)
	}
	typed := strings.ToUpper(strings.ReplaceAll(code.Secret, "-", " "))
	token, err := fixture.service.SignInWithCode(ctx, typed, "web", "192.0.2.9", 0)
	if err != nil {
		t.Fatalf("signing in with %q: %v", typed, err)
	}
	principal, err := fixture.service.Authenticate(ctx, token)
	if err != nil || principal.UserID != id || principal.DeviceID == 0 {
		t.Errorf("the television is %+v, %v; want the person, on a new device", principal, err)
	}
	_, err = fixture.service.SignInWithCode(ctx, code.Secret, "web", "192.0.2.9", 0)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("a code used twice: %v, want ErrBadInvitation", err)
	}
	late, err := fixture.service.IssueDeviceCode(ctx, id, "Bedroom TV", 0)
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(10*time.Minute + time.Second)
	_, err = fixture.service.SignInWithCode(ctx, late.Secret, "web", "192.0.2.9", 0)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("a code past ten minutes: %v, want expired", err)
	}
}

func TestDeviceCode_WrongCodesCountAgainstTheSignInLimit(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	code, err := fixture.service.IssueDeviceCode(ctx, id, "TV", 0)
	if err != nil {
		t.Fatal(err)
	}
	var last error
	for attempt := range 20 {
		if _, last = fixture.service.SignInWithCode(ctx, "0000-000"+string(rune('0'+attempt%10)), "web",
			"203.0.113.5", 0); errors.Is(last, ErrRateLimited) {
			break
		}
	}
	if !errors.Is(last, ErrRateLimited) {
		t.Fatalf("twenty wrong codes: %v, want the address limited", last)
	}
	_, err = fixture.service.SignInWithCode(ctx, code.Secret, "web", "203.0.113.5", 0)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("the right code from a limited address: %v, want limited", err)
	}
}

func TestChangePassword_NeedsTheCurrentOneAndKeepsOtherSessions(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "old", RoleUser)
	other, err := fixture.service.SignIn(ctx, "partner", "old", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.service.ChangePassword(ctx, id, "wrong", "new")
	if !errors.Is(err, ErrWrongPassword) || errors.Is(err, ErrBadCredentials) {
		t.Errorf("with the wrong current password: %v, want ErrWrongPassword, not a failed sign-in", err)
	}
	err = fixture.service.ChangePassword(ctx, id, "old", "new")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.SignIn(ctx, "partner", "new", "192.0.2.1", laptop)
	if err != nil {
		t.Errorf("the new password: %v", err)
	}
	_, err = fixture.service.Authenticate(ctx, other)
	if err != nil {
		t.Errorf("another session after the change: %v, want still signed in (D218 §6)", err)
	}
}

func TestChangePassword_OfNoOneIsNoSuchPerson(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	if err := fixture.service.ChangePassword(t.Context(), 999, "old", "new"); !errors.Is(err, ErrNoUser) {
		t.Errorf("the password of a person who is not there: %v, want ErrNoUser", err)
	}
}

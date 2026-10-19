// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

func TestIssueRegistrationInvitation_LastsAsLongAsTheSettingSays(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	settingstest.Set(t, fixture.settings, settings.RegistrationInvitationLifeDays, "30")
	id := fixture.createUser(t, "traveller", "forgotten", RoleUser)
	invitation, err := fixture.service.IssueRegistrationInvitation(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if life := invitation.ExpiresAt.Sub(fixture.clock.Now()); life != 30*24*time.Hour {
		t.Errorf("with registration_invitation_life_days at 30, the invitation lasts %v, want 30 days", life)
	}
}

// An invitation makes the account its person fills in, under the invitation's role and with the
// admin's note, once, and signs the device in; a name already taken spends nothing (D246 §2).
func TestInvitation_MakesTheAccountItsPersonFillsInOnceAndSignsTheDeviceIn(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "owner", "owner-password", RoleAdmin)
	_, _, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: " "})
	if !errors.Is(err, ErrNoNote) {
		t.Errorf("an invitation with no note: %v, want ErrNoNote", err)
	}
	_, link, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "Grandma"})
	if err != nil {
		t.Fatal(err)
	}
	if link.ExpiresAt.Sub(fixture.clock.Now()) != 7*24*time.Hour || len(link.Secret) < 40 {
		t.Errorf("the invitation = %+v, want 256 bits lasting seven days (D218 §1)", link)
	}
	if kind, kindErr := fixture.service.KindOfLink(ctx, link.Secret); kindErr != nil || !kind.IsInvitation {
		t.Errorf("the invitation's link reads as %+v, %v; want an invitation", kind, kindErr)
	}
	if _, err = fixture.service.Register(ctx, link.Secret, "OWNER", "pw", laptop); !errors.Is(err, ErrNameTaken) {
		t.Errorf("an invitation filled in with a name taken: %v, want ErrNameTaken", err)
	}
	token, err := tokenOf(fixture.service.Register(ctx, link.Secret, "Gran", "gran-password", laptop))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	principal, err := fixture.service.Authenticate(ctx, token)
	if err != nil || principal.Name != "Gran" || principal.Role != RoleUser || principal.DeviceID == 0 {
		t.Errorf("after registering: %+v, %v; want Gran, a user, signed in on a device", principal, err)
	}
	if _, err = fixture.service.Register(ctx, link.Secret, "Another", "pw", laptop); !errors.Is(err, ErrBadInvitation) {
		t.Errorf("an invitation used twice: %v, want ErrBadInvitation", err)
	}
	wantNoteKeptAndNoInvitationLeft(t, fixture.service, "Gran", "Grandma")
}

// wantNoteKeptAndNoInvitationLeft fails unless name's account kept the invitation's note, and no
// invitation is left waiting.
func wantNoteKeptAndNoInvitationLeft(t *testing.T, service *Service, name, note string) {
	t.Helper()
	accounts, err := service.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.Name == name && account.AdminNote != note {
			t.Errorf("%s's account = %+v; want the invitation's note %q kept", name, account, note)
		}
	}
	if pending, listErr := service.Invitations(t.Context()); listErr != nil || len(pending) != 0 {
		t.Errorf("invitations after the one was used: %+v, %v; want none", pending, listErr)
	}
}

// A withdrawn invitation admits no one, and a role deleted withdraws its invitations (D246 §2).
func TestInvitation_AWithdrawnOneAndOneOfADeletedRoleAdmitNoOne(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id, link, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "Uncle Joe"})
	if err != nil {
		t.Fatal(err)
	}
	if pending, listErr := fixture.service.Invitations(ctx); listErr != nil || len(pending) != 1 ||
		pending[0].AdminNote != "Uncle Joe" || pending[0].Role != RoleUser {
		t.Fatalf("the invitations = %+v, %v; want Uncle Joe's, a user's", pending, listErr)
	}
	if err = fixture.service.WithdrawInvitation(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, link.Secret, "Joe", "pw", laptop); !errors.Is(err, ErrBadInvitation) {
		t.Errorf("a withdrawn invitation: %v, want ErrBadInvitation", err)
	}
	roleID, err := fixture.service.CreateRole(ctx, RoleChange{Name: "friends"})
	if err != nil {
		t.Fatal(err)
	}
	_, link, err = fixture.service.Invite(ctx, NewInvitation{Role: "friends", AdminNote: "a friend"})
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.DeleteRole(ctx, roleID); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, link.Secret, "Friend", "pw", laptop); !errors.Is(err, ErrBadInvitation) {
		t.Errorf("an invitation into a deleted role: %v, want ErrBadInvitation", err)
	}
}

func TestRegistration_EveryLinkWorksUntilOneIsUsedAndTheOldPasswordUntilANewOneIsSet(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "forgotten", RoleUser)
	first, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	// Both links sent work: a new one leaves the last live (D318 §1).
	if kind, kindErr := fixture.service.KindOfLink(ctx, first.Secret); kindErr != nil || kind.Name != "partner" {
		t.Errorf("the first link after a second was made: %+v, %v; want it still live", kind, kindErr)
	}
	_, err = fixture.service.SignIn(ctx, "partner", "forgotten", "192.0.2.1", laptop)
	if err != nil {
		t.Errorf("the old password before the new one is set: %v, want it to work (D218 §5)", err)
	}
	_, err = fixture.service.Register(ctx, first.Secret, "", "remembered", laptop)
	if err != nil {
		t.Fatal(err)
	}
	// Using one ends the other (D318 §2), and the old password with it.
	_, err = fixture.service.Register(ctx, second.Secret, "", "taken over", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("the other link after one was used: %v, want ended", err)
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
	_, err = fixture.service.Register(ctx, expiring.Secret, "", "late", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("an invitation past seven days: %v, want expired", err)
	}
}

func TestChangePassword_NeedsTheCurrentOne(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "old", RoleUser)
	asking, err := tokenOf(fixture.service.SignIn(ctx, "partner", "old", "192.0.2.1", laptop))
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.service.ChangePassword(ctx, id, "wrong", "new", asking)
	if !errors.Is(err, ErrWrongPassword) || errors.Is(err, ErrBadCredentials) {
		t.Errorf("with the wrong current password: %v, want ErrWrongPassword, not a failed sign-in", err)
	}
	err = fixture.service.ChangePassword(ctx, id, "old", "new", asking)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.SignIn(ctx, "partner", "new", "192.0.2.1", laptop)
	if err != nil {
		t.Errorf("the new password: %v", err)
	}
}

func TestChangePassword_OfNoOneIsNoSuchPerson(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	if err := fixture.service.ChangePassword(t.Context(), 999, "old", "new", ""); !errors.Is(err, ErrNoUser) {
		t.Errorf("the password of a person who is not there: %v, want ErrNoUser", err)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestIssueRegistrationInvitation_LastsAsLongAsTheSettingSays(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	settingstest.Set(t, fixture.settings, settings.RegistrationInvitationLifeDays, "30")
	// A person with no password yet: a registered one's link is a reset link, with its own life (D466 §3).
	id := fixture.createUnregistered(t, "traveller")
	invitation, err := fixture.service.IssueRegistrationInvitation(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if life := invitation.ExpiresAt.Sub(fixture.clock.Now()); life != 30*24*time.Hour {
		t.Errorf("with registrationInvitationLifeDays at 30, the invitation lasts %v, want 30 days", life)
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
// invitation is listed: the one used is gone from the list, its account is among the accounts (D403).
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
	invitations, listErr := service.Invitations(t.Context())
	if listErr != nil || len(invitations) != 0 {
		t.Errorf("invitations after the one was used: %+v, %v; want none listed", invitations, listErr)
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

// An invitation's link makes the account once, and the invitation is then no longer listed, with
// no link to withdraw (D318 §2; D403).
func TestInvitation_ItsLinkWorksOnceAndThenItIsNoLongerListed(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id, link, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "Uncle Joe"})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := fixture.service.Invitations(ctx)
	if err != nil || len(listed) != 1 || listed[0].LiveLinks != 1 || !listed[0].ExpiresAt.Equal(link.ExpiresAt) {
		t.Fatalf("the invitations = %+v, %v; want one waiting with its link", listed, err)
	}
	fixture.clock.Advance(time.Hour)
	if _, err = fixture.service.Register(ctx, link.Secret, "Joe", "a good long password", laptop); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.Register(ctx, link.Secret, "Joe Again", "pw", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("the link used a second time: %v, want ended", err)
	}
	wantUsedBy(t, fixture.service, id, "Joe")
}

// wantUsedBy fails unless the invitation id is no longer listed, nothing is left to withdraw, and
// name's own link is the one used, still live (D292 §4; D403).
func wantUsedBy(t *testing.T, service *Service, id int64, name string) {
	t.Helper()
	ctx := t.Context()
	listed, err := service.Invitations(ctx)
	if err != nil || len(listed) != 0 {
		t.Fatalf("the invitations after %s's was used = %+v, %v; want none", name, listed, err)
	}
	accounts, err := service.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var madeID int64
	for _, account := range accounts {
		if account.Name == name {
			madeID = account.ID
		}
	}
	if err = service.WithdrawInvitation(ctx, id); !errors.Is(err, ErrInvitationUsed) {
		t.Errorf("withdrawing a used invitation: %v, want ErrInvitationUsed", err)
	}
	if own, hasOwn, ownErr := service.OwnLink(ctx, madeID); ownErr != nil || !hasOwn ||
		own.ExpiresAt.IsZero() {
		t.Errorf("%s's own link = %+v, %v, %v; want the link used, still live", name, own, hasOwn, ownErr)
	}
}

// An invitation whose link ended is still listed, waiting, until withdrawn, and withdrawing ends
// its link (D318 §3).
func TestInvitation_AnExpiredOneIsListedUntilWithdrawn(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id, first, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "Aunt May"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(7*24*time.Hour + time.Second)
	if listed, listErr := fixture.service.Invitations(ctx); listErr != nil || len(listed) != 1 ||
		listed[0].LiveLinks != 0 || !listed[0].ExpiresAt.IsZero() {
		t.Fatalf("an expired invitation = %+v, %v; want it listed, waiting with no live link", listed, listErr)
	}
	if err = fixture.service.WithdrawInvitation(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, first.Secret, "May", "pw", laptop); !errors.Is(err, ErrBadInvitation) {
		t.Errorf("the link of a withdrawn invitation: %v, want ErrBadInvitation", err)
	}
	if listed, listErr := fixture.service.Invitations(ctx); listErr != nil || len(listed) != 0 {
		t.Errorf("after withdrawing, the invitations = %+v, %v; want none", listed, listErr)
	}
}

// A person with no password yet keeps every link sent live until one is used (D318 §1); a registered
// person's reset link, one at a time (D466 §2, reset_link_test.go), leaves the old password working
// until the new one is set (D218 §5).
func TestRegistration_EveryLinkWorksUntilOneIsUsedAndTheOldPasswordUntilANewOneIsSet(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUnregistered(t, "partner")
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
	spouse := fixture.createUser(t, "spouse", "forgotten", RoleUser)
	reset, err := fixture.service.IssueRegistrationInvitation(ctx, spouse)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.SignIn(ctx, "spouse", "forgotten", "192.0.2.1", laptop)
	if err != nil {
		t.Errorf("the old password before the new one is set: %v, want it to work (D218 §5)", err)
	}
	_, err = fixture.service.Register(ctx, first.Secret, "", "remembered", laptop)
	if err != nil {
		t.Fatal(err)
	}
	// Using one ends the other (D318 §2).
	_, err = fixture.service.Register(ctx, second.Secret, "", "taken over", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("the other link after one was used: %v, want ended", err)
	}
	if _, err = fixture.service.Register(ctx, reset.Secret, "", "remembered", laptop); err != nil {
		t.Fatal(err)
	}
	// The new password ends the old.
	_, err = fixture.service.SignIn(ctx, "spouse", "forgotten", "192.0.2.1", laptop)
	if err == nil {
		t.Error("the old password still works after the new one is set")
	}
	fixture.clock.Advance(time.Second)
	expiring, err := fixture.service.IssueRegistrationInvitation(ctx, fixture.createUnregistered(t, "newcomer"))
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

// sealedLinksKept is how many invitation and registration links not yet used keep their secret,
// sealed, in their grant.
func sealedLinksKept(t *testing.T, kit fixture) string {
	t.Helper()
	return strings.Join(storetest.QueryRaw(t, kit.database.Path(),
		`SELECT count(*) FROM "grant" WHERE kind = 'issued_secret' AND link_sealed IS NOT NULL`), "")
}

// sealingFixture is a fixture whose links keep their secrets sealed, as the assembled core's do.
func sealingFixture(t *testing.T) fixture {
	t.Helper()
	kit := newFixture(t)
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kit.service.SealLinksWith(secrets.NewStore(kit.database, key, kit.clock, rand.Reader, kit.settings))
	return kit
}

// inviteFor issues an invitation for note and returns its id and first link.
func inviteFor(t *testing.T, kit fixture, note string) (int64, IssuedSecret) {
	t.Helper()
	id, link, err := kit.service.Invite(t.Context(), NewInvitation{Role: RoleUser, AdminNote: note})
	if err != nil {
		t.Fatal(err)
	}
	return id, link
}

// An invitation's live link is listed to pass on again, opened from its sealed secret, until it is
// used, which forgets the secret at once (D267 §5, as the owner chose for invitations, 2026-10-28).
func TestInvitation_ItsLiveLinkIsShownAgainUntilUsed(t *testing.T) {
	t.Parallel()
	kit := sealingFixture(t)
	ctx := t.Context()
	_, link := inviteFor(t, kit, "the neighbour")
	listed, err := kit.service.Invitations(ctx)
	isSame := func(a, b IssuedSecret) bool { return a.Secret == b.Secret && a.ExpiresAt.Equal(b.ExpiresAt) }
	if err != nil || len(listed) != 1 || !slices.EqualFunc(listed[0].Links, []IssuedSecret{link}, isSame) {
		t.Fatalf("the invitations = %+v, %v; want its link to pass on again", listed, err)
	}
	if _, err = kit.service.Register(ctx, link.Secret, "Ned", "a good long password", laptop); err != nil {
		t.Fatal(err)
	}
	if listed, err = kit.service.Invitations(ctx); err != nil || len(listed) != 0 {
		t.Errorf("the used invitation = %+v, %v; want it not listed, and no link shown", listed, err)
	}
	if kept := sealedLinksKept(t, kit); kept != "0" {
		t.Errorf("%s links keep their secret after the invitation was used; want none", kept)
	}
}

// Withdrawing an invitation forgets its links' secrets at once, and a link that ended, after
// registrationInvitationLifeDays (seven to start), is no longer shown (D267 §5; the owner,
// 2026-10-28: "unused invitation links expire after 7 days").
func TestInvitation_AWithdrawnOrEndedLinkIsNoLongerShown(t *testing.T) {
	t.Parallel()
	kit := sealingFixture(t)
	ctx := t.Context()
	withdrawn, _ := inviteFor(t, kit, "the cousin")
	expiring, _ := inviteFor(t, kit, "the aunt")
	if err := kit.service.WithdrawInvitation(ctx, withdrawn); err != nil {
		t.Fatal(err)
	}
	if kept := sealedLinksKept(t, kit); kept != "1" {
		t.Errorf("%s unused links keep their secret after one invitation was withdrawn; want the other's alone", kept)
	}
	if listed, err := kit.service.Invitations(ctx); err != nil || len(listed) != 1 || len(listed[0].Links) != 1 {
		t.Fatalf("the invitations = %+v, %v; want the aunt's with its link", listed, err)
	}
	kit.clock.Advance(7*24*time.Hour + time.Second)
	listed, err := kit.service.Invitations(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != expiring || len(listed[0].Links) != 0 {
		t.Errorf("after 7 days the invitations = %+v, %v; want the aunt's waiting with no link shown", listed, err)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// A registered person has one reset link at a time: making one cancels the outstanding one in the
// same step, and the new one sets the password (D466 §2).
func TestResetLink_ANewOneCancelsTheOutstandingOne(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "forgotten", RoleUser)
	first, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(time.Minute)
	second, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.Register(ctx, first.Secret, "", "taken over", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("the reset link a new one replaced: %v, want ErrBadInvitation", err)
	}
	if expiresAt, isOutstanding, readErr := fixture.service.ResetLink(ctx, id); readErr != nil || !isOutstanding ||
		!expiresAt.Equal(second.ExpiresAt) {
		t.Errorf("the outstanding reset link = %v, %v, %v; want the new one's end, %v",
			expiresAt, isOutstanding, readErr, second.ExpiresAt)
	}
	if _, err = fixture.service.Register(ctx, second.Secret, "", "remembered", laptop); err != nil {
		t.Errorf("the new reset link: %v, want it to set the password", err)
	}
	if _, err = fixture.service.SignIn(ctx, "partner", "remembered", "192.0.2.1", laptop); err != nil {
		t.Errorf("the password the new reset link set: %v, want it to work", err)
	}
}

// A reset link lives passwordResetLinkLifeMinutes, an hour to start, apart from a first invitation's
// registrationInvitationLifeDays (D466 §3).
func TestResetLink_LastsAsLongAsItsOwnSettingSays(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "traveller", "forgotten", RoleUser)
	started, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if life := started.ExpiresAt.Sub(fixture.clock.Now()); life != time.Hour {
		t.Errorf("a reset link at the starting setting lasts %v, want an hour", life)
	}
	settingstest.Set(t, fixture.settings, settings.RegistrationInvitationLifeDays, "30")
	settingstest.Set(t, fixture.settings, settings.PasswordResetLinkLifeMinutes, "90")
	set, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if life := set.ExpiresAt.Sub(fixture.clock.Now()); life != 90*time.Minute {
		t.Errorf("with passwordResetLinkLifeMinutes at 90, a reset link lasts %v, want 90 minutes", life)
	}
	fixture.clock.Advance(90*time.Minute + time.Second)
	if _, err = fixture.service.Register(ctx, set.Secret, "", "late", laptop); !errors.Is(err, ErrBadInvitation) {
		t.Errorf("a reset link past its life: %v, want ErrBadInvitation", err)
	}
	if _, isOutstanding, readErr := fixture.service.ResetLink(ctx, id); readErr != nil || isOutstanding {
		t.Errorf("an ended reset link reads as outstanding: %v, %v; want none", isOutstanding, readErr)
	}
}

// The person's page is told until when a reset link is outstanding, and nothing for a person with
// none or with no password yet, whose registration links are not reset links (D466 §4).
func TestResetLink_IsOutstandingUntilUsedOrCancelled(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "forgotten", RoleUser)
	if _, isOutstanding, err := fixture.service.ResetLink(ctx, id); err != nil || isOutstanding {
		t.Errorf("before any reset link: %v, %v; want none outstanding", isOutstanding, err)
	}
	issued, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if expiresAt, isOutstanding, readErr := fixture.service.ResetLink(ctx, id); readErr != nil || !isOutstanding ||
		!expiresAt.Equal(issued.ExpiresAt) {
		t.Errorf("a reset link just made reads as %v, %v, %v; want outstanding until %v",
			expiresAt, isOutstanding, readErr, issued.ExpiresAt)
	}
	if _, err = fixture.service.Register(ctx, issued.Secret, "", "remembered", laptop); err != nil {
		t.Fatal(err)
	}
	if _, isOutstanding, readErr := fixture.service.ResetLink(ctx, id); readErr != nil || isOutstanding {
		t.Errorf("a used reset link reads as outstanding: %v, %v; want none", isOutstanding, readErr)
	}
	newcomer := fixture.createUnregistered(t, "newcomer")
	if _, err = fixture.service.IssueRegistrationInvitation(ctx, newcomer); err != nil {
		t.Fatal(err)
	}
	if _, isOutstanding, readErr := fixture.service.ResetLink(ctx, newcomer); readErr != nil || isOutstanding {
		t.Errorf("a person with no password reads as having a reset link: %v, %v; want none",
			isOutstanding, readErr)
	}
	if _, _, readErr := fixture.service.ResetLink(ctx, 99); !errors.Is(readErr, ErrNoUser) {
		t.Errorf("the reset link of no one: %v, want ErrNoUser", readErr)
	}
}

// Cancelling a reset link ends it, and the person's password and sessions stay as they were (D318
// §3, D466 §4).
func TestCancelResetLink_EndsItLeavingThePasswordAndSessions(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "kept", RoleUser)
	session, err := fixture.service.SignIn(ctx, "partner", "kept", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.CancelResetLink(ctx, id); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.Register(ctx, issued.Secret, "", "taken over", laptop)
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("a cancelled reset link: %v, want ErrBadInvitation", err)
	}
	if _, isOutstanding, readErr := fixture.service.ResetLink(ctx, id); readErr != nil || isOutstanding {
		t.Errorf("a cancelled reset link reads as outstanding: %v, %v; want none", isOutstanding, readErr)
	}
	if _, err = fixture.service.SignIn(ctx, "partner", "kept", "192.0.2.1", laptop); err != nil {
		t.Errorf("the password after a cancelled reset link: %v, want it to work", err)
	}
	if _, err = fixture.service.Authenticate(ctx, session.Token); err != nil {
		t.Errorf("the session after a cancelled reset link: %v, want it signed in still", err)
	}
	if err = fixture.service.CancelResetLink(ctx, 99); !errors.Is(err, ErrNoUser) {
		t.Errorf("cancelling no one's reset link: %v, want ErrNoUser", err)
	}
}

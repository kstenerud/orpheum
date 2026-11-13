// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNoNote is an invitation without the admin's note of who it is for, which it must have (D246 §2).
var ErrNoNote = errors.New("an invitation needs a note of who it is for")

// ErrNoInvitation is an invitation that is not there.
var ErrNoInvitation = errors.New("no such invitation")

// ErrInvitationUsed is an invitation whose link was used: it made its account, and has no link to
// send again or withdraw.
var ErrInvitationUsed = errors.New("that invitation was used: it made its account")

// NewInvitation is an invitation the admin issues: the role the account will have, and the admin's
// note of who it is for, which only the admin sees (D246 §2).
type NewInvitation struct {
	Role      Role
	AdminNote string
}

// Invitation is an invitation as the admin's list shows it: one still waiting, with its links still
// live and when the last of them ends; one used is not listed, its account is (D403).
type Invitation struct {
	ID        int64
	Role      Role
	AdminNote string
	IssuedAt  time.Time
	// LiveLinks are how many of its links still work; ExpiresAt is when the last of them ends, the
	// zero time when none does.
	LiveLinks int
	ExpiresAt time.Time
	// Links are its live links while it waits, oldest first, each to pass on again until it is
	// used, withdrawn or ends (D267 §5, which the owner chose for invitations, 2026-10-28).
	Links []IssuedSecret
}

// Invite issues an invitation with its first link: single use, lasting
// registrationInvitationLifeDays, for a person to make their own account under role, filling in
// their own name and password (D246 §2). It returns the invitation's id and its link's secret.
func (service *Service) Invite(ctx context.Context, invitation NewInvitation) (int64, IssuedSecret, error) {
	note := textindex.Line(invitation.AdminNote)
	if note == "" {
		return 0, IssuedSecret{}, ErrNoNote
	}
	var id int64
	issued, err := service.issueInvitationLink(ctx, func(queries *sqlcgen.Queries, now time.Time) (int64, error) {
		role, roleErr := store.OrNotFound(queries.GetRoleByName(ctx, string(invitation.Role)))
		if errors.Is(roleErr, store.ErrNotFound) {
			return 0, ErrNoRole
		}
		if roleErr != nil {
			return 0, fmt.Errorf("find role %q: %w", invitation.Role, roleErr)
		}
		var insertErr error
		id, insertErr = queries.InsertInvitation(ctx, sqlcgen.InsertInvitationParams{
			RoleID: role.ID, AdminNote: note, IssuedAt: now.UnixMilli(),
		})
		if insertErr != nil {
			return 0, fmt.Errorf("record the invitation: %w", insertErr)
		}
		return id, nil
	})
	if err != nil {
		return 0, IssuedSecret{}, err
	}
	return id, issued, nil
}

// issueInvitationLink makes a link for the invitation invitationOf names, in the same write, and
// returns its secret.
func (service *Service) issueInvitationLink(
	ctx context.Context, invitationOf func(queries *sqlcgen.Queries, now time.Time) (int64, error),
) (IssuedSecret, error) {
	secret, err := randomSecret(service.random)
	if err != nil {
		return IssuedSecret{}, err
	}
	life, err := service.settings.Duration(ctx, settings.RegistrationInvitationLifeDays)
	if err != nil {
		return IssuedSecret{}, fmt.Errorf("read registrationInvitationLifeDays: %w", err)
	}
	kept, err := service.keepOfLink(secret)
	if err != nil {
		return IssuedSecret{}, err
	}
	now := service.clock.Now()
	expires := now.Add(life)
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		invitationID, ofErr := invitationOf(queries, now)
		if ofErr != nil {
			return ofErr
		}
		if insertErr := queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			SecretSha256: tokenHash(secret), Kind: grantKindIssuedSecret, Scope: grantScopeInvitation,
			IssuedAt: now.UnixMilli(), ExpiresAt: new(expires.UnixMilli()),
			LocateHash: kept.locateHash, OfferKey: kept.offerKey,
			LinkSealed: kept.sealed,
		}); insertErr != nil {
			return fmt.Errorf("record the invitation's link: %w", insertErr)
		}
		return queries.InsertInvitationLink(ctx, sqlcgen.InsertInvitationLinkParams{
			SecretSha256: tokenHash(secret), InvitationID: invitationID,
		})
	})
	if err != nil {
		return IssuedSecret{}, err
	}
	service.linksChanged()
	return IssuedSecret{Secret: secret, ExpiresAt: expires}, nil
}

// Invitations are the invitations still waiting, newest first, with their links; one used is gone
// from the list, the account it made is among the accounts (D403).
func (service *Service) Invitations(ctx context.Context) ([]Invitation, error) {
	return service.InvitationsBefore(ctx, InvitationsFirst(), -1)
}

// InvitationsFirst is where the first page of the invitations starts: before every one.
func InvitationsFirst() InvitationPlace {
	return InvitationPlace{IssuedAt: math.MaxInt64, ID: math.MaxInt64}
}

// InvitationPlace is an invitation's place in the list, newest first: when it was issued, Unix
// milliseconds, and its id.
type InvitationPlace struct {
	IssuedAt int64
	ID       int64
}

// InvitationsBefore is a page of the invitations still waiting, newest first, with their links: at
// most limit (-1 for every one) placed after before, InvitationsFirst for the first page.
func (service *Service) InvitationsBefore(
	ctx context.Context, before InvitationPlace, limit int64,
) ([]Invitation, error) {
	now := new(service.clock.Now().UnixMilli())
	rows, err := service.database.Read().ListInvitations(ctx, sqlcgen.ListInvitationsParams{
		Now: now, BeforeIssuedAt: before.IssuedAt, BeforeID: before.ID, MaxRows: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	links, err := service.liveInvitationLinks(ctx, now)
	if err != nil {
		return nil, err
	}
	invitations := make([]Invitation, 0, len(rows))
	for _, row := range rows {
		invitation := Invitation{
			ID: row.ID, Role: Role(row.RoleName), AdminNote: row.AdminNote, IssuedAt: time.UnixMilli(row.IssuedAt),
			LiveLinks: int(row.LiveLinks), Links: links[row.ID],
		}
		if row.LiveLinks > 0 {
			invitation.ExpiresAt = time.UnixMilli(row.LastExpiresAt)
		}
		invitations = append(invitations, invitation)
	}
	return invitations, nil
}

// liveInvitationLinks are the live links of the invitations still waiting, by invitation, each
// opened from its sealed secret; a link that does not open is left out, still live, and another
// can be sent.
func (service *Service) liveInvitationLinks(ctx context.Context, now *int64) (map[int64][]IssuedSecret, error) {
	rows, err := service.database.Read().ListLiveInvitationLinks(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("list the invitations' live links: %w", err)
	}
	links := map[int64][]IssuedSecret{}
	for _, row := range rows {
		secret := service.openLink(row.SecretSha256, row.LinkSealed)
		if secret == "" || row.ExpiresAt == nil {
			continue
		}
		links[row.InvitationID] = append(links[row.InvitationID], IssuedSecret{
			Secret: secret, ExpiresAt: time.UnixMilli(*row.ExpiresAt),
		})
	}
	return links, nil
}

// WithdrawInvitation withdraws an invitation still waiting: every link of it admits no one from now
// on (D318 §3), and it leaves the list. A used one is refused, as it made its account.
func (service *Service) WithdrawInvitation(ctx context.Context, id int64) error {
	now := service.clock.Now().UnixMilli()
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		invitation, err := store.OrNotFound(queries.GetInvitation(ctx, id))
		switch {
		case errors.Is(err, store.ErrNotFound):
			return ErrNoInvitation
		case err != nil:
			return fmt.Errorf("look up invitation %d: %w", id, err)
		case invitation.MadeUserID != nil:
			return ErrInvitationUsed
		}
		if err = queries.RevokeInvitationLinks(ctx, sqlcgen.RevokeInvitationLinksParams{
			RevokedAt: &now, InvitationID: id,
		}); err != nil {
			return fmt.Errorf("withdraw invitation %d's links: %w", id, err)
		}
		if err = queries.DeleteInvitationLinks(ctx, id); err != nil {
			return fmt.Errorf("forget invitation %d's links: %w", id, err)
		}
		return queries.DeleteInvitation(ctx, id)
	})
	if err == nil {
		service.linksChanged()
	}
	return err
}

// LinkKind is what a link is for, so the page that opened it asks for what it needs: an invitation,
// which asks for a name and a password, or a registration link, which asks for a new password for
// the person it names. ErrBadInvitation when it is neither, or no longer works: an unknown, used,
// cancelled, replaced and expired link are answered alike, naming no one.
type LinkKind struct {
	IsInvitation bool
	// Name is a registration link's person; empty for an invitation.
	Name string
	// IsReset is whether a registration link's person already has a password, so the link is a
	// reset link and sets a new one (D466); false for a first registration and an invitation.
	IsReset bool
}

// KindOfLink says what a link is for, without spending it.
func (service *Service) KindOfLink(ctx context.Context, secret string) (LinkKind, error) {
	grant, err := store.OrNotFound(service.database.Read().GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
		SecretSha256: tokenHash(secret), Now: new(service.clock.Now().UnixMilli()),
	}))
	if errors.Is(err, store.ErrNotFound) || (err == nil && grant.Kind != grantKindIssuedSecret) {
		return LinkKind{}, ErrBadInvitation
	}
	if err != nil {
		return LinkKind{}, fmt.Errorf("look up a link: %w", err)
	}
	switch {
	case grant.Scope == grantScopeInvitation:
		return LinkKind{IsInvitation: true}, nil
	case grant.Scope == grantScopeRegister && grant.UserID != nil:
		user, userErr := liveUser(ctx, service.database.Read(), *grant.UserID)
		if userErr != nil {
			return LinkKind{}, ErrBadInvitation
		}
		return LinkKind{Name: user.Name, IsReset: user.PasswordHash != nil}, nil
	}
	return LinkKind{}, ErrBadInvitation
}

// isInvitation is whether secret is a live invitation's.
func (service *Service) isInvitation(ctx context.Context, secret string) (bool, error) {
	kind, err := service.KindOfLink(ctx, secret)
	if errors.Is(err, ErrBadInvitation) {
		return false, nil // spend says it is not valid
	}
	return kind.IsInvitation, err
}

// acceptInvitation makes the invitation's account, with name and password, signs it in on device,
// and spends the invitation; a name already taken spends nothing, so the person can choose another.
func (service *Service) acceptInvitation(
	ctx context.Context, secret, name, password string, device NewDevice,
) (Issued, error) {
	name = textindex.Line(name)
	if name == "" {
		return Issued{}, errors.New("a person needs a name")
	}
	hash, err := service.hashWithCurrentCosts(ctx, password)
	if err != nil {
		return Issued{}, err
	}
	now := service.clock.Now().UnixMilli()
	secretHash := tokenHash(secret)
	var userID int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		grant, invitation, lookErr := liveInvitationOf(ctx, queries, secretHash, now)
		if lookErr != nil {
			return lookErr
		}
		if takenErr := refuseTakenName(ctx, queries, name, 0); takenErr != nil {
			return takenErr
		}
		var insertErr error
		userID, insertErr = queries.InsertUser(ctx, sqlcgen.InsertUserParams{
			Name: name, RoleID: invitation.RoleID, PasswordHash: hash, AdminNote: &invitation.AdminNote,
			CreatedAt: now,
		})
		if insertErr != nil {
			return fmt.Errorf("insert user %q: %w", name, insertErr)
		}
		return spendInvitation(ctx, queries, spentLink{
			invitationID: invitation.ID, secretHash: secretHash, locateHash: grant.LocateHash, userID: userID, now: now,
		})
	})
	if err != nil {
		return Issued{}, err
	}
	if keepErr := service.keepRealmKey(ctx, userID, name, password, true); keepErr != nil {
		return Issued{}, keepErr
	}
	return service.issueSession(ctx, userID, device, grantScopeRole)
}

// liveInvitationOf is the live link secretHash names and its invitation still waiting, or
// ErrBadInvitation.
func liveInvitationOf(
	ctx context.Context, queries *sqlcgen.Queries, secretHash []byte, now int64,
) (sqlcgen.GetLiveGrantRow, sqlcgen.GetInvitationBySecretRow, error) {
	grant, err := store.OrNotFound(queries.GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
		SecretSha256: secretHash, Now: &now,
	}))
	if errors.Is(err, store.ErrNotFound) || (err == nil && grant.Scope != grantScopeInvitation) {
		return grant, sqlcgen.GetInvitationBySecretRow{}, ErrBadInvitation
	}
	if err != nil {
		return grant, sqlcgen.GetInvitationBySecretRow{}, fmt.Errorf("look up the invitation: %w", err)
	}
	invitation, err := store.OrNotFound(queries.GetInvitationBySecret(ctx, secretHash))
	if errors.Is(err, store.ErrNotFound) || (err == nil && invitation.MadeUserID != nil) {
		return grant, invitation, ErrBadInvitation
	}
	if err != nil {
		return grant, invitation, fmt.Errorf("look up the invitation: %w", err)
	}
	return grant, invitation, nil
}

// spentLink is an invitation's link being used: the invitation, the link's secret's and locate
// hashes, the account it made, and when (Unix ms).
type spentLink struct {
	invitationID           int64
	secretHash, locateHash []byte
	userID, now            int64
}

// spendInvitation spends an invitation by one of its links: that link becomes the new person's own
// link (D292 §4; A12), its other links end in the same write (D318 §2), and it names the account it
// made (D311 §4).
func spendInvitation(ctx context.Context, queries *sqlcgen.Queries, spent spentLink) error {
	now, userID := spent.now, spent.userID
	if err := becomeOwnLink(ctx, queries, spent.secretHash, spent.locateHash, userID, time.UnixMilli(now)); err != nil {
		return fmt.Errorf("spend the invitation: %w", err)
	}
	if err := queries.RevokeInvitationLinks(ctx, sqlcgen.RevokeInvitationLinksParams{
		RevokedAt: &now, InvitationID: spent.invitationID,
	}); err != nil {
		return fmt.Errorf("end the invitation's other links: %w", err)
	}
	return queries.MarkInvitationMade(ctx, sqlcgen.MarkInvitationMadeParams{
		MadeUserID: &userID, MadeAt: &now, ID: spent.invitationID,
	})
}

// SetAdminNote changes the admin's note of who a person is for; empty removes it (D246 §2).
func (service *Service) SetAdminNote(ctx context.Context, userID int64, note string) error {
	note = textindex.Line(note)
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := liveUser(ctx, queries, userID); err != nil {
			return err
		}
		return queries.SetUserAdminNote(ctx, sqlcgen.SetUserAdminNoteParams{AdminNote: ptr.NilIfZero(note), ID: userID})
	})
}

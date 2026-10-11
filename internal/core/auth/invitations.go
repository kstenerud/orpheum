// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNoNote is an invitation without the admin's note of who it is for, which it must have (D246 §2).
var ErrNoNote = errors.New("an invitation needs a note of who it is for")

// ErrNoInvitation is an invitation that is not there, or no longer waits.
var ErrNoInvitation = errors.New("no such invitation")

// NewInvitation is an invitation the admin issues: the role the account will have, and the admin's
// note of who it is for, which only the admin sees (D246 §2).
type NewInvitation struct {
	Role      Role
	AdminNote string
}

// Invitation is an invitation not yet used, as the admin's list shows it.
type Invitation struct {
	ID        int64
	Role      Role
	AdminNote string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// Invite issues an invitation: a single-use link, which lasts registration_invitation_life_days,
// for a person to make their own account under role, filling in their own name and password
// (D246 §2). It returns the invitation's id and its link's secret.
func (service *Service) Invite(ctx context.Context, invitation NewInvitation) (int64, IssuedSecret, error) {
	note := textindex.Line(invitation.AdminNote)
	if note == "" {
		return 0, IssuedSecret{}, ErrNoNote
	}
	secret, err := randomSecret(service.random)
	if err != nil {
		return 0, IssuedSecret{}, err
	}
	life, err := service.settings.Duration(ctx, settings.RegistrationInvitationLifeDays)
	if err != nil {
		return 0, IssuedSecret{}, fmt.Errorf("read registration_invitation_life_days: %w", err)
	}
	now := service.clock.Now()
	expires := now.Add(life)
	var id int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		role, roleErr := store.OrNotFound(queries.GetRoleByName(ctx, string(invitation.Role)))
		if errors.Is(roleErr, store.ErrNotFound) {
			return ErrNoRole
		}
		if roleErr != nil {
			return fmt.Errorf("find role %q: %w", invitation.Role, roleErr)
		}
		if insertErr := queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			SecretSha256: tokenHash(secret), Kind: grantKindIssuedSecret, Scope: grantScopeInvitation,
			IssuedAt: now.UnixMilli(), ExpiresAt: new(expires.UnixMilli()),
		}); insertErr != nil {
			return fmt.Errorf("record the invitation's link: %w", insertErr)
		}
		var insertErr error
		id, insertErr = queries.InsertInvitation(ctx, sqlcgen.InsertInvitationParams{
			SecretSha256: tokenHash(secret), RoleID: role.ID, AdminNote: note,
		})
		return insertErr
	})
	if err != nil {
		return 0, IssuedSecret{}, err
	}
	return id, IssuedSecret{Secret: secret, ExpiresAt: expires}, nil
}

// Invitations are the invitations whose link still works, newest first.
func (service *Service) Invitations(ctx context.Context) ([]Invitation, error) {
	rows, err := service.database.Read().ListLiveInvitations(ctx, new(service.clock.Now().UnixMilli()))
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	invitations := make([]Invitation, 0, len(rows))
	for _, row := range rows {
		invitations = append(invitations, Invitation{
			ID: row.ID, Role: Role(row.RoleName), AdminNote: row.AdminNote, IssuedAt: time.UnixMilli(row.IssuedAt),
			ExpiresAt: time.UnixMilli(ptr.ValueOrZero(row.ExpiresAt)),
		})
	}
	return invitations, nil
}

// WithdrawInvitation withdraws an invitation: its link admits no one from now on.
func (service *Service) WithdrawInvitation(ctx context.Context, id int64) error {
	now := service.clock.Now().UnixMilli()
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		invitation, err := store.OrNotFound(queries.GetInvitation(ctx, id))
		if errors.Is(err, store.ErrNotFound) {
			return ErrNoInvitation
		}
		if err != nil {
			return fmt.Errorf("look up invitation %d: %w", id, err)
		}
		if err = queries.MarkGrantRevoked(ctx, sqlcgen.MarkGrantRevokedParams{
			SecretSha256: invitation.SecretSha256, RevokedAt: &now,
		}); err != nil {
			return fmt.Errorf("withdraw invitation %d's link: %w", id, err)
		}
		return queries.DeleteInvitation(ctx, id)
	})
}

// LinkKind is what a link is for, so the page that opened it asks for what it needs: an invitation,
// which asks for a name and a password, or a registration link, which asks for a new password for
// the person it names. ErrBadInvitation when it is neither, or no longer works.
type LinkKind struct {
	IsInvitation bool
	// Name is a registration link's person; empty for an invitation.
	Name string
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
		return LinkKind{Name: user.Name}, nil
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
) (string, error) {
	name = textindex.Line(name)
	if name == "" {
		return "", errors.New("a person needs a name")
	}
	hash, err := service.hashWithCurrentCosts(ctx, password)
	if err != nil {
		return "", err
	}
	now := service.clock.Now().UnixMilli()
	secretHash := tokenHash(secret)
	var userID int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		grant, lookErr := store.OrNotFound(queries.GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
			SecretSha256: secretHash, Now: &now,
		}))
		if errors.Is(lookErr, store.ErrNotFound) || (lookErr == nil && grant.Scope != grantScopeInvitation) {
			return ErrBadInvitation
		}
		if lookErr != nil {
			return fmt.Errorf("look up the invitation: %w", lookErr)
		}
		invitation, lookErr := store.OrNotFound(queries.GetInvitationBySecret(ctx, secretHash))
		if errors.Is(lookErr, store.ErrNotFound) {
			return ErrBadInvitation
		}
		if lookErr != nil {
			return fmt.Errorf("look up the invitation: %w", lookErr)
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
		if revokeErr := queries.MarkGrantRevoked(ctx, sqlcgen.MarkGrantRevokedParams{
			SecretSha256: secretHash, RevokedAt: &now,
		}); revokeErr != nil {
			return fmt.Errorf("spend the invitation: %w", revokeErr)
		}
		return queries.DeleteInvitation(ctx, invitation.ID)
	})
	if err != nil {
		return "", err
	}
	return service.issueSession(ctx, userID, device)
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

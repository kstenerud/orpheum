// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The invitations D218 makes: grants of kind issued_secret, a secret handed to a person to pass on,
// single use, each in its own scope.
const (
	grantKindIssuedSecret = "issued_secret"
	// A registration link, which lets its person set a new password (D218 §5).
	grantScopeRegister = "register"
	// An invitation, which makes an account under a role (D246 §2).
	grantScopeInvitation = "invitation"
)

var (
	// ErrBadInvitation is a registration invitation that is unknown, used, expired or withdrawn,
	// without saying which.
	ErrBadInvitation = errors.New("the link or code is not valid; ask for a new one")
	// ErrNoUser is a person who is not there.
	ErrNoUser = errors.New("no such person")
	// ErrRemoveSelf is an admin removing themselves, which would also leave the server without one.
	ErrRemoveSelf = errors.New("you cannot remove yourself; another admin can")
)

// IssuedSecret is a registration invitation's secret, and when it stops working.
type IssuedSecret struct {
	Secret    string
	ExpiresAt time.Time
}

// Account is one person, as the admin's list shows them: with the admin's note of who they are
// (D246 §2).
type Account struct {
	ID          int64
	Name        string
	Role        Role
	AdminNote   string
	HasPassword bool
}

// Accounts lists everyone.
func (service *Service) Accounts(ctx context.Context) ([]Account, error) {
	rows, err := service.database.Read().ListLiveUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	accounts := make([]Account, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, Account{
			ID: row.ID, Name: row.Name, Role: Role(row.RoleName), AdminNote: ptr.ValueOrZero(row.AdminNote),
			HasPassword: row.HasPassword,
		})
	}
	return accounts, nil
}

// IssueRegistrationInvitation makes a person's registration link, withdrawing any earlier one: for
// a forgotten password, when the old password works until the new one is set (D218 §5). It takes
// the person out of the failed-sign-in limit.
func (service *Service) IssueRegistrationInvitation(ctx context.Context, userID int64) (IssuedSecret, error) {
	user, err := liveUser(ctx, service.database.Read(), userID)
	if err != nil {
		return IssuedSecret{}, err
	}
	// The invitation is the admin's answer to a person locked out: it clears them from the limit (D221 §4).
	service.limiter.forgetPrefixed(accountLimitKey(user.Name))
	secret, err := randomSecret(service.random)
	if err != nil {
		return IssuedSecret{}, err
	}
	life, err := service.settings.Duration(ctx, settings.RegistrationInvitationLifeDays)
	if err != nil {
		return IssuedSecret{}, fmt.Errorf("read registration_invitation_life_days: %w", err)
	}
	locateHash, offerKey, err := linkHashes(secret)
	if err != nil {
		return IssuedSecret{}, err
	}
	now := service.clock.Now()
	expires := now.Add(life)
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		revokeErr := queries.RevokeLiveGrantsOfUser(ctx, sqlcgen.RevokeLiveGrantsOfUserParams{
			RevokedAt: new(now.UnixMilli()), UserID: &userID, Kind: grantKindIssuedSecret, Scope: grantScopeRegister,
		})
		if revokeErr != nil {
			return fmt.Errorf("withdraw the earlier invitation: %w", revokeErr)
		}
		return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			SecretSha256: tokenHash(secret), Kind: grantKindIssuedSecret, UserID: &userID, Scope: grantScopeRegister,
			IssuedAt: now.UnixMilli(), ExpiresAt: new(expires.UnixMilli()), LocateHash: locateHash, OfferKey: offerKey,
		})
	})
	if err != nil {
		return IssuedSecret{}, fmt.Errorf("record the registration invitation: %w", err)
	}
	service.linksChanged()
	return IssuedSecret{Secret: secret, ExpiresAt: expires}, nil
}

// Register uses a link: an invitation makes the account, with the name the person chose, under the
// invitation's role and with the admin's note (D246 §2); a registration link sets its person's new
// password (D218 §5), and name is not asked. Either signs them in on the device that opened it,
// which becomes theirs, and spends the link. The failed-sign-in limit does not hold it back: its
// 256 bits cannot be guessed (D221 §4).
func (service *Service) Register(
	ctx context.Context, secret, name, password string, device NewDevice,
) (Issued, error) {
	if password == "" {
		return Issued{}, errors.New("a password is needed")
	}
	if isInvitation, err := service.isInvitation(ctx, secret); err != nil {
		return Issued{}, err
	} else if isInvitation {
		return service.acceptInvitation(ctx, secret, name, password, device)
	}
	grant, err := service.spend(ctx, secret, grantScopeRegister)
	if err != nil {
		return Issued{}, err
	}
	userID := *grant.UserID
	hash, err := service.hashWithCurrentCosts(ctx, password)
	if err != nil {
		return Issued{}, err
	}
	// A new password from a registration link ends every session the person had: the link is how
	// someone who lost their password, or had it taken, gets back in (the second audit's K1).
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if updateErr := queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
			ID: userID, PasswordHash: hash,
		}); updateErr != nil {
			return updateErr
		}
		return queries.RevokeSessionsOfUserExceptSession(ctx, sqlcgen.RevokeSessionsOfUserExceptSessionParams{
			RevokedAt: new(service.clock.Now().UnixMilli()), UserID: &userID,
		})
	})
	if err != nil {
		return Issued{}, fmt.Errorf("set the password: %w", err)
	}
	return service.issueSession(ctx, userID, device, grantScopeRole)
}

// liveUser is a person who has not been removed, or ErrNoUser.
func liveUser(ctx context.Context, queries *sqlcgen.Queries, userID int64) (sqlcgen.GetLiveUserByIDRow, error) {
	user, err := store.OrNotFound(queries.GetLiveUserByID(ctx, userID))
	if errors.Is(err, store.ErrNotFound) {
		return user, ErrNoUser
	}
	if err != nil {
		return user, fmt.Errorf("look up user %d: %w", userID, err)
	}
	return user, nil
}

// refuseTakenName answers ErrNameTaken when a live person other than exceptUserID (0 for none)
// already has the name, in any case of its ASCII letters.
func refuseTakenName(ctx context.Context, queries *sqlcgen.Queries, name string, exceptUserID int64) error {
	other, err := store.OrNotFound(queries.GetLiveUserByName(ctx, name))
	if errors.Is(err, store.ErrNotFound) || (err == nil && other.ID == exceptUserID) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look up user %q: %w", name, err)
	}
	return fmt.Errorf("%q: %w", name, ErrNameTaken)
}

// ChangePassword changes a person's own password, given their current one (D218 §6 as amended
// 2026-10-13), and ends every other session of theirs, keepToken's alone staying: a password changed
// because it leaked left whoever had it signed in (the second audit's K1).
func (service *Service) ChangePassword(
	ctx context.Context, userID int64, currentPassword, newPassword, keepToken string,
) error {
	if newPassword == "" {
		return errors.New("a password is needed")
	}
	user, err := liveUser(ctx, service.database.Read(), userID)
	if err != nil {
		return err
	}
	isMatch, _, verifyErr := service.verifyPassword(ctx, currentPassword, string(user.PasswordHash))
	if verifyErr != nil || !isMatch {
		return errors.Join(ErrWrongPassword, verifyErr)
	}
	hash, err := service.hashWithCurrentCosts(ctx, newPassword)
	if err != nil {
		return err
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if updateErr := queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
			ID: userID, PasswordHash: hash,
		}); updateErr != nil {
			return updateErr
		}
		return queries.RevokeSessionsOfUserExceptSession(ctx, sqlcgen.RevokeSessionsOfUserExceptSessionParams{
			RevokedAt: new(service.clock.Now().UnixMilli()), UserID: &userID, KeepSecretSha256: tokenHash(keepToken),
		})
	})
}

// spend spends a live invitation of scope, once, and returns it.
func (service *Service) spend(ctx context.Context, secret, scope string) (sqlcgen.GetLiveGrantRow, error) {
	now := service.clock.Now().UnixMilli()
	hash := tokenHash(secret)
	var grant sqlcgen.GetLiveGrantRow
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		found, lookErr := store.OrNotFound(queries.GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
			SecretSha256: hash,
			Now:          &now,
		}))
		if lookErr != nil {
			return lookErr
		}
		if found.Kind != grantKindIssuedSecret || found.Scope != scope || (found.UserID == nil) !=
			(scope == grantScopeInvitation) {
			return store.ErrNotFound
		}
		grant = found
		// Used, a registration link becomes its person's own link, in place of the one they had
		// (D292 §4; A12): a new password from it is also how the old link is revoked.
		return becomeOwnLink(ctx, queries, hash, found.LocateHash, *found.UserID, time.UnixMilli(now))
	})
	if errors.Is(err, store.ErrNotFound) {
		return sqlcgen.GetLiveGrantRow{}, ErrBadInvitation
	}
	if err != nil {
		return sqlcgen.GetLiveGrantRow{}, fmt.Errorf("use an invitation: %w", err)
	}
	return grant, nil
}

// randomSecret is 256 random bits, base64url: a registration invitation's secret, as a session's is
// (D154).
func randomSecret(random io.Reader) (string, error) {
	secret, err := randbytes.Base64URLSecret(random, tokenBytes)
	if err != nil {
		return "", fmt.Errorf("make a secret: %w", err)
	}
	return secret, nil
}

// RenameUser renames a person. A name is what they are shown as and sign in with; nothing is
// keyed by it, and it stays unique so a sign-in finds one person (the owner, 2026-09-27).
func (service *Service) RenameUser(ctx context.Context, userID int64, name string) error {
	name = textindex.Line(name)
	if name == "" {
		return errors.New("a person needs a name")
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := liveUser(ctx, queries, userID); err != nil {
			return err
		}
		if err := refuseTakenName(ctx, queries, name, userID); err != nil {
			return err
		}
		return queries.RenameUser(ctx, sqlcgen.RenameUserParams{Name: name, ID: userID})
	})
}

// Forget deletes what was a removed person's alone from tables another package writes, through
// queries, the transaction that removes them: playback.ForgetWatchingOf, say.
type Forget func(ctx context.Context, queries *sqlcgen.Queries, userID int64) error

// RemoveUser removes a person, for an admin who is not them (S40). Every credential of theirs ends
// at its next request and their devices leave the list (D154); each of forgets deletes what was
// theirs alone, in the same transaction (D173 §4.6); the row stays as a tombstone, so what they
// set for everyone still says who, and their name is free for someone new.
func (service *Service) RemoveUser(ctx context.Context, byUserID, userID int64, forgets ...Forget) error {
	if byUserID == userID {
		return ErrRemoveSelf
	}
	now := service.clock.Now().UnixMilli()
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		removed, err := queries.RemoveUser(ctx, sqlcgen.RemoveUserParams{RemovedAt: &now, ID: userID})
		if err != nil {
			return fmt.Errorf("remove user %d: %w", userID, err)
		}
		if removed == 0 {
			return ErrNoUser
		}
		if err = queries.RevokeLiveGrantsOfRemovedUser(ctx, sqlcgen.RevokeLiveGrantsOfRemovedUserParams{
			RevokedAt: &now, UserID: &userID,
		}); err != nil {
			return fmt.Errorf("end user %d's credentials: %w", userID, err)
		}
		if err = queries.RemoveDevicesOfRemovedUser(ctx, sqlcgen.RemoveDevicesOfRemovedUserParams{
			RemovedAt: &now, UserID: &userID,
		}); err != nil {
			return fmt.Errorf("remove user %d's devices: %w", userID, err)
		}
		for _, forget := range forgets {
			if forgetErr := forget(ctx, queries, userID); forgetErr != nil {
				return forgetErr
			}
		}
		return nil
	})
}

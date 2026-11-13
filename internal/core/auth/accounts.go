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

// Accounts lists everyone, by name.
func (service *Service) Accounts(ctx context.Context) ([]Account, error) {
	return service.AccountsAfter(ctx, "", 0, -1)
}

// AccountsAfter is a page of everyone, by name: at most limit (-1 for everyone) after the person
// named afterName with afterID ("" and 0 for the first page).
func (service *Service) AccountsAfter(ctx context.Context, afterName string, afterID, limit int64) ([]Account, error) {
	rows, err := service.database.Read().ListLiveUsers(ctx, sqlcgen.ListLiveUsersParams{
		AfterName: afterName, AfterID: afterID, MaxRows: limit,
	})
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

// IssueRegistrationInvitation makes a person's registration link. A person with a password gets a
// reset link (D466): it lives passwordResetLinkLifeMinutes, and it cancels any outstanding one in
// the same write, one at a time (D466 §2); their old password works until it is used (D218 §5). A
// person without one gets a link for registrationInvitationLifeDays that leaves any earlier one live
// until one is used (D318 §1). It takes the person out of the failed-sign-in limit.
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
	isReset := user.PasswordHash != nil
	lifeKey := settings.RegistrationInvitationLifeDays
	if isReset {
		lifeKey = settings.PasswordResetLinkLifeMinutes
	}
	life, err := service.settings.Duration(ctx, lifeKey)
	if err != nil {
		return IssuedSecret{}, fmt.Errorf("read %s: %w", lifeKey, err)
	}
	kept, err := service.keepOfLink(secret)
	if err != nil {
		return IssuedSecret{}, err
	}
	now := service.clock.Now()
	expires := now.Add(life)
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		// A reset link is passed on by the admin in person, and the one just made is the one that
		// counts (D466 §2); a first registration's links stay live, each for its own life (D318 §1).
		if isReset {
			if revokeErr := queries.RevokeLiveGrantsOfUser(ctx, sqlcgen.RevokeLiveGrantsOfUserParams{
				RevokedAt: new(now.UnixMilli()), UserID: &userID, Kind: grantKindIssuedSecret,
				Scope: grantScopeRegister,
			}); revokeErr != nil {
				return revokeErr
			}
		}
		return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			SecretSha256: tokenHash(secret), Kind: grantKindIssuedSecret, UserID: &userID, Scope: grantScopeRegister,
			IssuedAt: now.UnixMilli(), ExpiresAt: new(expires.UnixMilli()),
			LocateHash: kept.locateHash, OfferKey: kept.offerKey,
			LinkSealed: kept.sealed,
		})
	})
	if err != nil {
		return IssuedSecret{}, fmt.Errorf("record the registration invitation: %w", err)
	}
	service.linksChanged()
	return IssuedSecret{Secret: secret, ExpiresAt: expires}, nil
}

// ResetLinkExpiresAt is when a registered person's outstanding reset link stops working, and whether
// they have one: their page says until when it is live, and never shows the link again (D466 §4). A
// person without a password has none, whatever registration links they were sent.
func (service *Service) ResetLinkExpiresAt(ctx context.Context, userID int64) (time.Time, bool, error) {
	queries := service.database.Read()
	user, err := liveUser(ctx, queries, userID)
	if err != nil || user.PasswordHash == nil {
		return time.Time{}, false, err
	}
	expiresAt, err := store.OrNotFound(queries.GetLiveRegistrationLinkOfUser(ctx,
		sqlcgen.GetLiveRegistrationLinkOfUserParams{UserID: &userID, Now: new(service.clock.Now().UnixMilli())}))
	if errors.Is(err, store.ErrNotFound) || (err == nil && expiresAt == nil) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read user %d's reset link: %w", userID, err)
	}
	return time.UnixMilli(*expiresAt), true, nil
}

// CancelResetLink ends every outstanding registration link of the person's, a reset link among them,
// which then sets no password (D318 §3, D466 §4); their password and sessions stay as they are.
func (service *Service) CancelResetLink(ctx context.Context, userID int64) error {
	now := service.clock.Now().UnixMilli()
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := liveUser(ctx, queries, userID); err != nil {
			return err
		}
		return queries.RevokeLiveGrantsOfUser(ctx, sqlcgen.RevokeLiveGrantsOfUserParams{
			RevokedAt: &now, UserID: &userID, Kind: grantKindIssuedSecret, Scope: grantScopeRegister,
		})
	})
	if err != nil {
		return err
	}
	service.linksChanged()
	return nil
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
	if keepErr := service.keepRealmKeyOfUser(ctx, userID, password); keepErr != nil {
		return Issued{}, keepErr
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
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if updateErr := queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
			ID: userID, PasswordHash: hash,
		}); updateErr != nil {
			return updateErr
		}
		return queries.RevokeSessionsOfUserExceptSession(ctx, sqlcgen.RevokeSessionsOfUserExceptSessionParams{
			RevokedAt: new(service.clock.Now().UnixMilli()), UserID: &userID, KeepSecretSha256: tokenHash(keepToken),
		})
	})
	if err != nil {
		return err
	}
	return service.keepRealmKey(ctx, userID, user.Name, newPassword, true)
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
		if err := becomeOwnLink(ctx, queries, hash, found.LocateHash, *found.UserID, time.UnixMilli(now)); err != nil {
			return err
		}
		// Every other registration link of theirs ends with it, in the same write (D318 §2).
		return queries.RevokeLiveGrantsOfUser(ctx, sqlcgen.RevokeLiveGrantsOfUserParams{
			RevokedAt: &now, UserID: found.UserID, Kind: grantKindIssuedSecret, Scope: grantScopeRegister,
		})
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

// RemoveUser removes a person, for an admin who is not them (S40), deleting what was theirs (D305
// §1): each of forgets deletes what was theirs alone, or forgets that they set what everyone
// shares, from the tables another package writes, in the same transaction; then their credentials,
// whose next request is refused, their devices and their realm key go, and their row stays only as
// a nameless tombstone, so ids are never reused and their name is free for someone new.
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
		// Their plays name their devices, so what other packages forget goes first.
		for _, forget := range forgets {
			if forgetErr := forget(ctx, queries, userID); forgetErr != nil {
				return forgetErr
			}
		}
		// In this order: what names a credential of theirs, then the credentials, which name their
		// devices, then the devices.
		for _, step := range []struct {
			what     string
			deleteOf func(context.Context, *int64) error
		}{
			{"locators", queries.DeleteLocatorsOfRemovedUser},
			{"invitations' links", queries.DeleteInvitationLinksOfRemovedUser},
			{"invitations", queries.DeleteInvitationsOfRemovedUser},
			{"derived credentials", queries.DeleteGrantsDerivedFromRemovedUser},
			{"credentials", queries.DeleteGrantsOfRemovedUser},
			{"devices", queries.DeleteDevicesOfRemovedUser},
		} {
			if err = step.deleteOf(ctx, &userID); err != nil {
				return fmt.Errorf("delete user %d's %s: %w", userID, step.what, err)
			}
		}
		if err = queries.DeleteRealmKeyOfRemovedUser(ctx, userID); err != nil {
			return fmt.Errorf("delete user %d's realm key: %w", userID, err)
		}
		return nil
	})
}

// ErrNotAnAdminAway is a setting for administering from away that is none of D302's values.
var ErrNotAnAdminAway = errors.New("want never or always")

// SetAdminAway sets whether the admin userID administers from away from home too (D302): the
// admin's own setting, which the API lets them change from home alone.
func (service *Service) SetAdminAway(ctx context.Context, userID int64, adminAway string) error {
	if adminAway != AdminAwayNever && adminAway != AdminAwayAlways {
		return fmt.Errorf("%q: %w", adminAway, ErrNotAnAdminAway)
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.SetAdminAway(ctx, sqlcgen.SetAdminAwayParams{AdminAway: adminAway, ID: userID})
	})
}

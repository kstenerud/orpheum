// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

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
	// A registration invitation, which lets its person set a password (D218 §1).
	grantScopeRegister = "register"
	// A device code, which signs a new device in as its person (D218 §3).
	grantScopeAddDevice = "add_device"
)

// How long each invitation lasts (D218 §1, §3).
const (
	registrationInvitationLife = 7 * 24 * time.Hour
	deviceCodeLife             = 10 * time.Minute
)

var (
	// ErrBadInvitation is a registration invitation or device code that is unknown, used, expired or
	// withdrawn, without saying which.
	ErrBadInvitation = errors.New("the link or code is not valid; ask for a new one")
	// ErrNoUser is a person who is not there.
	ErrNoUser = errors.New("no such person")
)

// IssuedSecret is a registration invitation's secret or a device code, and when it stops working.
type IssuedSecret struct {
	Secret    string
	ExpiresAt time.Time
}

// Account is one person, as the admin's list shows them.
type Account struct {
	ID          int64
	Name        string
	Role        Role
	IsOutsider  bool
	HasPassword bool
}

// CreateAccount makes a person with a name and role and no password, and a registration invitation for
// them to set one (D218 §1).
func (service *Service) CreateAccount(ctx context.Context, name string, role Role) (int64, IssuedSecret, error) {
	id, err := service.CreateUser(ctx, NewUser{Name: name, Role: role, AwaitsRegistration: true})
	if err != nil {
		return 0, IssuedSecret{}, err
	}
	invitation, err := service.IssueRegistrationInvitation(ctx, id)
	return id, invitation, err
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
			ID: row.ID, Name: row.Name, Role: Role(row.RoleName), IsOutsider: row.IsOutsider == 1,
			HasPassword: row.HasPassword,
		})
	}
	return accounts, nil
}

// IssueRegistrationInvitation makes a person's registration invitation, withdrawing any earlier one: the
// first for a new account, or a new one for a forgotten password, when the old password works
// until the new one is set (D218 §1, §5). It takes the person out of the failed-sign-in limit.
func (service *Service) IssueRegistrationInvitation(ctx context.Context, userID int64) (IssuedSecret, error) {
	user, err := liveUser(ctx, service.database.Read(), userID)
	if err != nil {
		return IssuedSecret{}, err
	}
	// The invitation is the admin's answer to a person locked out: it clears them from the limit (D221 §4).
	service.limiter.forget(accountLimitKey(user.Name))
	secret, err := randomSecret(service.random)
	if err != nil {
		return IssuedSecret{}, err
	}
	now := service.clock.Now()
	expires := now.Add(registrationInvitationLife)
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		revokeErr := queries.RevokeLiveGrantsOfUser(ctx, sqlcgen.RevokeLiveGrantsOfUserParams{
			RevokedAt: new(now.UnixMilli()), UserID: &userID, Kind: grantKindIssuedSecret, Scope: grantScopeRegister,
		})
		if revokeErr != nil {
			return fmt.Errorf("withdraw the earlier invitation: %w", revokeErr)
		}
		return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			SecretSha256: tokenHash(secret), Kind: grantKindIssuedSecret, UserID: &userID, Scope: grantScopeRegister,
			IssuedAt: now.UnixMilli(), ExpiresAt: new(expires.UnixMilli()),
		})
	})
	if err != nil {
		return IssuedSecret{}, fmt.Errorf("record the registration invitation: %w", err)
	}
	return IssuedSecret{Secret: secret, ExpiresAt: expires}, nil
}

// Register uses a registration invitation: it sets its person's password, signs them in on the device
// that opened it, which becomes theirs, and spends the invitation (D218 §2). The failed-sign-in limit
// does not hold it back: its 256 bits cannot be guessed (D221 §4).
func (service *Service) Register(ctx context.Context, secret, password string, device NewDevice) (string, error) {
	if password == "" {
		return "", errors.New("a password is needed")
	}
	grant, err := service.spend(ctx, secret, grantScopeRegister)
	if err != nil {
		return "", err
	}
	userID := *grant.UserID
	hash, err := service.hashWithCurrentCosts(ctx, password)
	if err != nil {
		return "", err
	}
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{ID: userID, PasswordHash: hash})
	})
	if err != nil {
		return "", fmt.Errorf("set the password: %w", err)
	}
	return service.issueSession(ctx, userID, device)
}

// IssueDeviceCode makes a code that signs a device in as the person (D218 §3): a new one, named
// as it will be listed, or, with forDeviceID, one of theirs already listed, which the code signs
// back in (D221 §2). Eight characters of safe32, for ten minutes, once; making one withdraws the
// person's last, whichever device it was for (D221 §3).
func (service *Service) IssueDeviceCode(
	ctx context.Context, userID int64, deviceName string, forDeviceID int64,
) (IssuedSecret, error) {
	if _, err := liveUser(ctx, service.database.Read(), userID); err != nil {
		return IssuedSecret{}, err
	}
	var deviceID *int64
	if forDeviceID != 0 {
		known, err := liveDevice(ctx, service.database.Read(), userID, forDeviceID)
		if err != nil {
			return IssuedSecret{}, err
		}
		deviceID, deviceName = &known.ID, known.Name
	}
	deviceName = textindex.Line(deviceName)
	raw, err := randbytes.Read(service.random, codeBytes)
	if err != nil {
		return IssuedSecret{}, fmt.Errorf("make a device code: %w", err)
	}
	code := encodeCode([codeBytes]byte(raw))
	now := service.clock.Now()
	expires := now.Add(deviceCodeLife)
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		revokeErr := queries.RevokeLiveGrantsOfUser(ctx, sqlcgen.RevokeLiveGrantsOfUserParams{
			RevokedAt: new(now.UnixMilli()), UserID: &userID, Kind: grantKindIssuedSecret, Scope: grantScopeAddDevice,
		})
		if revokeErr != nil {
			return fmt.Errorf("withdraw the last code: %w", revokeErr)
		}
		return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			SecretSha256: tokenHash(code), Kind: grantKindIssuedSecret, UserID: &userID, Scope: grantScopeAddDevice,
			IssuedAt: now.UnixMilli(), ExpiresAt: new(expires.UnixMilli()), DeviceName: &deviceName, DeviceID: deviceID,
		})
	})
	if err != nil {
		return IssuedSecret{}, fmt.Errorf("record the device code: %w", err)
	}
	return IssuedSecret{Secret: code, ExpiresAt: expires}, nil
}

// WaitingCode is a device code not yet used: the name of the device it is for, and when it stops
// working.
type WaitingCode struct {
	DeviceName string
	ExpiresAt  time.Time
}

// WaitingCodeOf is the person's device code that has not been used yet, or nil when there is
// none, so a client that showed one knows it was used, withdrawn or expired (D221 §3).
func (service *Service) WaitingCodeOf(ctx context.Context, userID int64) (*WaitingCode, error) {
	waiting, err := store.OrNotFound(service.database.Read().GetLatestLiveGrantOfUser(ctx,
		sqlcgen.GetLatestLiveGrantOfUserParams{
			UserID: &userID, Kind: grantKindIssuedSecret, Scope: grantScopeAddDevice,
			Now: new(service.clock.Now().UnixMilli()),
		}))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil //nolint:nilnil // no code waiting is an answer, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("read user %d's device code: %w", userID, err)
	}
	return &WaitingCode{
		DeviceName: ptr.ValueOrZero(waiting.DeviceName), ExpiresAt: time.UnixMilli(ptr.ValueOrZero(waiting.ExpiresAt)),
	}, nil
}

// SignInWithCode signs a device in with a device code, as its person, and spends the code (D218
// §4). A code made for a listed device signs that one back in; otherwise the device the client
// remembers, resumeDeviceID, is resumed when it is the person's (D221), or a new one is made under the
// name the code carries. What was typed is read as safe32 reads it; a wrong code counts against
// the address's failed sign-ins.
func (service *Service) SignInWithCode(
	ctx context.Context, typed, platform, address string, resumeDeviceID int64,
) (string, error) {
	code, err := normalizeCode(typed)
	if err != nil {
		code = typed // it cannot match, and is counted as the failure it is
	}
	grant, err := service.spendGuessable(ctx, code, grantScopeAddDevice, address)
	if err != nil {
		return "", err
	}
	device := NewDevice{Name: ptr.ValueOrZero(grant.DeviceName), Platform: platform, PreviousDeviceID: resumeDeviceID}
	if grant.DeviceID != nil {
		device.PreviousDeviceID = *grant.DeviceID
	}
	return service.issueSession(ctx, *grant.UserID, device)
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

// ChangePassword changes a person's own password, given their current one (D218 §6). Other
// sessions stay signed in; signing out everywhere is a separate act.
func (service *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	if newPassword == "" {
		return errors.New("a password is needed")
	}
	user, err := liveUser(ctx, service.database.Read(), userID)
	if err != nil {
		return err
	}
	isMatch, _, verifyErr := VerifyPassword(currentPassword, string(user.PasswordHash))
	if verifyErr != nil || !isMatch {
		return errors.Join(ErrWrongPassword, verifyErr)
	}
	hash, err := service.hashWithCurrentCosts(ctx, newPassword)
	if err != nil {
		return err
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{ID: userID, PasswordHash: hash})
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
		if found.Kind != grantKindIssuedSecret || found.Scope != scope || found.UserID == nil {
			return store.ErrNotFound
		}
		grant = found
		return queries.MarkGrantRevoked(ctx, sqlcgen.MarkGrantRevokedParams{SecretSha256: hash, RevokedAt: &now})
	})
	if errors.Is(err, store.ErrNotFound) {
		return sqlcgen.GetLiveGrantRow{}, ErrBadInvitation
	}
	if err != nil {
		return sqlcgen.GetLiveGrantRow{}, fmt.Errorf("use an invitation: %w", err)
	}
	return grant, nil
}

// spendGuessable is spend for a secret short enough to guess, a device code: one that is not
// live counts against the address's failed sign-ins, as a wrong password does, and a limited
// address is refused before it is looked up.
func (service *Service) spendGuessable(
	ctx context.Context, secret, scope, address string,
) (sqlcgen.GetLiveGrantRow, error) {
	maxFailures, window, err := service.failurePolicy(ctx)
	if err != nil {
		return sqlcgen.GetLiveGrantRow{}, err
	}
	addressKey := "address:" + address
	if service.limiter.isLimited(maxFailures, window, addressKey) {
		return sqlcgen.GetLiveGrantRow{}, ErrRateLimited
	}
	grant, err := service.spend(ctx, secret, scope)
	if errors.Is(err, ErrBadInvitation) {
		service.limiter.recordFailure(window, addressKey)
	}
	return grant, err
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

// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The invitations D218 makes: grants of kind invite, single use, each in its own scope.
const (
	grantKindInvite = "invite"
	// A registration link, which lets its person set a password (D218 §1).
	grantScopeRegister = "register"
	// A device code, which signs a new device in as its person (D218 §3).
	grantScopeAddDevice = "add_device"
)

// How long each invitation lasts (D218 §1, §3).
const (
	registrationLinkLife = 7 * 24 * time.Hour
	deviceCodeLife       = 10 * time.Minute
)

var (
	// ErrBadInvitation is a registration link or device code that is unknown, used, expired or
	// withdrawn, without saying which.
	ErrBadInvitation = errors.New("the link or code is not valid; ask for a new one")
	// ErrNoUser is a person who is not there.
	ErrNoUser = errors.New("no such person")
)

// Invitation is a registration link's secret or a device code, and when it stops working.
type Invitation struct {
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

// CreateAccount makes a person with a name and role and no password, and a registration link for
// them to set one (D218 §1).
func (service *Service) CreateAccount(ctx context.Context, name string, role Role) (int64, Invitation, error) {
	id, err := service.CreateUser(ctx, NewUser{Name: name, Role: role, AwaitsRegistration: true})
	if err != nil {
		return 0, Invitation{}, err
	}
	link, err := service.IssueRegistrationLink(ctx, id)
	return id, link, err
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
			ID: row.ID, Name: row.Name, Role: Role(row.RoleName), IsOutsider: row.Outsider == 1,
			HasPassword: row.HasPassword,
		})
	}
	return accounts, nil
}

// IssueRegistrationLink makes a person's registration link, withdrawing any earlier one: the
// first for a new account, or a new one for a forgotten password, when the old password works
// until the new one is set (D218 §1, §5).
func (service *Service) IssueRegistrationLink(ctx context.Context, userID int64) (Invitation, error) {
	if _, err := store.One(service.database.Read().GetLiveUserByID(ctx, userID)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Invitation{}, ErrNoUser
		}
		return Invitation{}, fmt.Errorf("look up user %d: %w", userID, err)
	}
	secret, err := randomSecret(service.random)
	if err != nil {
		return Invitation{}, err
	}
	now := service.clock.Now()
	expires := now.Add(registrationLinkLife)
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		revokeErr := queries.RevokeLiveGrantsOf(ctx, sqlcgen.RevokeLiveGrantsOfParams{
			RevokedAt: new(now.UnixMilli()), Subject: &userID, Kind: grantKindInvite, Scope: grantScopeRegister,
		})
		if revokeErr != nil {
			return fmt.Errorf("withdraw the earlier link: %w", revokeErr)
		}
		return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			Hash: tokenHash(secret), Kind: grantKindInvite, Subject: &userID, Scope: grantScopeRegister,
			IssuedAt: now.UnixMilli(), ExpiresAt: new(expires.UnixMilli()),
		})
	})
	if err != nil {
		return Invitation{}, fmt.Errorf("record the registration link: %w", err)
	}
	return Invitation{Secret: secret, ExpiresAt: expires}, nil
}

// Register uses a registration link: it sets its person's password, signs them in on the device
// that opened it, which becomes theirs, and spends the link (D218 §2). A link that does not work
// counts against the address's failed sign-ins.
func (service *Service) Register(
	ctx context.Context, secret, password, address string, device NewDevice,
) (string, error) {
	if password == "" {
		return "", errors.New("a password is needed")
	}
	userID, err := service.redeem(ctx, secret, grantScopeRegister, address)
	if err != nil {
		return "", err
	}
	hash, err := service.hash(ctx, password)
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

// IssueDeviceCode makes a code that signs one new device in as the person, named as it will be
// listed (D218 §3): eight characters of safe32, for ten minutes, once.
func (service *Service) IssueDeviceCode(ctx context.Context, userID int64, deviceName string) (Invitation, error) {
	if _, err := store.One(service.database.Read().GetLiveUserByID(ctx, userID)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Invitation{}, ErrNoUser
		}
		return Invitation{}, fmt.Errorf("look up user %d: %w", userID, err)
	}
	var raw [codeBytes]byte
	if _, err := io.ReadFull(service.random, raw[:]); err != nil {
		return Invitation{}, fmt.Errorf("make a device code: %w", err)
	}
	code := encodeCode(raw)
	now := service.clock.Now()
	expires := now.Add(deviceCodeLife)
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			Hash: tokenHash(code), Kind: grantKindInvite, Subject: &userID, Scope: grantScopeAddDevice,
			IssuedAt: now.UnixMilli(), ExpiresAt: new(expires.UnixMilli()), Label: &deviceName,
		})
	})
	if err != nil {
		return Invitation{}, fmt.Errorf("record the device code: %w", err)
	}
	return Invitation{Secret: code, ExpiresAt: expires}, nil
}

// SignInWithCode signs a device in with a device code, as its person, under the name the code
// was made with, and spends the code (D218 §4). What was typed is read as safe32 reads it; a
// wrong code counts against the address's failed sign-ins.
func (service *Service) SignInWithCode(ctx context.Context, typed, platform, address string) (string, error) {
	code, err := normalizeCode(typed)
	if err != nil {
		code = typed // it cannot match, and is counted as the failure it is
	}
	grant, err := service.redeemGrant(ctx, code, grantScopeAddDevice, address)
	if err != nil {
		return "", err
	}
	device := NewDevice{Name: ptr.ValueOrZero(grant.Label), Platform: platform}
	return service.issueSession(ctx, *grant.Subject, device)
}

// ChangePassword changes a person's own password, given their current one (D218 §6). Other
// sessions stay signed in; signing out everywhere is a separate act.
func (service *Service) ChangePassword(ctx context.Context, userID int64, current, replacement string) error {
	if replacement == "" {
		return errors.New("a password is needed")
	}
	user, err := store.One(service.database.Read().GetLiveUserByID(ctx, userID))
	if err != nil {
		return fmt.Errorf("look up user %d: %w", userID, err)
	}
	if matches, _, verifyErr := VerifyPassword(current, string(user.PasswordHash)); verifyErr != nil || !matches {
		return ErrBadCredentials
	}
	hash, err := service.hash(ctx, replacement)
	if err != nil {
		return err
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{ID: userID, PasswordHash: hash})
	})
}

// redeem spends an invitation of scope and says whose it was.
func (service *Service) redeem(ctx context.Context, secret, scope, address string) (int64, error) {
	grant, err := service.redeemGrant(ctx, secret, scope, address)
	if err != nil {
		return 0, err
	}
	return *grant.Subject, nil
}

// redeemGrant spends a live invitation of scope, once, and returns it. One that is not live
// counts against the address's failed sign-ins, as a wrong password does.
func (service *Service) redeemGrant(
	ctx context.Context, secret, scope, address string,
) (sqlcgen.GetLiveGrantRow, error) {
	limit, window, err := service.failurePolicy(ctx)
	if err != nil {
		return sqlcgen.GetLiveGrantRow{}, err
	}
	addressKey := "address:" + address
	if service.limiter.isLimited(limit, window, addressKey) {
		return sqlcgen.GetLiveGrantRow{}, ErrRateLimited
	}
	now := service.clock.Now().UnixMilli()
	hash := tokenHash(secret)
	var grant sqlcgen.GetLiveGrantRow
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		found, lookErr := store.One(queries.GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{Hash: hash, Now: &now}))
		if lookErr != nil {
			return lookErr
		}
		if found.Kind != grantKindInvite || found.Scope != scope || found.Subject == nil {
			return store.ErrNotFound
		}
		grant = found
		return queries.MarkGrantRevoked(ctx, sqlcgen.MarkGrantRevokedParams{Hash: hash, RevokedAt: &now})
	})
	if errors.Is(err, store.ErrNotFound) {
		service.limiter.fail(window, addressKey)
		return sqlcgen.GetLiveGrantRow{}, ErrBadInvitation
	}
	if err != nil {
		return sqlcgen.GetLiveGrantRow{}, fmt.Errorf("use an invitation: %w", err)
	}
	return grant, nil
}

// randomSecret is 256 random bits, base64url: a registration link's secret, as a session's is
// (D154).
func randomSecret(random io.Reader) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", fmt.Errorf("make a secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

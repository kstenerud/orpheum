// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Role is one of Cut A's two built-in roles (cut-a §1, D125); the rows are in migration 0001.
type Role string

// The roles.
const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// ParseRole accepts a role's name.
func ParseRole(name string) (Role, error) {
	role := Role(name)
	switch role {
	case RoleAdmin, RoleUser:
		return role, nil
	}
	return "", fmt.Errorf("role %q: want %q or %q", name, RoleAdmin, RoleUser)
}

// capabilityAdminister is the capability that makes a principal an administrator (D130).
const capabilityAdminister = "administer"

// Grant kinds and scopes this package mints (schema.md, grant; D154 §3).
const (
	grantKindSession  = "session"
	grantScopeSession = "role" // a session carries the user's role, entire (D154 §3)
	tokenBytes        = 32
	// The resolution of a session's last_seen: once a minute is enough for idle expiry, and a
	// write per request is not.
	lastSeenResolution = time.Minute
)

var (
	// ErrBadCredentials is returned for an unknown user or a wrong password, without saying which.
	ErrBadCredentials = errors.New("the name or password is wrong")
	// ErrRateLimited is returned while an account or address has failed too often recently.
	ErrRateLimited = errors.New("too many failed sign-ins; wait and try again")
	// ErrNotAuthenticated is returned for a missing, unknown, revoked or expired session.
	ErrNotAuthenticated = errors.New("not signed in")
	// ErrNameTaken is returned when a live user already has the name.
	ErrNameTaken = errors.New("a user with that name already exists")
)

// Principal is who a request acts as.
type Principal struct {
	UserID       int64
	Name         string
	Role         Role
	Capabilities []string
	IsOutsider   bool
}

// IsAdmin reports whether the principal's role carries the administer capability.
func (principal Principal) IsAdmin() bool {
	return slices.Contains(principal.Capabilities, capabilityAdminister)
}

// Service signs people in and out and says who a credential belongs to.
type Service struct {
	database   *store.Database
	clock      clock.Clock
	random     io.Reader
	parameters Parameters
	limiter    *failureLimiter
	// dummyHash is verified when a name is unknown, so an unknown name costs what a known one does.
	dummyHash string
}

// NewService returns a Service that hashes new passwords under parameters.
func NewService(database *store.Database, now clock.Clock, random io.Reader, parameters Parameters) (*Service, error) {
	dummyHash, err := HashPassword("unknown user", parameters, random)
	if err != nil {
		return nil, err
	}
	return &Service{
		database: database, clock: now, random: random, parameters: parameters,
		limiter: newFailureLimiter(now), dummyHash: dummyHash,
	}, nil
}

// NewUser describes a user to create.
type NewUser struct {
	Name       string
	Password   string
	Role       Role
	IsOutsider bool
}

// CreateUser adds a user with an argon2id password hash and returns its id.
func (service *Service) CreateUser(ctx context.Context, user NewUser) (int64, error) {
	var id int64
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var insertErr error
		id, insertErr = service.insertUser(ctx, queries, user)
		return insertErr
	})
	return id, err
}

// insertUser adds a user inside the caller's write transaction.
func (service *Service) insertUser(ctx context.Context, queries *sqlcgen.Queries, user NewUser) (int64, error) {
	name := strings.TrimSpace(user.Name)
	if name == "" || user.Password == "" {
		return 0, errors.New("a user needs a name and a password")
	}
	if _, err := store.One(queries.GetLiveUserByName(ctx, name)); err == nil {
		return 0, fmt.Errorf("%q: %w", name, ErrNameTaken)
	} else if !errors.Is(err, store.ErrNotFound) {
		return 0, fmt.Errorf("look up user %q: %w", name, err)
	}
	role, err := queries.GetRoleByName(ctx, string(user.Role))
	if err != nil {
		return 0, fmt.Errorf("find role %q: %w", user.Role, err)
	}
	hash, err := HashPassword(user.Password, service.parameters, service.random)
	if err != nil {
		return 0, err
	}
	var outsider int64
	if user.IsOutsider {
		outsider = 1
	}
	id, err := queries.InsertUser(ctx, sqlcgen.InsertUserParams{
		Name: name, RoleID: role.ID, PasswordHash: []byte(hash), Outsider: outsider,
		CreatedAt: service.clock.Now().UnixMilli(),
	})
	if err != nil {
		return 0, fmt.Errorf("insert user %q: %w", name, err)
	}
	return id, nil
}

// CountUsers returns how many live users exist; none means the server still needs setting up.
func (service *Service) CountUsers(ctx context.Context) (int64, error) {
	count, err := service.database.Read().CountLiveUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// SignIn checks a name and password and issues a session. The address is the client's network
// address, for the per-address failure limit. A hash made under older parameters is re-hashed.
func (service *Service) SignIn(ctx context.Context, name, password, address string) (token string, err error) {
	accountKey, addressKey := "account:"+strings.ToLower(strings.TrimSpace(name)), "address:"+address
	if service.limiter.isLimited(accountKey, addressKey) {
		return "", ErrRateLimited
	}
	user, err := store.One(service.database.Read().GetLiveUserByName(ctx, strings.TrimSpace(name)))
	if errors.Is(err, store.ErrNotFound) {
		_, _, verifyErr := VerifyPassword(password, service.dummyHash)
		service.limiter.fail(accountKey, addressKey)
		return "", errors.Join(ErrBadCredentials, verifyErr)
	}
	if err != nil {
		return "", fmt.Errorf("look up user: %w", err)
	}
	matches, used, err := VerifyPassword(password, string(user.PasswordHash))
	if err != nil || !matches {
		service.limiter.fail(accountKey, addressKey)
		return "", errors.Join(ErrBadCredentials, err)
	}
	service.limiter.forget(accountKey)
	if !sameCosts(used, service.parameters) {
		if rehashErr := service.rehash(ctx, user.ID, password); rehashErr != nil {
			return "", rehashErr
		}
	}
	return service.issueSession(ctx, user.ID)
}

func (service *Service) rehash(ctx context.Context, userID int64, password string) error {
	hash, err := HashPassword(password, service.parameters, service.random)
	if err != nil {
		return err
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
			ID: userID, PasswordHash: []byte(hash),
		})
	})
}

// issueSession mints a session grant for the user and returns its token, which is stored only
// as its SHA-256 (D154).
func (service *Service) issueSession(ctx context.Context, userID int64) (string, error) {
	secret := make([]byte, tokenBytes)
	if _, err := io.ReadFull(service.random, secret); err != nil {
		return "", fmt.Errorf("make a session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			Hash: tokenHash(token), Kind: grantKindSession, Subject: &userID, Scope: grantScopeSession,
			IssuedAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return "", fmt.Errorf("record the session: %w", err)
	}
	return token, nil
}

// Authenticate returns the principal a session token belongs to.
func (service *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrNotAuthenticated
	}
	now := service.clock.Now()
	hash := tokenHash(token)
	grant, err := store.One(service.database.Read().GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
		Hash: hash, Now: new(now.UnixMilli()),
	}))
	if errors.Is(err, store.ErrNotFound) || (err == nil && (grant.Kind != grantKindSession || grant.Subject == nil)) {
		return Principal{}, ErrNotAuthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up the session: %w", err)
	}
	user, err := store.One(service.database.Read().GetLiveUserByID(ctx, *grant.Subject))
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, ErrNotAuthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up the session's user: %w", err)
	}
	if grant.LastSeen == nil || now.Sub(time.UnixMilli(*grant.LastSeen)) >= lastSeenResolution {
		err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
			return queries.UpdateGrantLastSeen(ctx, sqlcgen.UpdateGrantLastSeenParams{
				Hash: hash, LastSeen: new(now.UnixMilli()),
			})
		})
		if err != nil {
			return Principal{}, fmt.Errorf("record the session's use: %w", err)
		}
	}
	return Principal{
		UserID: user.ID, Name: user.Name, Role: Role(user.RoleName),
		Capabilities: strings.Split(user.Capabilities, ","), IsOutsider: user.Outsider == 1,
	}, nil
}

// SignOut revokes a session; revoking one already gone is not an error.
func (service *Service) SignOut(ctx context.Context, token string) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.MarkGrantRevoked(ctx, sqlcgen.MarkGrantRevokedParams{
			Hash: tokenHash(token), RevokedAt: new(service.clock.Now().UnixMilli()),
		})
	})
}

func tokenHash(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}

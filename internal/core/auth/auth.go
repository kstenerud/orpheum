// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/clientnet"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Role is one of Cut A's two built-in roles (cut-a §1, D125); the rows are in the baseline migration.
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

// capabilityHostWatchTogether lets a role start a Watch Together session (D125).
const capabilityHostWatchTogether = "host_watch_together"

// capabilityEditMetadata lets a role fix identification and metadata (the role table's
// capabilities; the built-in admin role carries it).
const capabilityEditMetadata = "edit_metadata"

// Grant kinds and scopes this package mints (schema.md, grant; D154 §3).
const (
	grantKindSession = "session"
	grantScopeRole   = "role" // a session carries the user's role, entire (D154 §3)
	// A device-scoped session carries the role but for administering and the account's security:
	// a device a code activated, whoever entered the code (D292 §4; the security audit's A11). A
	// password sign-in carries the whole role wherever it comes from, a discovery's page included
	// (D294 §4).
	grantScopeDevice = "device"
)

// tokenBytes is a session token's length: 256 random bits, which no one guesses. A format, not an
// opinion, so not a setting (D200 §1).
const tokenBytes = 32

var (
	// ErrBadCredentials is a sign-in whose name is unknown or whose password is wrong, without
	// saying which.
	ErrBadCredentials = errors.New("the name or password is wrong")
	// ErrWrongPassword is a signed-in person's current password, given to change it, that is
	// wrong: they are who they are, and are refused the change, not the sign-in.
	ErrWrongPassword = errors.New("the current password is wrong")
	// ErrRateLimited is returned while an account or address has failed to sign in too often
	// recently, with a password or a device code.
	ErrRateLimited = errors.New("too many failed sign-ins; wait and try again")
	// ErrNotAuthenticated is returned for a missing, unknown, revoked or expired session.
	ErrNotAuthenticated = errors.New("not signed in")
	// ErrSessionEnded is a session that was ended: signed out, its device removed, or signed out
	// from another device. It is ErrNotAuthenticated, said more exactly, so a client can tell the
	// person why.
	ErrSessionEnded = fmt.Errorf("%w: the session was ended", ErrNotAuthenticated)
	// ErrSessionExpired is a session that lapsed: unused for longer than the idle expiry.
	ErrSessionExpired = fmt.Errorf("%w: the session expired", ErrNotAuthenticated)
	// ErrNameTaken is returned when a live user already has the name.
	ErrNameTaken = errors.New("a user with that name already exists")
)

// Principal is who a request acts as.
type Principal struct {
	UserID       int64
	Name         string
	Role         Role
	Capabilities []string
	// Reach is the libraries the person may browse and play, as their role says now (D243 §4).
	Reach Reach
	// DeviceID is the device the session was signed in on (D217 §8), or 0 for a session from
	// before devices were made at sign-in.
	DeviceID int64
	// SessionKey names the session without holding its credential: the hex of the token's
	// SHA-256, the same digest the grant row keeps. A media token is bound to it, so signing out
	// ends the playbacks the session started (D217 §2).
	SessionKey string
	// isDeviceScoped is a session of the device scope: its Capabilities lack administering, and
	// it may not change the account's security (CanManageAccount).
	isDeviceScoped bool
}

// CanManageAccount reports whether the session may change the account's security: its password,
// its devices, and the codes it enters for other devices. A device-scoped session may not (D292 §4;
// the security audit's A11).
func (principal Principal) CanManageAccount() bool {
	return !principal.isDeviceScoped
}

// IsAdmin reports whether the principal's role carries the administer capability.
func (principal Principal) IsAdmin() bool {
	return slices.Contains(principal.Capabilities, capabilityAdminister)
}

// CanHostWatchTogether reports whether the principal's role may host a Watch Together session.
func (principal Principal) CanHostWatchTogether() bool {
	return slices.Contains(principal.Capabilities, capabilityHostWatchTogether)
}

// CanEditMetadata reports whether the principal's role may fix identification and metadata:
// choose a match, override a field.
func (principal Principal) CanEditMetadata() bool {
	return slices.Contains(principal.Capabilities, capabilityEditMetadata)
}

// Service signs people in and out and says who a credential belongs to. Its opinions — argon2id's
// costs, the failure limit, session idle expiry — are settings, read when they are used (D200).
type Service struct {
	database *store.Database
	clock    clock.Clock
	random   io.Reader
	settings Settings
	limiter  *failureLimiter
	// pairings are the devices being activated by a code (D288).
	pairings *pairings
	// hashing is the turn each password hash waits for (hashesAtOnce).
	hashing hashSlots
}

// NewService returns a Service.
func NewService(database *store.Database, clockSource clock.Clock, random io.Reader, settings Settings) *Service {
	return &Service{
		database: database, clock: clockSource, random: random, settings: settings,
		limiter: newFailureLimiter(clockSource), pairings: &pairings{bySecret: map[string]*pairing{}},
		hashing: newHashSlots(),
	}
}

// NewUser describes a user to create: set up in full, a name, a password and a role, and the
// admin's note of who it is for, which only the admin sees (D246 §2).
type NewUser struct {
	Name      string
	Password  string
	Role      Role
	AdminNote string
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
	name := textindex.Line(user.Name)
	if name == "" || user.Password == "" {
		return 0, errors.New("a user needs a name and a password")
	}
	if err := refuseTakenName(ctx, queries, name, 0); err != nil {
		return 0, err
	}
	role, err := store.OrNotFound(queries.GetRoleByName(ctx, string(user.Role)))
	if errors.Is(err, store.ErrNotFound) {
		return 0, ErrNoRole
	}
	if err != nil {
		return 0, fmt.Errorf("find role %q: %w", user.Role, err)
	}
	hash, err := service.hashWithCurrentCosts(ctx, user.Password)
	if err != nil {
		return 0, err
	}
	id, err := queries.InsertUser(ctx, sqlcgen.InsertUserParams{
		Name: name, RoleID: role.ID, PasswordHash: hash, AdminNote: ptr.NilIfZero(textindex.Line(user.AdminNote)),
		CreatedAt: service.clock.Now().UnixMilli(),
	})
	if err != nil {
		return 0, fmt.Errorf("insert user %q: %w", name, err)
	}
	return id, nil
}

// CountUsers returns how many live users exist; none means the server still needs setting up.
func (service *Service) CountUsers(ctx context.Context) (int64, error) {
	return countUsers(ctx, service.database.Read())
}

// countUsers counts the live users as the caller's queries see them, so setup can count inside
// the transaction that makes the first admin.
func countUsers(ctx context.Context, queries *sqlcgen.Queries) (int64, error) {
	count, err := queries.CountLiveUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// NewDevice is how a client names itself when it signs in (D217 §8): "Chrome on Android", and the
// kind of client it is, "web".
type NewDevice struct {
	Name     string
	Platform string
	// PreviousDeviceID is the device the client remembers being signed in as: a sign-in resumes it when it
	// is the same person's, has not been removed, and PreviousDeviceProof proves the client was it,
	// rather than making a new one (D221 §1).
	PreviousDeviceID int64
	// PreviousDeviceProof is a session token that device was issued, live or not: an id alone let any
	// client that named one sit on another's device, unseen in the list and kept by "sign out
	// everywhere else" (the second audit's K1 and W4, 2026-10-13).
	PreviousDeviceProof string
}

// SignIn checks a name and password and issues a session on the device the client remembers,
// when it is the person's, or on a new one the client named (D221 §1). The per-address failure
// limit counts by clientAddress, the client's network address. A hash made under older
// parameters is re-hashed.
func (service *Service) SignIn(
	ctx context.Context, name, password, clientAddress string, device NewDevice,
) (token string, err error) {
	maxFailures, window, err := service.failurePolicy(ctx)
	if err != nil {
		return "", err
	}
	keys := signInLimitKeys(name, clientAddress)
	attemptedAt, isAllowed := service.limiter.attempt(window, map[string]int64{
		keys.accountHere: maxFailures, keys.accountEverywhere: maxFailures * accountEverywhereTimes,
		keys.network: maxFailures * networkTimes,
	})
	if !isAllowed {
		return "", ErrRateLimited
	}
	parameters, err := service.argon2Parameters(ctx)
	if err != nil {
		return "", err
	}
	user, err := store.OrNotFound(service.database.Read().GetLiveUserByName(ctx, textindex.Line(name)))
	if errors.Is(err, store.ErrNotFound) {
		// An unknown name costs what a known one does, so timing does not tell them apart.
		_, hashErr := service.hashPassword(ctx, password, parameters)
		return "", errors.Join(ErrBadCredentials, hashErr)
	}
	if err != nil {
		return "", fmt.Errorf("look up user: %w", err)
	}
	isMatch, hashParameters, err := service.verifyPassword(ctx, password, string(user.PasswordHash))
	if err != nil || !isMatch {
		return "", errors.Join(ErrBadCredentials, err)
	}
	service.limiter.withdraw(attemptedAt, keys.network, keys.accountEverywhere)
	service.limiter.forget(keys.accountHere)
	if !sameCosts(hashParameters, parameters) {
		if rehashErr := service.rehash(ctx, user.ID, password, parameters); rehashErr != nil {
			return "", rehashErr
		}
	}
	return service.issueSession(ctx, user.ID, device, grantScopeRole)
}

// hashWithCurrentCosts hashes a password with the costs the settings name now.
func (service *Service) hashWithCurrentCosts(ctx context.Context, password string) ([]byte, error) {
	parameters, err := service.argon2Parameters(ctx)
	if err != nil {
		return nil, err
	}
	hashed, err := service.hashPassword(ctx, password, parameters)
	if err != nil {
		return nil, err
	}
	return []byte(hashed), nil
}

func (service *Service) rehash(ctx context.Context, userID int64, password string, parameters Argon2Parameters) error {
	hash, err := service.hashPassword(ctx, password, parameters)
	if err != nil {
		return err
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
			ID: userID, PasswordHash: []byte(hash),
		})
	})
}

// How far the failed-sign-in limit (sign_in_failure_limit) stretches beyond one person on one network:
// a network is a household or a carrier's shared address, many people's failures; and a person
// everywhere slows guessing spread across many networks without letting a stranger lock them out by
// their name alone, which the account-wide limit at sign_in_failure_limit did (the second audit's
// K2, 2026-10-13).
const (
	networkTimes           = 4
	accountEverywhereTimes = 10
)

// signInKeys are a sign-in's keys in the failed-sign-in limit: the person on the asking network, the
// person everywhere, and the asking network (clientnet: an IPv4 address or an IPv6 /64, so one
// network's many IPv6 addresses are one).
type signInKeys struct {
	accountHere, accountEverywhere, network string
}

func signInLimitKeys(name, clientAddress string) signInKeys {
	network := clientnet.Of(clientAddress)
	if network == "" {
		network = clientAddress
	}
	account := accountLimitKey(name)
	return signInKeys{accountHere: account + "@" + network, accountEverywhere: account, network: "network:" + network}
}

// accountLimitKey is a person's key in the failed-sign-in limit.
func accountLimitKey(name string) string {
	return "account:" + strings.ToLower(textindex.Line(name))
}

// isProvenDevice is whether proof is a session token issued to the person userID on deviceID, live
// or not: what lets a client resume the device it was.
func isProvenDevice(ctx context.Context, queries *sqlcgen.Queries, userID, deviceID int64, proof string) (bool, error) {
	if proof == "" || deviceID == 0 {
		return false, nil
	}
	issued, err := store.OrNotFound(queries.GetSessionDevice(ctx, tokenHash(proof)))
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up a device's proof: %w", err)
	}
	return ptr.ValueOrZero(issued.UserID) == userID && ptr.ValueOrZero(issued.DeviceID) == deviceID, nil
}

// resumeOrMakeDevice is the device a sign-in is on: the one the client remembers, when it is the
// person's, not removed, and the client proves it was it (D221 §1), else a new one.
func resumeOrMakeDevice(
	ctx context.Context, queries *sqlcgen.Queries, userID int64, device NewDevice, now int64,
) (int64, error) {
	isProven, err := isProvenDevice(ctx, queries, userID, device.PreviousDeviceID, device.PreviousDeviceProof)
	if err != nil {
		return 0, err
	}
	if isProven {
		known, liveErr := liveDevice(ctx, queries, userID, device.PreviousDeviceID)
		if liveErr == nil {
			touch := sqlcgen.RecordDeviceSignInParams{
				Platform: textindex.Line(device.Platform), SignedInAt: now, ID: known.ID,
			}
			if touchErr := queries.RecordDeviceSignIn(ctx, touch); touchErr != nil {
				return 0, fmt.Errorf("resume device %d: %w", known.ID, touchErr)
			}
			return known.ID, nil
		}
		if !errors.Is(liveErr, ErrNoDevice) {
			return 0, liveErr
		}
	}
	id, insertErr := queries.InsertDevice(ctx, sqlcgen.InsertDeviceParams{
		UserID: &userID, Name: textindex.Line(device.Name), Platform: textindex.Line(device.Platform), SignedInAt: now,
	})
	if insertErr != nil {
		return 0, fmt.Errorf("record the device: %w", insertErr)
	}
	return id, nil
}

// issueSession signs the user in on a device, resumed or made, with a session grant of scope tied to
// it, and returns its token, which is stored only as its SHA-256 (D154).
func (service *Service) issueSession(
	ctx context.Context, userID int64, device NewDevice, scope string,
) (string, error) {
	token, err := randomSecret(service.random)
	if err != nil {
		return "", err
	}
	now := service.clock.Now().UnixMilli()
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		deviceID, deviceErr := resumeOrMakeDevice(ctx, queries, userID, device, now)
		if deviceErr != nil {
			return deviceErr
		}
		return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			SecretSha256: tokenHash(token), Kind: grantKindSession, UserID: &userID, Scope: scope,
			IssuedAt: now, DeviceID: &deviceID,
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
	grant, err := store.OrNotFound(service.database.Read().GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
		SecretSha256: hash, Now: new(now.UnixMilli()),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, service.whyNotLive(ctx, hash)
	}
	if err == nil && (grant.Kind != grantKindSession || grant.UserID == nil) {
		return Principal{}, ErrNotAuthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up the session: %w", err)
	}
	idleExpiry, useWriteInterval, err := service.sessionPolicy(ctx)
	if err != nil {
		return Principal{}, err
	}
	// Sessions end on idle, not absolutely (D154 §3). issued_at stands in for a session never used.
	lastUsedAt := grant.IssuedAt
	if grant.LastUsedAt != nil {
		lastUsedAt = *grant.LastUsedAt
	}
	if now.Sub(time.UnixMilli(lastUsedAt)) > idleExpiry {
		return Principal{}, ErrSessionExpired
	}
	user, err := liveUser(ctx, service.database.Read(), *grant.UserID)
	if errors.Is(err, ErrNoUser) {
		// A session whose person is gone is no session: the client is signed out, not told of a
		// missing person.
		return Principal{}, ErrNotAuthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	isDeviceScoped := grant.Scope == grantScopeDevice
	capabilities := sessionCapabilities(user.Capabilities, isDeviceScoped)
	reach, err := reachOfRole(ctx, service.database.Read(), user.RoleID, capabilities)
	if err != nil {
		return Principal{}, err
	}
	if now.Sub(time.UnixMilli(lastUsedAt)) >= useWriteInterval {
		err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
			return queries.UpdateGrantLastUsed(ctx, sqlcgen.UpdateGrantLastUsedParams{
				SecretSha256: hash, LastUsedAt: new(now.UnixMilli()),
			})
		})
		if err != nil {
			return Principal{}, fmt.Errorf("record the session's use: %w", err)
		}
	}
	return Principal{
		UserID: user.ID, Name: user.Name, Role: Role(user.RoleName),
		Capabilities: capabilities, Reach: reach,
		DeviceID: ptr.ValueOrZero(grant.DeviceID), SessionKey: hex.EncodeToString(hash),
		isDeviceScoped: isDeviceScoped,
	}, nil
}

// sessionCapabilities are a session's capabilities: its role's, stored, but for administering in a
// device-scoped session.
func sessionCapabilities(stored string, isDeviceScoped bool) []string {
	capabilities := strings.Split(stored, ",")
	if isDeviceScoped {
		capabilities = slices.DeleteFunc(capabilities, func(each string) bool { return each == capabilityAdminister })
	}
	return capabilities
}

// whyNotLive says why a token finds no live session: ended, expired, or never one of ours.
func (service *Service) whyNotLive(ctx context.Context, hash []byte) error {
	state, err := store.OrNotFound(service.database.Read().GetGrantState(ctx, hash))
	switch {
	case errors.Is(err, store.ErrNotFound) || (err == nil && state.Kind != grantKindSession):
		return ErrNotAuthenticated
	case err != nil:
		return fmt.Errorf("look up the session: %w", err)
	case state.RevokedAt != nil:
		return ErrSessionEnded
	}
	return ErrSessionExpired
}

// SignOut revokes a session; revoking one already gone is not an error.
func (service *Service) SignOut(ctx context.Context, token string) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.MarkGrantRevoked(ctx, sqlcgen.MarkGrantRevokedParams{
			SecretSha256: tokenHash(token), RevokedAt: new(service.clock.Now().UnixMilli()),
		})
	})
}

func tokenHash(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}

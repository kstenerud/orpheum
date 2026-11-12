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
	"sync/atomic"
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
const capabilityHostWatchTogether = "hostWatchTogether"

// capabilityEditMetadata lets a role fix identification and metadata (the role table's
// capabilities; the built-in admin role carries it).
const capabilityEditMetadata = "editMetadata"

// Grant kinds and scopes this package mints (schema.md, grant; D154 §3).
const (
	grantKindSession = "session"
	grantScopeRole   = "role" // a session carries the user's role, entire (D154 §3)
	// A device-scoped session carries the role but for administering and the account's security:
	// a device a code activated, whoever entered the code (D292 §4; the security audit's A11). A
	// password sign-in carries the whole role wherever it comes from, a discovery's page included
	// (D294 §4).
	grantScopeDevice = "device"
	// A device secret is an issued secret that proves a client was a device: what a sign-in resumes
	// the device by (the third audit's A8).
	grantScopeDeviceSecret = "deviceSecret"
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
	// isGuestReceiver is a guest receiver's session (D465 §2): no capability, and refused wherever a
	// person signs in (IsGuestReceiver).
	isGuestReceiver bool
	// AdminAway is the person's own word on administering from away from home (D302): AdminAwayNever
	// or AdminAwayAlways.
	AdminAway string
}

// The values of an admin's setting for administering from away from home (D302).
const (
	AdminAwayNever  = "never"
	AdminAwayAlways = "always"
)

// AdministersFromAway reports whether the principal administers from any network, not only from
// home: an admin who set it so (D302). From home, IsAdmin alone decides.
func (principal Principal) AdministersFromAway() bool {
	return principal.IsAdmin() && principal.AdminAway == AdminAwayAlways
}

// CanManageAccount reports whether the session may change the account's security: its password,
// its devices, and the codes it enters for other devices. A device-scoped session may not (D292 §4;
// the security audit's A11).
func (principal Principal) CanManageAccount() bool {
	return !principal.isDeviceScoped && !principal.isGuestReceiver
}

// IsGuestReceiver reports whether the session is a guest receiver's (D465 §2): a Google Cast device
// signed in for one cast, which may receive that cast and play its films, and nothing else.
func (principal Principal) IsGuestReceiver() bool {
	return principal.isGuestReceiver
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
	// handOffs are the sessions a loader's sign-in made, waiting for the client origin (D308 §4).
	handOffs *handOffs
	// guestGrants are the grants for Google Cast devices waiting to be traded (D465 §2).
	guestGrants *guestGrants
	// hashing is the turn each password hash waits for (hashesAtOnce).
	hashing hashSlots
	// onLinksChanged is told when a link or a locator is made (OnLinksChanged); nil tells nothing.
	onLinksChanged func()
	// linkSealer seals each link's secret for its row, so a person's own link can be shown to them
	// again (SealLinksWith); nil keeps none, and the link cannot be shown.
	linkSealer LinkSealer
	// uses are the sessions used since their use was last written (FlushSessionUses).
	uses sessionUses
	// realm is the realm the core is found by at its discovery (SetRealm), a string; none, or empty,
	// and no realm key is made.
	realm atomic.Value
	// serviceDeviceCodes claims the codes the paid service issued (ClaimDeviceCodesFrom); nil claims
	// none.
	serviceDeviceCodes ServiceDeviceCodes
	// onActivated is told of each device a code activates (OnDeviceActivated); nil tells no one.
	onActivated func(context.Context, Activation)
}

// OnSignInLimitFull has onFull told when the failed-sign-in limit holds all the keys it may and
// refuses an attempt that needs another, at most once a minute: only an attack from thousands of
// networks at once fills it (failureLimiter's maxLimiterKeys).
func (service *Service) OnSignInLimitFull(onFull func()) {
	service.limiter.tellWhenFull(onFull)
}

// NewService returns a Service.
func NewService(database *store.Database, clockSource clock.Clock, random io.Reader, settings Settings) *Service {
	return &Service{
		database: database, clock: clockSource, random: random, settings: settings,
		limiter: newFailureLimiter(clockSource), pairings: &pairings{bySecret: map[string]*pairing{}},
		handOffs:    &handOffs{byCode: map[string]handOff{}},
		guestGrants: &guestGrants{byHash: map[string]guestGrant{}},
		hashing:     newHashSlots(),
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
	if err != nil {
		return id, err
	}
	return id, service.keepRealmKeyOfUser(ctx, id, user.Password)
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
	// PreviousDeviceProof is that device's secret, which the sign-in that made or resumed it gave: an
	// id alone let any client that named one sit on another's device, unseen in the list and kept by
	// "sign out everywhere else" (the second audit's K1 and W4), and a session token as the proof
	// handed whoever the client named its device to a working session (the third audit's A8).
	PreviousDeviceProof string
}

// SignIn checks a name and password and issues a session on the device the client remembers,
// when it is the person's, or on a new one the client named (D221 §1). The per-address failure
// limit counts by clientAddress, the client's network address. A hash made under older
// parameters is re-hashed.
//
// Every failure, whatever failed, is answered no sooner than signinFailureAnswerSeconds after the
// sign-in began, and a success as soon as it is done (D312 §17): an unknown name already costs a
// hash, and the floor hides what the hash slots' wait and the limits would still tell.
func (service *Service) SignIn(
	ctx context.Context, name, password, clientAddress string, device NewDevice,
) (Issued, error) {
	began := service.clock.Now()
	issued, err := service.signIn(ctx, name, password, clientAddress, device)
	if err != nil {
		service.answerFailureAt(ctx, began)
	}
	return issued, err
}

func (service *Service) signIn(
	ctx context.Context, name, password, clientAddress string, device NewDevice,
) (Issued, error) {
	keys := signInLimitKeys(name, clientAddress)
	attemptedAt, err := service.admitSignIn(ctx, name, device, keys)
	if err != nil {
		return Issued{}, err
	}
	parameters, err := service.argon2Parameters(ctx)
	if err != nil {
		return Issued{}, err
	}
	user, err := store.OrNotFound(service.database.Read().GetLiveUserByName(ctx, textindex.Line(name)))
	if errors.Is(err, store.ErrNotFound) {
		// An unknown name costs what a known one does, so timing does not tell them apart.
		_, hashErr := service.hashPassword(ctx, password, parameters)
		return Issued{}, errors.Join(ErrBadCredentials, hashErr)
	}
	if err != nil {
		return Issued{}, fmt.Errorf("look up user: %w", err)
	}
	isMatch, hashParameters, err := service.verifyPassword(ctx, password, string(user.PasswordHash))
	if err != nil || !isMatch {
		return Issued{}, errors.Join(ErrBadCredentials, err)
	}
	service.limiter.withdraw(attemptedAt, keys.network, keys.accountEverywhere)
	service.limiter.forget(keys.accountHere)
	if !sameCosts(hashParameters, parameters) {
		if rehashErr := service.rehash(ctx, user.ID, password, parameters); rehashErr != nil {
			return Issued{}, rehashErr
		}
	}
	// A person who has no realm key for the core's realm yet, as after the realm was set, has one
	// from this sign-in's password (D297 §3).
	if keepErr := service.keepRealmKey(ctx, user.ID, user.Name, password, false); keepErr != nil {
		return Issued{}, keepErr
	}
	return service.issueSession(ctx, user.ID, device, grantScopeRole)
}

// admitSignIn holds a sign-in to the failed sign-in limits, refusing it as ErrRateLimited past one, or
// waiting the pace past the person's everywhere; it returns when the attempt was counted. The person
// everywhere slows a sign-in rather than refusing it, and a device that proves it was signed in before
// is not held to it at all: a stranger failing in someone's name from many networks locked them out
// everywhere (the third audit's A13).
func (service *Service) admitSignIn(
	ctx context.Context, name string, device NewDevice, keys signInKeys,
) (time.Time, error) {
	maxFailures, window, err := service.failurePolicy(ctx)
	if err != nil {
		return time.Time{}, err
	}
	stretch, err := service.failureStretchOf(ctx)
	if err != nil {
		return time.Time{}, err
	}
	isKnownDevice, err := service.provesAnEarlierSignIn(ctx, name, device)
	if err != nil {
		return time.Time{}, err
	}
	pacing := map[string]int64{}
	if !isKnownDevice {
		pacing[keys.accountEverywhere] = maxFailures * stretch.everywhere
	}
	attemptedAt, isAllowed, isPaced := service.limiter.attempt(window, map[string]int64{
		keys.accountHere: maxFailures, keys.network: maxFailures * stretch.network,
	}, pacing)
	if !isAllowed {
		return time.Time{}, ErrRateLimited
	}
	if isPaced {
		if err = service.clock.Sleep(ctx, stretch.pace); err != nil {
			return time.Time{}, fmt.Errorf("wait before a sign-in the account's failures slow: %w", err)
		}
	}
	return attemptedAt, nil
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

// provesAnEarlierSignIn is whether device proves, by its device secret, that it is a device of the
// person name names: one that signed in before, which the person-everywhere limit does not hold.
func (service *Service) provesAnEarlierSignIn(ctx context.Context, name string, device NewDevice) (bool, error) {
	if device.PreviousDeviceProof == "" {
		return false, nil
	}
	queries := service.database.Read()
	user, err := store.OrNotFound(queries.GetLiveUserByName(ctx, textindex.Line(name)))
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up user: %w", err)
	}
	return isProvenDevice(ctx, queries, service.clock.Now().UnixMilli(), user.ID, device.PreviousDeviceID,
		device.PreviousDeviceProof)
}

// signInKeys are a sign-in's keys in the failed-sign-in limit: the person on the asking site, the
// person everywhere, and the asking site (clientnet.SiteOf: an IPv4 address or an IPv6 /48, so a
// customer's 65,536 /64s are one, the third audit's A13).
type signInKeys struct {
	accountHere, accountEverywhere, network string
}

func signInLimitKeys(name, clientAddress string) signInKeys {
	network := clientnet.SiteOf(clientAddress)
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

// isProvenDevice is whether proof is the live device secret of the person userID's device deviceID:
// what lets a client resume the device it was. A session token never is: a proof that was a live
// token handed every core or discovery a client named its device to a working session (the third
// audit's A8).
func isProvenDevice(
	ctx context.Context, queries *sqlcgen.Queries, now, userID, deviceID int64, proof string,
) (bool, error) {
	if proof == "" || deviceID == 0 {
		return false, nil
	}
	issued, err := store.OrNotFound(queries.GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
		SecretSha256: tokenHash(proof), Now: &now,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up a device's proof: %w", err)
	}
	return issued.Kind == grantKindIssuedSecret && issued.Scope == grantScopeDeviceSecret &&
		ptr.ValueOrZero(issued.UserID) == userID && ptr.ValueOrZero(issued.DeviceID) == deviceID, nil
}

// resumeOrMakeDevice is the device a sign-in is on: the one the client remembers, when it is the
// person's, not removed, and the client proves it was it (D221 §1), else a new one.
func resumeOrMakeDevice(
	ctx context.Context, queries *sqlcgen.Queries, userID int64, device NewDevice, now int64,
) (int64, error) {
	isProven, err := isProvenDevice(ctx, queries, now, userID, device.PreviousDeviceID, device.PreviousDeviceProof)
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

// Issued is a new session: its token, and the device secret by which its device is resumed at a later
// sign-in, which the client keeps apart from the token and never sends anywhere but its own core
// (the third audit's A8).
type Issued struct {
	Token        string
	DeviceSecret string
	// Locator is what the device finds its core by at a discovery, handed over the direct path only
	// (discovery-core.md K7).
	Locator string
	// DeviceID is the device the session is signed in on.
	DeviceID int64
}

// issueSession signs the user in on a device, resumed or made, with a session grant of scope tied to
// it and a new device secret in place of any the device had, and returns both, each stored only as
// its SHA-256 (D154).
func (service *Service) issueSession(
	ctx context.Context, userID int64, device NewDevice, scope string,
) (Issued, error) {
	token, err := randomSecret(service.random)
	if err != nil {
		return Issued{}, err
	}
	deviceSecret, err := randomSecret(service.random)
	if err != nil {
		return Issued{}, err
	}
	locator, err := randomSecret(service.random)
	if err != nil {
		return Issued{}, err
	}
	now := service.clock.Now().UnixMilli()
	var deviceID int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var deviceErr error
		deviceID, deviceErr = resumeOrMakeDevice(ctx, queries, userID, device, now)
		if deviceErr != nil {
			return deviceErr
		}
		if insertErr := queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
			SecretSha256: tokenHash(token), Kind: grantKindSession, UserID: &userID, Scope: scope,
			IssuedAt: now, DeviceID: &deviceID,
		}); insertErr != nil {
			return insertErr
		}
		if locatorErr := setLocator(ctx, queries, tokenHash(token), locator, now); locatorErr != nil {
			return locatorErr
		}
		return replaceDeviceSecret(ctx, queries, userID, deviceID, deviceSecret, now)
	})
	if err != nil {
		return Issued{}, fmt.Errorf("record the session: %w", err)
	}
	service.linksChanged()
	return Issued{Token: token, DeviceSecret: deviceSecret, Locator: locator, DeviceID: deviceID}, nil
}

// replaceDeviceSecret makes secret the device's one device secret.
func replaceDeviceSecret(
	ctx context.Context, queries *sqlcgen.Queries, userID, deviceID int64, secret string, now int64,
) error {
	if err := queries.RevokeDeviceSecretsOfDevice(ctx, sqlcgen.RevokeDeviceSecretsOfDeviceParams{
		RevokedAt: &now, DeviceID: &deviceID,
	}); err != nil {
		return fmt.Errorf("revoke the device's old secret: %w", err)
	}
	return queries.InsertGrant(ctx, sqlcgen.InsertGrantParams{
		SecretSha256: tokenHash(secret), Kind: grantKindIssuedSecret, UserID: &userID,
		Scope: grantScopeDeviceSecret, IssuedAt: now, DeviceID: &deviceID,
	})
}

// Authenticate returns the principal a session token belongs to. It reads the session, its person
// and their reach in one read transaction (D303, M2), and writes nothing: the session's use is
// remembered, and FlushSessionUses writes it later (D303 §4, DL10).
func (service *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrNotAuthenticated
	}
	now := service.clock.Now()
	hash := tokenHash(token)
	idleExpiry, err := service.sessionIdleExpiry(ctx)
	if err != nil {
		return Principal{}, err
	}
	var principal Principal
	err = service.database.ReadTx(ctx, func(queries *sqlcgen.Queries) error {
		var readErr error
		principal, readErr = service.sessionPrincipal(ctx, queries, hash, now, idleExpiry)
		return readErr
	})
	if err != nil {
		return Principal{}, err
	}
	service.uses.note(hash, principal.DeviceID, now.UnixMilli())
	return principal, nil
}

// sessionPrincipal is who the live session hash belongs to, read through queries, or why there is
// no one: not a session, ended, or unused for longer than idleExpiry by the later of its stored
// and its remembered use.
func (service *Service) sessionPrincipal(
	ctx context.Context, queries *sqlcgen.Queries, hash []byte, now time.Time, idleExpiry time.Duration,
) (Principal, error) {
	grant, err := store.OrNotFound(queries.GetLiveGrant(ctx, sqlcgen.GetLiveGrantParams{
		SecretSha256: hash, Now: new(now.UnixMilli()),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, whyNotLive(ctx, queries, hash)
	}
	if err == nil && (grant.Kind != grantKindSession || grant.UserID == nil) {
		return Principal{}, ErrNotAuthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up the session: %w", err)
	}
	// Sessions end on idle, not absolutely (D154 §3). issued_at stands in for a session never used.
	lastUsedAt := grant.IssuedAt
	if grant.LastUsedAt != nil {
		lastUsedAt = *grant.LastUsedAt
	}
	if remembered, ok := service.uses.latest(hash); ok && remembered > lastUsedAt {
		lastUsedAt = remembered
	}
	if now.Sub(time.UnixMilli(lastUsedAt)) > idleExpiry {
		return Principal{}, ErrSessionExpired
	}
	user, err := liveUser(ctx, queries, *grant.UserID)
	if errors.Is(err, ErrNoUser) {
		// A session whose person is gone is no session: the client is signed out, not told of a
		// missing person.
		return Principal{}, ErrNotAuthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	isDeviceScoped, isGuestReceiver := grant.Scope == grantScopeDevice, grant.Scope == grantScopeGuestReceiver
	capabilities := sessionCapabilities(user.Capabilities, isDeviceScoped)
	if isGuestReceiver {
		capabilities = []string{}
	}
	// The role's reach, not the session's: a device scope takes away administering, never the
	// libraries an admin's role reaches by it (D292 §4; the production NAS's TV, 2026-10-20, saw none).
	reach, err := reachOfRole(ctx, queries, user.RoleID, strings.Split(user.Capabilities, ","))
	if err != nil {
		return Principal{}, err
	}
	return Principal{
		UserID: user.ID, Name: user.Name, Role: Role(user.RoleName),
		Capabilities: capabilities, Reach: reach,
		DeviceID: ptr.ValueOrZero(grant.DeviceID), SessionKey: hex.EncodeToString(hash),
		isDeviceScoped: isDeviceScoped, isGuestReceiver: isGuestReceiver, AdminAway: user.AdminAway,
	}, nil
}

// sessionCapabilities are a session's capabilities: its role's, stored, but in a device-scoped
// session for administering and for changing what the libraries say, which a device someone talked
// a person into activating should not do to the household (D292 §4, narrowed by the owner after the
// device code audit's M4, 2026-10-20).
func sessionCapabilities(stored string, isDeviceScoped bool) []string {
	capabilities := strings.Split(stored, ",")
	if isDeviceScoped {
		capabilities = slices.DeleteFunc(capabilities, func(each string) bool {
			return each == capabilityAdminister || each == capabilityEditMetadata
		})
	}
	return capabilities
}

// whyNotLive says why a token finds no live session: ended, expired, or never one of ours.
func whyNotLive(ctx context.Context, queries *sqlcgen.Queries, hash []byte) error {
	state, err := store.OrNotFound(queries.GetGrantState(ctx, hash))
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

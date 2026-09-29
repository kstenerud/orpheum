// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"crypto/rand"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// rfc9106Second is RFC 9106's second recommended option, the argon2id costs a new server starts
// with (D194 §2); the unit tests run under the cheap costs settingstest sets.
func rfc9106Second() Parameters {
	return Parameters{MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 4, SaltBytes: saltBytes, KeyBytes: keyBytes}
}

type fixture struct {
	database *store.Database
	settings *settings.Service
	clock    *clocktest.Fake
	service  *Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	database := storetest.OpenMigrated(t)
	now := clocktest.NewFake(time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC))
	seeded := settingstest.Seeded(t, database)
	return fixture{
		database: database, settings: seeded, clock: now, service: NewService(database, now, rand.Reader, seeded),
	}
}

func (fixture fixture) createUser(t *testing.T, name, password string, role Role) int64 {
	t.Helper()
	id, err := fixture.service.CreateUser(t.Context(), NewUser{Name: name, Password: password, Role: role})
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", name, err)
	}
	return id
}

func TestHashPassword_IsAPHCStringThatVerifies(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("correct horse", rfc9106Second(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\$argon2id\$v=19\$m=65536,t=3,p=4\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`).
		MatchString(hash) {
		t.Errorf("hash %q is not an argon2id PHC string with RFC 9106's second parameters", hash)
	}
	matches, used, err := VerifyPassword("correct horse", hash)
	if err != nil || !matches || !sameCosts(used, rfc9106Second()) {
		t.Errorf("VerifyPassword(right) = %v, %+v, %v", matches, used, err)
	}
	if matches, _, err := VerifyPassword("wrong horse", hash); err != nil || matches {
		t.Errorf("VerifyPassword(wrong) = %v, %v; want false", matches, err)
	}
	for _, malformed := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", "$argon2id$v=19$m=1$AA$AA"} {
		if _, _, err := VerifyPassword("x", malformed); !errors.Is(err, ErrMalformedHash) {
			t.Errorf("VerifyPassword(hash %q) error = %v, want ErrMalformedHash", malformed, err)
		}
	}
}

func TestCreateUser_StoresOnlyTheHash(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	fixture.createUser(t, "owner", "CANARY-password", RoleAdmin)
	user, err := fixture.database.Read().GetLiveUserByName(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	stored := string(user.PasswordHash)
	if strings.Contains(stored, "CANARY-password") || !strings.HasPrefix(stored, phcPrefix) {
		t.Errorf("password_hash = %q, want an argon2id hash that does not contain the password", user.PasswordHash)
	}
	_, err = fixture.service.CreateUser(t.Context(), NewUser{Name: "owner", Password: "x", Role: RoleUser})
	if !errors.Is(err, ErrNameTaken) {
		t.Errorf("a second user named owner: error = %v, want ErrNameTaken", err)
	}
}

// laptop is the device the tests sign in on.
var laptop = NewDevice{Name: "Firefox on Linux", Platform: "web"}

func TestSignIn_IssuesASessionUntilSignedOut(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "owner", "right", RoleAdmin)
	token, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	principal, err := fixture.service.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if principal.UserID != id || principal.Role != RoleAdmin || !principal.IsAdmin() {
		t.Errorf("principal = %+v, want the admin owner", principal)
	}
	second, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", NewDevice{Name: "TV", Platform: "web"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := fixture.service.Authenticate(ctx, second)
	if err != nil || principal.DeviceID == 0 || other.DeviceID == 0 || other.DeviceID == principal.DeviceID {
		t.Errorf("two sign-ins are on devices %d and %d (%v), want two new devices (D217 §8)",
			principal.DeviceID, other.DeviceID, err)
	}
	if err := fixture.service.SignOut(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Authenticate(ctx, token); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("Authenticate(revoked) error = %v, want ErrNotAuthenticated", err)
	}
	for _, token := range []string{"", "not-a-token"} {
		if _, err := fixture.service.Authenticate(ctx, token); !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Authenticate(%q) error = %v, want ErrNotAuthenticated", token, err)
		}
	}
}

func TestSignIn_SaysNothingAboutWhichPartWasWrong(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	fixture.createUser(t, "partner", "right", RoleUser)
	for _, attempt := range [][2]string{{"partner", "wrong"}, {"nobody", "right"}} {
		_, err := fixture.service.SignIn(t.Context(), attempt[0], attempt[1], "192.0.2.1", laptop)
		if !errors.Is(err, ErrBadCredentials) || err.Error() != ErrBadCredentials.Error() {
			t.Errorf("SignIn(%s, %s) error = %v, want exactly ErrBadCredentials", attempt[0], attempt[1], err)
		}
	}
}

func TestSignIn_RateLimitsRepeatedFailuresBySetting(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	const limit, window = 3, time.Minute
	settingstest.Set(t, fixture.settings, settings.SignInFailureLimit, "3")
	settingstest.Set(t, fixture.settings, settings.SignInFailureWindowS, "60")
	fixture.createUser(t, "partner", "right", RoleUser)
	for range limit {
		_, err := fixture.service.SignIn(ctx, "partner", "wrong", "198.51.100.7", laptop)
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("a failed sign-in: %v", err)
		}
	}
	_, err := fixture.service.SignIn(ctx, "partner", "right", "203.0.113.9", laptop)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("the right password after %d failures, from another address: %v, want ErrRateLimited",
			limit, err)
	}
	_, err = fixture.service.SignIn(ctx, "someone", "right", "198.51.100.7", laptop)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("another account from the failing address: %v, want ErrRateLimited", err)
	}
	fixture.clock.Advance(window + time.Second)
	if _, err := fixture.service.SignIn(ctx, "partner", "right", "198.51.100.7", laptop); err != nil {
		t.Errorf("after the window: %v, want the sign-in to succeed", err)
	}
}

func TestSignIn_RehashesAPasswordMadeUnderOlderCosts(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	fixture.createUser(t, "owner", "right", RoleAdmin)
	settingstest.Set(t, fixture.settings, settings.Argon2Iterations, "2")
	if _, err := fixture.service.SignIn(t.Context(), "owner", "right", "192.0.2.1", laptop); err != nil {
		t.Fatal(err)
	}
	user, err := fixture.database.Read().GetLiveUserByName(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, used, err := VerifyPassword("right", string(user.PasswordHash)); err != nil || used.Iterations != 2 {
		t.Errorf("after signing in under newer costs, the hash uses %+v (%v); want t=2", used, err)
	}
}

func TestAuthenticate_EndsASessionUnusedForTheIdleExpiry(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	settingstest.Set(t, fixture.settings, settings.SessionIdleExpiryH, "1")
	settingstest.Set(t, fixture.settings, settings.SessionLastSeenWriteIntervalS, "60")
	fixture.createUser(t, "owner", "right", RoleAdmin)
	token, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		fixture.clock.Advance(50 * time.Minute)
		if _, err = fixture.service.Authenticate(ctx, token); err != nil {
			t.Fatalf("a session used every 50 minutes, under a 1 hour idle expiry: %v", err)
		}
	}
	fixture.clock.Advance(61 * time.Minute)
	if _, err = fixture.service.Authenticate(ctx, token); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("a session unused for 61 minutes: %v, want ErrNotAuthenticated", err)
	}
}

func TestSignIn_HealsAMissingSettingWithItsStartingValue(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	fixture.createUser(t, "owner", "right", RoleAdmin)
	storetest.ExecRaw(t, fixture.database.Path(), "DELETE FROM setting WHERE key = 'signin_failure_limit'")
	if _, err := fixture.service.SignIn(t.Context(), "owner", "right", "192.0.2.1", laptop); err != nil {
		t.Fatalf("signing in with signin_failure_limit removed: %v, want the row healed and the sign-in to work", err)
	}
	if limit, err := fixture.settings.Int(t.Context(), settings.SignInFailureLimit); err != nil || limit != 5 {
		t.Errorf("the removed setting = %d, %v; want its starting value back in the database", limit, err)
	}
}

// memorySetupCodes is the state directory's setup-code file, in memory.
type memorySetupCodes struct {
	code string
}

func (codes *memorySetupCodes) Read() (string, error) {
	if codes.code == "" {
		return "", ErrNoSetupCode
	}
	return codes.code, nil
}

func (codes *memorySetupCodes) Write(code string) error {
	codes.code = code
	return nil
}

func (codes *memorySetupCodes) Remove() error {
	codes.code = ""
	return nil
}

func TestSetup_ACodeAnEarlierReleaseStoredInAnotherAlphabetIsReplaced(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	codes := &memorySetupCodes{code: "7KQM-2XHP-9RTW"}
	code, err := fixture.service.EnsureSetupCode(t.Context(), codes)
	if err != nil || code == "7KQM-2XHP-9RTW" || codes.code != code {
		t.Errorf("EnsureSetupCode = %q, %v, stored %q; want a new safe32 code, stored", code, err, codes.code)
	}
}

func TestSetup_TheCodeFromTheLogCreatesTheFirstAdminOnce(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	codes := &memorySetupCodes{}
	code, err := fixture.service.EnsureSetupCode(ctx, codes)
	if err != nil {
		t.Fatal(err)
	}
	safe32Group := `[0-9a-hjkmnp-tv-z]{4}`
	if !regexp.MustCompile(`^` + safe32Group + `-` + safe32Group + `-` + safe32Group + `$`).MatchString(code) {
		t.Errorf("setup code %q is not three groups of four safe32 characters (D223 §2)", code)
	}
	again, err := fixture.service.EnsureSetupCode(ctx, codes)
	if err != nil || again != code {
		t.Errorf("a restart before setup: code %q, %v; want the same %q", again, err, code)
	}

	admin := NewUser{Name: "owner", Password: "right"}
	_, err = fixture.service.CompleteSetup(ctx, codes, "AAAA-AAAA-AAAA", admin)
	if !errors.Is(err, ErrWrongSetupCode) {
		t.Errorf("a wrong code: %v, want ErrWrongSetupCode", err)
	}
	// Typed as a person reads it from the log: in capitals, with spaces, an O for a 0.
	typed := strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(code), "-", " "), "0", "O")
	id, err := fixture.service.CompleteSetup(ctx, codes, typed, admin)
	if err != nil {
		t.Fatalf("the right code: %v", err)
	}
	token, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := fixture.service.Authenticate(ctx, token)
	if err != nil || principal.UserID != id || !principal.IsAdmin() {
		t.Errorf("the first user is %+v, %v; want an admin", principal, err)
	}
	if codes.code != "" {
		t.Error("the setup code survived setup")
	}
	_, err = fixture.service.CompleteSetup(ctx, codes, code, NewUser{Name: "intruder", Password: "x"})
	if !errors.Is(err, ErrSetupComplete) {
		t.Errorf("setup a second time: %v, want ErrSetupComplete", err)
	}
	if code, err := fixture.service.EnsureSetupCode(ctx, codes); err != nil || code != "" {
		t.Errorf("after setup, EnsureSetupCode = %q, %v; want no code", code, err)
	}
}

// BenchmarkHashPassword_DefaultParameters measures argon2id at the production costs. D194 §2 sets
// a 500 ms ceiling on devnas; the verification queue records that run.
func BenchmarkHashPassword_DefaultParameters(b *testing.B) {
	for b.Loop() {
		if _, err := HashPassword("correct horse battery staple", rfc9106Second(), rand.Reader); err != nil {
			b.Fatal(err)
		}
	}
}

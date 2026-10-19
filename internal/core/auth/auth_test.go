// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// rfc9106SecondParameters is RFC 9106's second recommended option, the argon2id costs a new server starts
// with (D194 §2); the unit tests run under the cheap costs settingstest sets.
func rfc9106SecondParameters() Argon2Parameters {
	return Argon2Parameters{
		MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 4, SaltBytes: saltBytes, KeyBytes: keyBytes,
	}
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
	fakeClock := clocktest.NewFake(time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC))
	seeded := settingstest.Seeded(t, database)
	return fixture{
		database: database, settings: seeded, clock: fakeClock,
		service: NewService(database, fakeClock, rand.Reader, seeded),
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
	hash, err := HashPassword("correct horse", rfc9106SecondParameters(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\$argon2id\$v=19\$m=65536,t=3,p=4\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`).
		MatchString(hash) {
		t.Errorf("hash %q is not an argon2id PHC string with RFC 9106's second parameters", hash)
	}
	matches, used, err := VerifyPassword("correct horse", hash)
	if err != nil || !matches || !sameCosts(used, rfc9106SecondParameters()) {
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
	token, err := tokenOf(fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop))
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
	second, err := tokenOf(fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1",
		NewDevice{Name: "TV", Platform: "web"}))
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

func TestAuthenticate_NamesTheSessionWithoutItsCredential(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "owner", "right", RoleAdmin)
	first, err := tokenOf(fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop))
	if err != nil {
		t.Fatal(err)
	}
	second, err := tokenOf(fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop))
	if err != nil {
		t.Fatal(err)
	}
	keyOf := func(token string) string {
		principal, err := fixture.service.Authenticate(ctx, token)
		if err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		return principal.SessionKey
	}
	firstKey := keyOf(first)
	if firstKey == "" || firstKey != keyOf(first) {
		t.Errorf("one session's keys are %q and %q, want one non-empty key", firstKey, keyOf(first))
	}
	if firstKey == keyOf(second) {
		t.Errorf("two sessions share the key %q, want one each", firstKey)
	}
	if strings.Contains(firstKey, first) {
		t.Errorf("session key %q holds the token", firstKey)
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

// Every failed sign-in is answered at one time after it began, whatever failed, the limits' refusal
// too, and a success at once (D312 §17).
func TestSignIn_EveryFailureIsAnsweredAtOneTimeAndASuccessAtOnce(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	settingstest.Set(t, fixture.settings, settings.SignInFailureLimit, "1") // the third is refused by it
	fixture.createUser(t, "partner", "right", RoleUser)
	const answer = 2 * time.Second
	for _, attempt := range []struct{ name, password, address string }{
		{"partner", "wrong", "192.0.2.1"}, {"nobody", "right", "192.0.2.1"}, {"partner", "right", "192.0.2.1"},
	} {
		began := fixture.clock.Now()
		_, err := fixture.service.SignIn(ctx, attempt.name, attempt.password, attempt.address, laptop)
		if took := fixture.clock.Now().Sub(began); err == nil || took != answer {
			t.Errorf("SignIn(%s, %s): %v after %s; want a failure after %s", attempt.name, attempt.password,
				err, took, answer)
		}
	}
	began := fixture.clock.Now()
	if _, err := fixture.service.SignIn(ctx, "partner", "right", "198.51.100.1", laptop); err != nil ||
		!fixture.clock.Now().Equal(began) {
		t.Errorf("the right password: %v after %s; want signed in at once", err, fixture.clock.Now().Sub(began))
	}
}

// Failed sign-ins are limited by the setting for a person on one site, a site (an IPv6 /48 is one,
// however many addresses it uses) at four times it, and past ten times it a person everywhere is
// slowed, not refused; a stranger's failures elsewhere do not lock the person out, and the limits
// lift after the window (the second audit's K2, 2026-10-13: an account-wide limit at the setting
// let anyone lock anyone out by name, and the address limit counted single IPv6 addresses; the third
// audit's A13).
func TestSignIn_RateLimitsRepeatedFailuresBySetting(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	const limit, window = 3, 5 * time.Minute
	settingstest.Set(t, fixture.settings, settings.SignInFailureLimit, "3")
	settingstest.Set(t, fixture.settings, settings.SignInFailureWindowSeconds, "300")
	// Each failure is answered after the floor, which moves the fake clock; thirty at the starting
	// two seconds would outlast the minute this test counts them in, so the window is longer.
	fixture.createUser(t, "partner", "right", RoleUser)
	fixture.createUser(t, "someone", "right", RoleUser)
	for index := range limit {
		_, err := fixture.service.SignIn(ctx, "partner", "wrong", fmt.Sprintf("2001:db8:7:7::%x", index+1), laptop)
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("a failed sign-in: %v", err)
		}
	}
	if _, err := fixture.service.SignIn(ctx, "partner", "right", "2001:db8:7:7::99", laptop); !errors.Is(err,
		ErrRateLimited) {
		t.Errorf("the right password after %d failures on that /64, from another of its addresses: %v, want "+
			"ErrRateLimited", limit, err)
	}
	if _, err := fixture.service.SignIn(ctx, "partner", "right", "203.0.113.9", laptop); err != nil {
		t.Errorf("the person from another network after a stranger's failures: %v, want signed in", err)
	}
	if _, err := fixture.service.SignIn(ctx, "someone", "right", "2001:db8:7:7::5", laptop); err != nil {
		t.Errorf("another person on the failing network, below its limit: %v, want signed in", err)
	}
	for index := range limit * accountEverywhereTimes {
		_, _ = tokenOf(fixture.service.SignIn(ctx, "someone", "wrong", //nolint:errcheck // failing on purpose
			fmt.Sprintf("198.51.100.%d", index+1), laptop))
	}
	// Past the person-everywhere limit a sign-in is slowed, not refused: a stranger's failures from
	// many networks no longer lock the person out (the third audit's A13; D293 §9).
	before := fixture.clock.Now()
	if _, err := fixture.service.SignIn(ctx, "someone", "right", "192.0.2.200", laptop); err != nil ||
		fixture.clock.Now().Sub(before) < accountPace {
		t.Errorf("after %d failures spread over as many networks: %v after %s; want signed in after %s",
			limit*accountEverywhereTimes, err, fixture.clock.Now().Sub(before), accountPace)
	}
	fixture.clock.Advance(window + time.Second)
	if _, err := fixture.service.SignIn(ctx, "partner", "right", "2001:db8:7:7::1", laptop); err != nil {
		t.Errorf("after the window: %v, want the sign-in to succeed", err)
	}
}

// Sign-ins tried at once each count against the limit: the check made before the slow hash and the
// failure recorded after it let 53 of 60 parallel guesses through (the second audit's K2).
func TestSignIn_GuessesMadeAtOnceEachCountAgainstTheLimit(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	settingstest.Set(t, fixture.settings, settings.SignInFailureLimit, "3")
	fixture.createUser(t, "partner", "right", RoleUser)
	const guesses = 30
	tried := make(chan error, guesses)
	var started sync.WaitGroup
	for range guesses {
		started.Go(func() {
			_, err := fixture.service.SignIn(ctx, "partner", "wrong", "198.51.100.7", laptop)
			tried <- err
		})
	}
	started.Wait()
	close(tried)
	checked := 0
	for err := range tried {
		if errors.Is(err, ErrBadCredentials) {
			checked++
		}
	}
	if checked > 3 {
		t.Errorf("%d of %d guesses made at once were checked; want at most the limit, 3", checked, guesses)
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
	settingstest.Set(t, fixture.settings, settings.SessionIdleExpiryHours, "1")
	settingstest.Set(t, fixture.settings, settings.SessionLastUsedWriteIntervalSeconds, "60")
	fixture.createUser(t, "owner", "right", RoleAdmin)
	token, err := tokenOf(fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop))
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

// BenchmarkHashPassword_DefaultParameters measures argon2id at the production costs. D194 §2 sets
// a 500 ms ceiling on devnas; the verification queue records that run.
func BenchmarkHashPassword_DefaultParameters(b *testing.B) {
	for b.Loop() {
		if _, err := HashPassword("correct horse battery staple", rfc9106SecondParameters(), rand.Reader); err != nil {
			b.Fatal(err)
		}
	}
}

// tokenOf is an issued session's token, for a test that needs only that.
func tokenOf(issued Issued, err error) (string, error) {
	return issued.Token, err
}

// A site's many IPv6 networks are one network in the limit: an IPv6 /48 (the third audit's A13).
func TestSignIn_ASitesNetworksCountAsOne(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	settingstest.Set(t, fixture.settings, settings.SignInFailureLimit, "3")
	fixture.createUser(t, "partner", "right", RoleUser)
	for index := range 3 {
		address := fmt.Sprintf("2001:db8:7:%x::1", index+1)
		if _, err := fixture.service.SignIn(ctx, "partner", "wrong", address, laptop); !errors.Is(err,
			ErrBadCredentials) {
			t.Fatalf("a failed sign-in: %v", err)
		}
	}
	if _, err := fixture.service.SignIn(ctx, "partner", "right", "2001:db8:7:ffff::1", laptop); !errors.Is(err,
		ErrRateLimited) {
		t.Errorf("after three failures from three /64s of one /48, from a fourth: %v, want ErrRateLimited", err)
	}
}

// A device that proves it signed in before, by its device secret, is not slowed by the person's
// failures everywhere (the third audit's A13).
func TestSignIn_AKnownDeviceIsNotSlowedByTheAccountsFailuresElsewhere(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	settingstest.Set(t, fixture.settings, settings.SignInFailureLimit, "3")
	fixture.createUser(t, "partner", "right", RoleUser)
	known, err := fixture.service.SignIn(ctx, "partner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := fixture.service.Authenticate(ctx, known.Token)
	if err != nil {
		t.Fatal(err)
	}
	for index := range 3 * accountEverywhereTimes {
		_, _ = fixture.service.SignIn(ctx, "partner", "wrong", //nolint:errcheck // failing on purpose
			fmt.Sprintf("198.51.100.%d", index+1), laptop)
	}
	before := fixture.clock.Now()
	device := NewDevice{
		Name: laptop.Name, Platform: laptop.Platform, PreviousDeviceID: principal.DeviceID,
		PreviousDeviceProof: known.DeviceSecret,
	}
	if _, err = fixture.service.SignIn(ctx, "partner", "right", "203.0.113.5", device); err != nil ||
		fixture.clock.Now() != before {
		t.Errorf("the known device's sign-in: %v after %s; want signed in at once", err,
			fixture.clock.Now().Sub(before))
	}
}

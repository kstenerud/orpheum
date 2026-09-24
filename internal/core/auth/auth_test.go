// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"crypto/rand"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// cheapParameters keep the unit tests fast; the production costs are measured by the benchmark.
func cheapParameters() Parameters {
	return Parameters{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32}
}

type fixture struct {
	database *store.Database
	clock    *clocktest.Fake
	service  *Service
}

func newFixture(t *testing.T, parameters Parameters) fixture {
	t.Helper()
	database := storetest.Open(t)
	now := clocktest.NewFake(time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC))
	service, err := NewService(database, now, rand.Reader, parameters)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{database: database, clock: now, service: service}
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
	hash, err := HashPassword("correct horse", DefaultParameters(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\$argon2id\$v=19\$m=65536,t=3,p=4\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`).
		MatchString(hash) {
		t.Errorf("hash %q is not an argon2id PHC string with RFC 9106's second parameters", hash)
	}
	matches, used, err := VerifyPassword("correct horse", hash)
	if err != nil || !matches || !sameCosts(used, DefaultParameters()) {
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
	fixture := newFixture(t, cheapParameters())
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

func TestSignIn_IssuesASessionUntilSignedOut(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, cheapParameters())
	ctx := t.Context()
	id := fixture.createUser(t, "owner", "right", RoleAdmin)
	token, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1")
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
	fixture := newFixture(t, cheapParameters())
	fixture.createUser(t, "partner", "right", RoleUser)
	for _, attempt := range [][2]string{{"partner", "wrong"}, {"nobody", "right"}} {
		_, err := fixture.service.SignIn(t.Context(), attempt[0], attempt[1], "192.0.2.1")
		if !errors.Is(err, ErrBadCredentials) || err.Error() != ErrBadCredentials.Error() {
			t.Errorf("SignIn(%s, %s) error = %v, want exactly ErrBadCredentials", attempt[0], attempt[1], err)
		}
	}
}

func TestSignIn_RateLimitsRepeatedFailures(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, cheapParameters())
	ctx := t.Context()
	fixture.createUser(t, "partner", "right", RoleUser)
	for range failureLimit {
		_, err := fixture.service.SignIn(ctx, "partner", "wrong", "198.51.100.7")
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("a failed sign-in: %v", err)
		}
	}
	if _, err := fixture.service.SignIn(ctx, "partner", "right", "203.0.113.9"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("the right password after %d failures, from another address: %v, want ErrRateLimited",
			failureLimit, err)
	}
	if _, err := fixture.service.SignIn(ctx, "someone", "right", "198.51.100.7"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("another account from the failing address: %v, want ErrRateLimited", err)
	}
	fixture.clock.Advance(failureWindow + time.Second)
	if _, err := fixture.service.SignIn(ctx, "partner", "right", "198.51.100.7"); err != nil {
		t.Errorf("after the window: %v, want the sign-in to succeed", err)
	}
}

func TestSignIn_RehashesAPasswordMadeUnderOlderCosts(t *testing.T) {
	t.Parallel()
	older := cheapParameters()
	fixture := newFixture(t, older)
	fixture.createUser(t, "owner", "right", RoleAdmin)
	newer := older
	newer.Iterations = 2
	upgraded, err := NewService(fixture.database, fixture.clock, rand.Reader, newer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.SignIn(t.Context(), "owner", "right", "192.0.2.1"); err != nil {
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

func TestSetup_TheCodeFromTheLogCreatesTheFirstAdminOnce(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, cheapParameters())
	ctx := t.Context()
	codes := &memorySetupCodes{}
	code, err := fixture.service.EnsureSetupCode(ctx, codes)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[2-9A-HJ-NP-Z]{4}-[2-9A-HJ-NP-Z]{4}-[2-9A-HJ-NP-Z]{4}$`).MatchString(code) {
		t.Errorf("setup code %q is not three groups of four unambiguous characters", code)
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
	id, err := fixture.service.CompleteSetup(ctx, codes, strings.ToLower(code), admin)
	if err != nil {
		t.Fatalf("the right code: %v", err)
	}
	token, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1")
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
		if _, err := HashPassword("correct horse battery staple", DefaultParameters(), rand.Reader); err != nil {
			b.Fatal(err)
		}
	}
}

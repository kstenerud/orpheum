// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// storedLastUse is when the database says the session was last used.
func storedLastUse(t *testing.T, database *store.Database, token string) int64 {
	t.Helper()
	state, err := database.Read().GetGrantState(t.Context(), tokenHash(token))
	if err != nil {
		t.Fatal(err)
	}
	if state.LastUsedAt == nil {
		return 0
	}
	return *state.LastUsedAt
}

// A request that authenticates writes nothing; its use is written by the next flush (D303 §4, DL10).
func TestAuthenticate_WritesNothingAndTheFlushWritesTheUse(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "owner", "right", RoleAdmin)
	token, err := tokenOf(fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop))
	if err != nil {
		t.Fatal(err)
	}
	signedIn := storedLastUse(t, fixture.database, token)
	fixture.clock.Advance(10 * time.Minute)
	if _, err = fixture.service.Authenticate(ctx, token); err != nil {
		t.Fatal(err)
	}
	if got := storedLastUse(t, fixture.database, token); got != signedIn {
		t.Errorf("authenticating wrote the session's use (%d, was %d); want it remembered until the flush",
			got, signedIn)
	}
	if err = fixture.service.FlushSessionUses(ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := storedLastUse(t, fixture.database, token), fixture.clock.Now().UnixMilli(); got != want {
		t.Errorf("after the flush the session's use is %d; want %d, when it authenticated", got, want)
	}
}

// A use written late never moves the stored time back.
func TestFlushSessionUses_NeverMovesAStoredUseBack(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "owner", "right", RoleAdmin)
	token, err := tokenOf(fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop))
	if err != nil {
		t.Fatal(err)
	}
	later := fixture.clock.Now().Add(time.Hour).UnixMilli()
	err = fixture.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpdateGrantLastUsed(ctx, sqlcgen.UpdateGrantLastUsedParams{
			SecretSha256: tokenHash(token), LastUsedAt: new(later),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.service.uses.note(tokenHash(token), 0, fixture.clock.Now().UnixMilli())
	if err = fixture.service.FlushSessionUses(ctx); err != nil {
		t.Fatal(err)
	}
	if got := storedLastUse(t, fixture.database, token); got != later {
		t.Errorf("a flush of an earlier use moved the stored use to %d; want it left at %d", got, later)
	}
}

// A session used just before its idle expiry, whose use is not yet written, still leads a discovery
// to the core: the locators wait one flush interval past the idle expiry.
func TestLiveLocators_KeepASessionWhoseUseIsNotWrittenYet(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	settingstest.Set(t, fixture.settings, settings.SessionIdleExpiryHours, "1")
	settingstest.Set(t, fixture.settings, settings.SessionLastUsedWriteIntervalSeconds, "60")
	fixture.createUser(t, "owner", "right", RoleAdmin)
	issued, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", NewDevice{Name: "laptop"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(time.Hour + 30*time.Second)
	hashes, err := fixture.service.LiveLocators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(hashes, []string{link.LocatorHash(issued.Locator)}) {
		t.Errorf("30 s past the idle expiry, within a flush, the live locators are %v; want the session's", hashes)
	}
}

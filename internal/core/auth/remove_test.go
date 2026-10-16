// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// setPreference stores one of a person's own preferences, as a played film would.
func setPreference(t *testing.T, database *store.Database, userID int64) {
	t.Helper()
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.SetPreference(t.Context(), sqlcgen.SetPreferenceParams{
			UserID: userID, Scope: "user", Axis: "dialogue_boost", ValueJSON: "true", SetAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// preferenceCount is how many of the person's own preferences are stored.
func preferenceCount(t *testing.T, database *store.Database, userID int64) int {
	t.Helper()
	listed, err := database.Read().ListPreferencesAtScope(t.Context(), sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	return len(listed)
}

func TestRemoveUser_EndsTheirAccessDeletesTheirOwnRowsAndFreesTheName(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	partner := fixture.createUser(t, "partner", "pw", RoleUser)
	owner := fixture.createUser(t, "owner", "pw", RoleAdmin)
	phone, television := phoneAndTelevision(t, fixture.service, partner)
	link, err := fixture.service.IssueRegistrationInvitation(ctx, partner)
	if err != nil {
		t.Fatal(err)
	}
	setPreference(t, fixture.database, partner)
	setPreference(t, fixture.database, owner)

	if err = fixture.service.RemoveUser(ctx, owner, owner); !errors.Is(err, ErrRemoveSelf) {
		t.Errorf("an admin removing themselves: %v, want ErrRemoveSelf", err)
	}
	if err = fixture.service.RemoveUser(ctx, owner, partner, preferences.ForgetPreferencesOf); err != nil {
		t.Fatal(err)
	}

	wantSignedOutAndForgotten(t, fixture.service, fixture.database, removedPerson{
		userID: partner, linkSecret: link.Secret, sessions: map[string]string{"phone": phone, "television": television},
	})
	if count := preferenceCount(t, fixture.database, owner); count != 1 {
		t.Errorf("the admin's preferences: %d, want kept", count)
	}
	accounts, err := fixture.service.Accounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].ID != owner {
		t.Errorf("accounts = %+v, %v; want only the admin", accounts, err)
	}
	if err = fixture.service.RemoveUser(ctx, owner, partner); !errors.Is(err, ErrNoUser) {
		t.Errorf("removing them again: %v, want ErrNoUser", err)
	}
	if again := fixture.createUser(t, "Partner", "pw", RoleUser); again == partner {
		t.Errorf("a new account with the removed person's name has their id %d; want a fresh one", again)
	}
}

// removedPerson is who was removed: their id, their registration link's secret, and their
// sessions by device.
type removedPerson struct {
	userID     int64
	linkSecret string
	sessions   map[string]string
}

// wantSignedOutAndForgotten checks a removed person: every device signed out and gone, their
// registration link and password refused, and their preferences deleted.
func wantSignedOutAndForgotten(t *testing.T, service *Service, database *store.Database, removed removedPerson) {
	t.Helper()
	ctx := t.Context()
	var err error
	for name, token := range removed.sessions {
		if _, err = service.Authenticate(ctx, token); !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("the removed person's %s: %v, want signed out", name, err)
		}
	}
	if devices, listErr := service.Devices(ctx, removed.userID); listErr != nil || len(devices) != 0 {
		t.Errorf("the removed person's devices = %+v, %v; want none listed", devices, listErr)
	}
	if _, err = service.Register(ctx, removed.linkSecret, "", "new-pw", laptop); !errors.Is(err, ErrBadInvitation) {
		t.Errorf("their registration link after removal: %v, want ErrBadInvitation", err)
	}
	if _, err = service.SignIn(ctx, "partner", "pw", "192.0.2.1", laptop); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("signing in as the removed person: %v, want ErrBadCredentials", err)
	}
	if count := preferenceCount(t, database, removed.userID); count != 0 {
		t.Errorf("the removed person's preferences: %d, want deleted (D173 §4.6)", count)
	}
}

// Removing a person deletes what was theirs, not pseudonymises it (D305 §1): their credentials,
// live or ended, their devices and their realm key go, and their row stays only as a nameless
// tombstone, its password and the admin's note on it gone.
func TestRemoveUser_LeavesNothingOfTheirsButANamelessRow(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	partner := fixture.createUser(t, "partner", "pw", RoleUser)
	owner := fixture.createUser(t, "owner", "pw", RoleAdmin)
	phoneAndTelevision(t, fixture.service, partner)
	storetest.ExecRaw(t, fixture.database.Path(), fmt.Sprintf(
		"UPDATE user SET admin_note = 'my sister' WHERE id = %d", partner),
		fmt.Sprintf("INSERT INTO realm_key (user_id, master_realm, key, made_at) VALUES (%d, 'home', X'01', 1)",
			partner))
	if err := fixture.service.RemoveUser(ctx, owner, partner); err != nil {
		t.Fatal(err)
	}
	path := fixture.database.Path()
	left := map[string][]string{
		"their row": storetest.QueryRaw(t, path, fmt.Sprintf(
			"SELECT name || '|' || coalesce(hex(password_hash), '') || '|' || coalesce(admin_note, '') FROM user "+
				"WHERE id = %d AND removed_at IS NOT NULL", partner)),
		"credentials": storetest.QueryRaw(t, path,
			fmt.Sprintf(`SELECT count(*) FROM "grant" WHERE user_id = %d`, partner)),
		"devices": storetest.QueryRaw(t, path, fmt.Sprintf("SELECT count(*) FROM device WHERE user_id = %d", partner)),
		"realm key": storetest.QueryRaw(t, path,
			fmt.Sprintf("SELECT count(*) FROM realm_key WHERE user_id = %d", partner)),
	}
	want := map[string][]string{"their row": {"||"}, "credentials": {"0"}, "devices": {"0"}, "realm key": {"0"}}
	for what, got := range left {
		if !slices.Equal(got, want[what]) {
			t.Errorf("after removal, %s: %v; want %v", what, got, want[what])
		}
	}
}

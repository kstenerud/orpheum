// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A person administers from home alone until an admin chose otherwise, and the choice is one of
// never and always (D302, step 0042).
func TestUser_AdministersFromAwayOnlyWhenSetAlways(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	storetest.ExecRaw(t, database.Path(),
		"INSERT INTO role (id, name, capabilities) VALUES (9, 'admins', 'administer')",
		"INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 9, 1)")
	ctx := t.Context()
	user, err := database.Read().GetLiveUserByID(ctx, 1)
	if err != nil || user.AdminAway != "never" {
		t.Fatalf("a new person's admin_away = %q, %v; want never", user.AdminAway, err)
	}
	set := func(value string) error {
		return database.Write(ctx, func(queries *sqlcgen.Queries) error {
			return queries.SetAdminAway(ctx, sqlcgen.SetAdminAwayParams{AdminAway: value, ID: 1})
		})
	}
	if err = set("always"); err != nil {
		t.Fatal(err)
	}
	if user, err = database.Read().GetLiveUserByID(ctx, 1); err != nil || user.AdminAway != "always" {
		t.Errorf("after setting always: %q, %v", user.AdminAway, err)
	}
	if err = set("sometimes"); err == nil {
		t.Error("admin_away took sometimes; want only never and always")
	}
}

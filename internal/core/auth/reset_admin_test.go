// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"slices"
	"testing"
)

// signsInAsAdmin is whether name, signing in with password, administers.
func signsInAsAdmin(t *testing.T, service *Service, name, password string) bool {
	t.Helper()
	token, err := service.SignIn(t.Context(), name, password, "192.0.2.10",
		NewDevice{Name: "Laptop", Platform: "web"})
	if err != nil {
		t.Fatalf("%s signing in: %v", name, err)
	}
	principal, err := service.Authenticate(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	return principal.IsAdmin()
}

// The operator's reset demotes everyone who administered, by the built-in role or one the admin
// made, and makes the person named the admin with the new password, ending their old sessions; no
// account is removed (D294).
func TestResetAdmin_DemotesEveryAdministratorAndMakesTheOneNamedTheAdmin(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "owner", "pw", RoleAdmin)
	deputyRole := RoleChange{Name: "deputy", Capabilities: []string{"administer"}}
	if _, err := fixture.service.CreateRole(ctx, deputyRole); err != nil {
		t.Fatal(err)
	}
	deputy := fixture.createUser(t, "deputy", "pw", RoleUser)
	if err := fixture.service.SetPersonsRole(ctx, deputy, "deputy"); err != nil {
		t.Fatal(err)
	}
	fixture.createUser(t, "partner", "old", RoleUser)
	held, err := fixture.service.SignIn(ctx, "partner", "old", "192.0.2.10",
		NewDevice{Name: "Phone", Platform: "web"})
	if err != nil {
		t.Fatal(err)
	}
	reset, err := fixture.service.ResetAdmin(ctx, "partner", "new")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(reset.Demoted)
	if !slices.Equal(reset.Demoted, []string{"deputy", "owner"}) || reset.IsCreated {
		t.Errorf("the reset did %+v; want owner and deputy demoted, no one made", reset)
	}
	if _, err = fixture.service.Authenticate(ctx, held); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("the new admin's old session = %v; want it ended", err)
	}
	if !signsInAsAdmin(t, fixture.service, "partner", "new") || signsInAsAdmin(t, fixture.service, "owner", "pw") ||
		signsInAsAdmin(t, fixture.service, "deputy", "pw") {
		t.Error("after the reset, want partner alone the admin, signing in with the new password")
	}
	if count, countErr := fixture.service.CountUsers(ctx); countErr != nil || count != 3 {
		t.Errorf("%d people after the reset, %v; want all three kept", count, countErr)
	}
}

// A reset naming a name no one has makes that account the admin (D294).
func TestResetAdmin_MakesTheAccountWhenNoOneHasTheName(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	fixture.createUser(t, "owner", "pw", RoleAdmin)
	reset, err := fixture.service.ResetAdmin(t.Context(), "rescue", "pw2")
	if err != nil || !reset.IsCreated || !slices.Equal(reset.Demoted, []string{"owner"}) {
		t.Errorf("a reset naming no one's name did %+v, %v; want rescue made, owner demoted", reset, err)
	}
	if !signsInAsAdmin(t, fixture.service, "rescue", "pw2") || signsInAsAdmin(t, fixture.service, "owner", "pw") {
		t.Error("want rescue the admin and owner not")
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A reach that was never read reaches no library; a role that administers reaches every one, and
// any other only those it lists, an item in no library among none of them (D243 §4).
func TestReach_FailsClosedAndIsWhatTheRoleLists(t *testing.T) {
	t.Parallel()
	var unread Reach
	one, three := int64(1), int64(3)
	if unread.Reaches(&one) || unread.Reaches(nil) || unread.IsEveryLibrary() {
		t.Error("the zero Reach reaches a library; want none")
	}
	if every := EveryLibrary(); !every.Reaches(&one) || !every.Reaches(nil) || !every.IsEveryLibrary() {
		t.Error("EveryLibrary does not reach every library and an item in none")
	}
	listed := LibrariesOnly(1)
	if !listed.Reaches(&one) || listed.Reaches(&three) || listed.Reaches(nil) {
		t.Error("LibrariesOnly(1) reaches other than library 1")
	}
	if isEvery, idsJSON := listed.QueryArgs(); isEvery != 0 || idsJSON != "[1]" {
		t.Errorf("LibrariesOnly(1)'s query arguments are %d %q, want 0 [1]", isEvery, idsJSON)
	}
	if isEvery, idsJSON := unread.QueryArgs(); isEvery != 0 || idsJSON != "[]" {
		t.Errorf("the zero Reach's query arguments are %d %q, want 0 []", isEvery, idsJSON)
	}
}

// A reach narrowed to one library reaches it alone, and only when it reached it; one less the
// libraries a person left out reaches the rest of what it reached (D349).
func TestReach_NarrowedToALibraryOrWithoutTheLeftOut(t *testing.T) {
	t.Parallel()
	one, two, three := int64(1), int64(2), int64(3)
	if narrowed, ok := EveryLibrary().Narrowed(2); !ok || narrowed.Reaches(&one) || !narrowed.Reaches(&two) {
		t.Error("every library narrowed to 2 reaches other than library 2")
	}
	if _, ok := LibrariesOnly(1).Narrowed(2); ok {
		t.Error("a role of library 1 narrowed to library 2 reaches it")
	}
	without := EveryLibrary().Without([]int64{2}, []int64{1, 2, 3})
	if !without.Reaches(&one) || without.Reaches(&two) || !without.Reaches(&three) {
		t.Error("every library less 2 is not libraries 1 and 3")
	}
	listed := LibrariesOnly(1, 2).Without([]int64{2, 3}, []int64{1, 2, 3})
	if !slices.Equal(listed.libraryIDs, []int64{1}) {
		t.Errorf("libraries 1 and 2 less 2 and 3 reach %v, want 1", listed.libraryIDs)
	}
	if every := EveryLibrary().Without(nil, nil); !every.IsEveryLibrary() {
		t.Error("every library less none is no longer every library")
	}
}

// A principal's reach is its role's, read as it is authenticated: a library given to the role, or
// taken back, counts from the next request (D243 §4).
func TestAuthenticate_ReadsTheRolesLibrariesAtEachRequest(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	storetest.ExecRaw(t, fixture.database.Path(),
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movie', 'tmdb', 'manual', 0, 1), (2, 'Private', 'movie', 'tmdb', 'manual', 0, 1)`)
	fixture.createUser(t, "owner", "owner-password", RoleAdmin)
	fixture.createUser(t, "partner", "partner-password", RoleUser)
	signIn := func(name, password string) string {
		t.Helper()
		token, err := tokenOf(fixture.service.SignIn(t.Context(), name, password, "192.0.2.1", laptop))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	owner, partner := signIn("owner", "owner-password"), signIn("partner", "partner-password")
	reachOf := func(token string) Reach {
		t.Helper()
		principal, err := fixture.service.Authenticate(t.Context(), token)
		if err != nil {
			t.Fatal(err)
		}
		return principal.Reach
	}
	if !reachOf(owner).IsEveryLibrary() {
		t.Error("the admin's reach is not every library")
	}
	library := func(id int64) *int64 { return &id }
	if reach := reachOf(partner); reach.Reaches(library(1)) || reach.Reaches(library(2)) {
		t.Error("the user role reaches a library no one gave it")
	}
	roles, err := fixture.service.ListRoles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	userRole := roles[slices.IndexFunc(roles, func(role RoleSummary) bool { return role.Name == "user" })]
	if err = fixture.service.UpdateRole(t.Context(), userRole.ID, RoleChange{
		Name: "user", Capabilities: userRole.Capabilities, Libraries: []RoleLibrary{{LibraryID: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	if reach := reachOf(partner); !reach.Reaches(library(1)) || reach.Reaches(library(2)) {
		t.Error("after the user role was given library 1, the partner does not reach it alone")
	}
}

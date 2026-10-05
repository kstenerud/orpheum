// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// listedRole is a role as GET /roles lists it.
type listedRole struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	IsBuiltin    bool     `json:"isBuiltin"`
	Capabilities []string `json:"capabilities"`
	Libraries    []struct {
		LibraryID int64 `json:"libraryId"`
	} `json:"libraries"`
}

// roleNamed is the role GET /roles lists under name, which fails the test when there is none.
func (harness testHarness) roleNamed(t *testing.T, admin, name string) listedRole {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/roles", admin, nil)
	var listed struct {
		Roles []listedRole `json:"roles"`
	}
	if err := json.Unmarshal([]byte(answer.body), &listed); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET /roles = %d %s", answer.status, answer.body)
	}
	for _, role := range listed.Roles {
		if role.Name == name {
			return role
		}
	}
	t.Fatalf("GET /roles lists no role %q: %s", name, answer.body)
	return listedRole{}
}

// giveRoleLibraries sets the libraries the role named roleName reaches, as the admin would.
func (harness testHarness) giveRoleLibraries(t *testing.T, admin, roleName string, libraryIDs ...int64) {
	t.Helper()
	role := harness.roleNamed(t, admin, roleName)
	libraries := make([]map[string]any, 0, len(libraryIDs))
	for _, id := range libraryIDs {
		libraries = append(libraries, map[string]any{"libraryId": id})
	}
	answer := harness.do(t, http.MethodPut, fmt.Sprintf("/roles/%d", role.ID), admin, map[string]any{
		"name": role.Name, "capabilities": role.Capabilities, "libraries": libraries,
	})
	if answer.status != http.StatusNoContent {
		t.Fatalf("PUT /roles/%d = %d %s", role.ID, answer.status, answer.body)
	}
}

// The admin makes a role with its capabilities and libraries, invites someone from outside the
// house into it, and the invitee reaches only the role's libraries; a role held cannot be deleted,
// and a person given another role reaches what it does from their next request (D243 §3, §4).
func TestRoles_AnInviteeReachesOnlyTheirRolesLibraries(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t) // library 1, Films
	harness.identifyFilm(t)
	made := harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "friends", "capabilities": []string{"host_watch_together"},
		"libraries": []map[string]any{{"libraryId": 1, "mayRebroadcast": true}},
	})
	if made.status != http.StatusCreated {
		t.Fatalf("POST /roles = %d %s", made.status, made.body)
	}
	if friends := harness.roleNamed(t, admin, "friends"); len(friends.Libraries) != 1 ||
		friends.Libraries[0].LibraryID != 1 || friends.IsBuiltin {
		t.Errorf("GET /roles lists friends as %+v; want library 1, made by the admin", friends)
	}
	invitation := harness.invite(t, admin, "friends", "my cousin")
	cousin, _ := readSignIn(t, harness.do(t, http.MethodPost, "/registration", "", map[string]any{
		"secret": invitation.Link.Secret, "name": "cousin", "password": "cousin-password", "device": testDevice,
	}))
	if page := harness.do(t, http.MethodGet, "/items/30", cousin, nil); page.status != http.StatusOK {
		t.Errorf("the invitee's role reaches library 1, yet GET /items/30 = %d %s", page.status, page.body)
	}
	wantError(t, harness.do(t, http.MethodDelete, fmt.Sprintf("/roles/%d", harness.roleNamed(t, admin, "friends").ID),
		admin, nil), http.StatusConflict, CodeConflict)

	harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "nothing", "capabilities": []string{}, "libraries": []any{},
	})
	var cousinAccount UserBody
	cousinMe := harness.do(t, http.MethodGet, "/me", cousin, nil)
	if err := json.Unmarshal([]byte(cousinMe.body), &cousinAccount); err != nil {
		t.Fatal(err)
	}
	moved := harness.do(t, http.MethodPut, fmt.Sprintf("/users/%d/role", cousinAccount.ID), admin,
		map[string]any{"role": "nothing"})
	if moved.status != http.StatusNoContent {
		t.Fatalf("PUT /users/%d/role = %d %s", cousinAccount.ID, moved.status, moved.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items/30", cousin, nil), http.StatusNotFound, CodeNotFound)
	friends := harness.roleNamed(t, admin, "friends")
	if deleted := harness.do(t, http.MethodDelete, fmt.Sprintf("/roles/%d", friends.ID), admin, nil); deleted.status !=
		http.StatusNoContent {
		t.Errorf("DELETE /roles/%d, held by no one now = %d %s", friends.ID, deleted.status, deleted.body)
	}
}

// A built-in role keeps its name and capabilities and is never deleted, and the admin role reaches
// every library and lists none; a role's capabilities come from the closed set, its libraries
// exist, each once, and its name is its own (D130, D243 §4).
func TestRoles_RefuseWhatARoleCannotBe(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	adminRole, userRole := harness.roleNamed(t, admin, "admin"), harness.roleNamed(t, admin, "user")
	for _, refused := range []struct {
		what   string
		method string
		path   string
		body   map[string]any
		code   Code
	}{
		{
			"renaming the user role", http.MethodPut, fmt.Sprintf("/roles/%d", userRole.ID),
			map[string]any{"name": "people", "capabilities": userRole.Capabilities, "libraries": []any{}},
			CodeConflict,
		},
		{"narrowing the admin role", http.MethodPut, fmt.Sprintf("/roles/%d", adminRole.ID), map[string]any{
			"name": "admin", "capabilities": adminRole.Capabilities,
			"libraries": []map[string]any{{"libraryId": 1}},
		}, CodeConflict},
		{"deleting a built-in role", http.MethodDelete, fmt.Sprintf("/roles/%d", userRole.ID), nil, CodeConflict},
		{
			"a capability not in the set", http.MethodPost, "/roles",
			map[string]any{"name": "x", "capabilities": []string{"fly"}, "libraries": []any{}},
			CodeInvalidRequest,
		},
		{
			"a library that does not exist", http.MethodPost, "/roles",
			map[string]any{"name": "x", "capabilities": []string{}, "libraries": []map[string]any{{"libraryId": 9}}},
			CodeInvalidRequest,
		},
		{"a library twice", http.MethodPost, "/roles", map[string]any{
			"name": "x", "capabilities": []string{},
			"libraries": []map[string]any{{"libraryId": 1}, {"libraryId": 1}},
		}, CodeInvalidRequest},
		{
			"another role's name", http.MethodPost, "/roles",
			map[string]any{"name": "user", "capabilities": []string{}, "libraries": []any{}},
			CodeConflict,
		},
	} {
		answer := harness.do(t, refused.method, refused.path, admin, refused.body)
		if answer.status < 400 || !strings.Contains(answer.body, `"code":"`+string(refused.code)+`"`) {
			t.Errorf("%s = %d %s; want %s", refused.what, answer.status, answer.body, refused.code)
		}
	}
	if userRole = harness.roleNamed(t, admin, "user"); len(userRole.Libraries) != 1 {
		t.Errorf("after the refusals the user role lists %+v; want its library 1 unchanged", userRole.Libraries)
	}
}

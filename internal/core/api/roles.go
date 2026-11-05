// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// RoleLibraryBody is one library a role reaches.
type RoleLibraryBody struct {
	LibraryID      int64 `json:"libraryId" minimum:"1"`
	MayRebroadcast bool  `doc:"Its members may add it to their own server; offered once rebroadcasting is built (D243 §5)" json:"mayRebroadcast,omitempty"` //nolint:lll // one struct tag
}

// RoleBody is a role as the admin sees it (D130, D243 §4).
type RoleBody struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	IsBuiltin    bool     `json:"isBuiltin"`
	Capabilities []string `json:"capabilities"`
	// IsEveryLibrary is a role that administers: it reaches every library and lists none.
	IsEveryLibrary bool              `doc:"Administers, so reaches every library" json:"isEveryLibrary"`
	Libraries      []RoleLibraryBody `json:"libraries"`
	HolderCount    int64             `doc:"How many users hold it" json:"holderCount"`
}

// RolesOutput is every role, and the capabilities a role may have.
type RolesOutput struct {
	Body struct {
		Roles        []RoleBody `json:"roles"`
		Capabilities []string   `doc:"The closed set a role's capabilities come from" json:"capabilities"`
	}
}

// RoleChangeBody is a role as the admin describes it: its libraries are taken as given, and one
// left out is taken back from its members at their next request.
type RoleChangeBody struct {
	Name         string            `json:"name" maxLength:"100" minLength:"1"`
	Capabilities []string          `json:"capabilities"`
	Libraries    []RoleLibraryBody `json:"libraries"`
}

// CreateRoleInput is the admin making a role.
type CreateRoleInput struct {
	AuthorizationHeader
	Body RoleChangeBody
}

// CreateRoleOutput is the new role's id.
type CreateRoleOutput struct {
	Body struct {
		ID int64 `json:"id"`
	}
}

// RoleInput names a role.
type RoleInput struct {
	AuthorizationHeader
	RoleID int64 `minimum:"1" path:"id"`
}

// UpdateRoleInput is the admin changing a role.
type UpdateRoleInput struct {
	AuthorizationHeader
	RoleID int64 `minimum:"1" path:"id"`
	Body   RoleChangeBody
}

// UserRoleInput gives a user a role.
type UserRoleInput struct {
	AuthorizationHeader
	UserID int64 `minimum:"1" path:"id"`
	Body   struct {
		Role string `doc:"The role's name" json:"role" maxLength:"100" minLength:"1"`
	}
}

func (server *Server) registerRoles() {
	tags := []string{tagUsers}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listRoles", Method: http.MethodGet, Path: "/roles",
		Summary: "Every role, with its capabilities and the libraries it reaches (admin; D243 §4)", Tags: tags,
	}, server.listRoles)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "createRole", Method: http.MethodPost, Path: "/roles", DefaultStatus: http.StatusCreated,
		Summary: "Make a role: its name, capabilities and libraries (admin; D243 §4)", Tags: tags,
	}, server.createRole)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "updateRole", Method: http.MethodPut, Path: "/roles/{id}", DefaultStatus: http.StatusNoContent,
		Summary: "Change a role; a library taken back is gone for its members at their next request " +
			"(admin; D243 §4)",
		Tags: tags,
	}, server.updateRole)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "deleteRole", Method: http.MethodDelete, Path: "/roles/{id}",
		DefaultStatus: http.StatusNoContent,
		Summary:       "Delete a role the admin made that no one holds (admin)", Tags: tags,
	}, server.deleteRole)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setUserRole", Method: http.MethodPut, Path: "/users/{id}/role",
		DefaultStatus: http.StatusNoContent,
		Summary:       "Give a user a role, their whole access, from their next request (admin; D243 §4)",
		Tags:          tags,
	}, server.setUserRole)
}

func (server *Server) listRoles(ctx context.Context, input *AuthorizationHeader) (*RolesOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	roles, err := dependencies.Auth.ListRoles(ctx)
	if err != nil {
		return nil, server.authError(ctx, "list roles", err)
	}
	output := &RolesOutput{}
	output.Body.Capabilities = auth.Capabilities()
	output.Body.Roles = make([]RoleBody, 0, len(roles))
	for _, role := range roles {
		body := RoleBody{
			ID: role.ID, Name: role.Name, IsBuiltin: role.IsBuiltin, Capabilities: role.Capabilities,
			IsEveryLibrary: role.IsEveryLibrary, HolderCount: role.HolderCount,
			Libraries: make([]RoleLibraryBody, 0, len(role.Libraries)),
		}
		for _, library := range role.Libraries {
			body.Libraries = append(body.Libraries, RoleLibraryBody(library))
		}
		output.Body.Roles = append(output.Body.Roles, body)
	}
	return output, nil
}

func (server *Server) createRole(ctx context.Context, input *CreateRoleInput) (*CreateRoleOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	id, err := dependencies.Auth.CreateRole(ctx, roleChangeOf(input.Body))
	if err != nil {
		return nil, server.authError(ctx, "make a role", err)
	}
	output := &CreateRoleOutput{}
	output.Body.ID = id
	return output, nil
}

func (server *Server) updateRole(ctx context.Context, input *UpdateRoleInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.UpdateRole(ctx, input.RoleID, roleChangeOf(input.Body)); err != nil {
		return nil, server.authError(ctx, "change a role", err)
	}
	return &struct{}{}, nil
}

func (server *Server) deleteRole(ctx context.Context, input *RoleInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.DeleteRole(ctx, input.RoleID); err != nil {
		return nil, server.authError(ctx, "delete a role", err)
	}
	return &struct{}{}, nil
}

func (server *Server) setUserRole(ctx context.Context, input *UserRoleInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.SetPersonsRole(ctx, input.UserID, input.Body.Role); err != nil {
		return nil, server.authError(ctx, "give a person a role", err)
	}
	return &struct{}{}, nil
}

// roleChangeOf is a role as the API describes it, as auth takes it.
func roleChangeOf(body RoleChangeBody) auth.RoleChange {
	change := auth.RoleChange{Name: body.Name, Capabilities: body.Capabilities}
	for _, library := range body.Libraries {
		change.Libraries = append(change.Libraries, auth.RoleLibrary(library))
	}
	return change
}

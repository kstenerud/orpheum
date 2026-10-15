// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// AdminAwayInput sets whether the admin signed in administers from away from home too (D302).
type AdminAwayInput struct {
	AuthorizationHeader
	Body struct {
		AdminAway string `enum:"never,always" json:"adminAway"`
	}
}

func (server *Server) registerAdminAway() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setAdminAway", Method: http.MethodPut, Path: "/me/admin-away",
		DefaultStatus: http.StatusNoContent, Tags: []string{tagAccounts},
		Summary: "Whether you administer from away from home too: never (the default) or always; set from home",
	}, server.setAdminAway)
}

// setAdminAway is an admin's own setting, changed from home alone (D302 §1): one administering from
// away could otherwise turn it back on from there, after another had turned it off at home.
func (server *Server) setAdminAway(ctx context.Context, input *AdminAwayInput) (*struct{}, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if !isFromHome(ctx) {
		return nil, newError(http.StatusForbidden, CodeForbidden,
			"administering from away is set from the server's own network alone: open it at home")
	}
	if err = dependencies.Auth.SetAdminAway(ctx, principal.UserID, input.Body.AdminAway); err != nil {
		return nil, server.authError(ctx, "set administering from away", err)
	}
	return &struct{}{}, nil
}

// adminAwayOf is an admin's setting as /me says it, and nothing for anyone else.
func adminAwayOf(principal auth.Principal) string {
	if !principal.IsAdmin() {
		return ""
	}
	if principal.AdminAway == "" {
		return auth.AdminAwayNever
	}
	return principal.AdminAway
}

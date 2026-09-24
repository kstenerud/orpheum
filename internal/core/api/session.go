// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// SetupInput creates the first admin with the code from the log.
type SetupInput struct {
	Body struct {
		SetupCode string `doc:"The one-time code the server logged at start" json:"setupCode" minLength:"1"`
		Name      string `json:"name"                                         minLength:"1"`
		Password  string `json:"password"                                     minLength:"1"`
	}
}

// UserBody is a user as the API shows one.
type UserBody struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Role       string `enum:"admin,user" json:"role"`
	IsAdmin    bool   `json:"isAdmin"`
	IsOutsider bool   `json:"isOutsider"`
}

// UserOutput answers with a user.
type UserOutput struct {
	Body UserBody
}

// SignInInput signs in with a name and password.
type SignInInput struct {
	Body struct {
		Name     string `json:"name"     minLength:"1"`
		Password string `json:"password" minLength:"1"`
	}
	clientAddress string
}

// Resolve records the client's address, for the per-address failure limit (D194 §2).
func (input *SignInInput) Resolve(ctx huma.Context) []error {
	host, _, err := net.SplitHostPort(ctx.RemoteAddr())
	if err != nil {
		host = ctx.RemoteAddr()
	}
	input.clientAddress = host
	return nil
}

// SessionOutput carries a new session's token. The client sends it back in the Authorization
// header; it never goes in a URL (D154).
type SessionOutput struct {
	Body struct {
		Token string   `json:"token"`
		User  UserBody `json:"user"`
	}
}

func (server *Server) registerSetupAndAuth() {
	tags := []string{"Setup and auth"}
	huma.Register(server.api, huma.Operation{
		OperationID: "setup", Method: http.MethodPost, Path: "/setup", DefaultStatus: http.StatusCreated,
		Summary: "Create the first admin with the one-time setup code from the log", Tags: tags,
	}, server.setup)
	huma.Register(server.api, huma.Operation{
		OperationID: "signIn", Method: http.MethodPost, Path: "/session", DefaultStatus: http.StatusCreated,
		Summary: "Sign in; returns a session token for the Authorization header", Tags: tags,
	}, server.signIn)
	huma.Register(server.api, huma.Operation{
		OperationID: "signOut", Method: http.MethodDelete, Path: "/session", DefaultStatus: http.StatusNoContent,
		Summary: "Sign out, revoking this session", Tags: tags,
	}, server.signOut)
	huma.Register(server.api, huma.Operation{
		OperationID: "getMe", Method: http.MethodGet, Path: "/me",
		Summary: "Who this session belongs to", Tags: tags,
	}, server.me)
}

func (server *Server) setup(ctx context.Context, input *SetupInput) (*UserOutput, error) {
	dependencies, err := server.ready()
	if err != nil {
		return nil, err
	}
	admin := auth.NewUser{Name: input.Body.Name, Password: input.Body.Password}
	id, err := dependencies.Auth.CompleteSetup(ctx, dependencies.SetupCodes, input.Body.SetupCode, admin)
	switch {
	case errors.Is(err, auth.ErrSetupComplete):
		return nil, newError(http.StatusConflict, CodeConflict, "setup is already complete; sign in instead")
	case errors.Is(err, auth.ErrWrongSetupCode):
		return nil, newError(http.StatusForbidden, CodeForbidden,
			"the setup code is wrong; it is in the server's log, or run `orpheum setup-code`")
	case err != nil:
		return nil, server.internal(ctx, "complete setup", err)
	}
	server.logger.InfoContext(ctx, "first admin created", "userId", id)
	return &UserOutput{Body: UserBody{ID: id, Name: admin.Name, Role: string(auth.RoleAdmin), IsAdmin: true}}, nil
}

func (server *Server) signIn(ctx context.Context, input *SignInInput) (*SessionOutput, error) {
	dependencies, err := server.ready()
	if err != nil {
		return nil, err
	}
	count, err := dependencies.Auth.CountUsers(ctx)
	if err != nil {
		return nil, server.internal(ctx, "count users", err)
	}
	if count == 0 {
		return nil, newError(http.StatusConflict, CodeSetupRequired,
			"no one can sign in until the first admin is created with the setup code")
	}
	token, err := dependencies.Auth.SignIn(ctx, input.Body.Name, input.Body.Password, input.clientAddress)
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		return nil, newError(http.StatusTooManyRequests, CodeRateLimited,
			"too many failed sign-ins; wait and try again")
	case errors.Is(err, auth.ErrBadCredentials):
		return nil, newError(http.StatusUnauthorized, CodeAuthRequired, "the name or password is wrong")
	case err != nil:
		return nil, server.internal(ctx, "sign in", err)
	}
	principal, err := dependencies.Auth.Authenticate(ctx, token)
	if err != nil {
		return nil, server.internal(ctx, "read the new session", err)
	}
	output := &SessionOutput{}
	output.Body.Token, output.Body.User = token, userBody(principal)
	return output, nil
}

func (server *Server) signOut(ctx context.Context, input *AuthorizationHeader) (*struct{}, error) {
	dependencies, _, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.SignOut(ctx, bearerToken(input.Authorization)); err != nil {
		return nil, server.internal(ctx, "sign out", err)
	}
	return &struct{}{}, nil
}

func (server *Server) me(ctx context.Context, input *AuthorizationHeader) (*UserOutput, error) {
	_, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return &UserOutput{Body: userBody(principal)}, nil
}

func userBody(principal auth.Principal) UserBody {
	return UserBody{
		ID: principal.UserID, Name: principal.Name, Role: string(principal.Role),
		IsAdmin: principal.IsAdmin(), IsOutsider: principal.IsOutsider,
	}
}

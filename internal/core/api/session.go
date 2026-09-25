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
	overTLS       bool
}

// Resolve records the client's address, for the per-address failure limit (D194 §2), and
// whether it came over TLS, for the media cookie.
func (input *SignInInput) Resolve(ctx huma.Context) []error {
	host, _, err := net.SplitHostPort(ctx.RemoteAddr())
	if err != nil {
		host = ctx.RemoteAddr()
	}
	input.clientAddress, input.overTLS = host, ctx.TLS() != nil
	return nil
}

// SessionOutput carries a new session's token. The client sends it back in the Authorization
// header; it never goes in a URL (D154). The same session is set as the media cookie, for what
// cannot send a header: an <img>.
type SessionOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		Token string   `json:"token"`
		User  UserBody `json:"user"`
	}
}

// SignOutInput signs a session out.
type SignOutInput struct {
	AuthorizationHeader
	overTLS bool
}

// Resolve records whether the request came over TLS, for clearing the media cookie.
func (input *SignOutInput) Resolve(ctx huma.Context) []error {
	input.overTLS = ctx.TLS() != nil
	return nil
}

// SignOutOutput clears the media cookie.
type SignOutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
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
	output := &SessionOutput{SetCookie: mediaCookie(token, input.overTLS)}
	output.Body.Token, output.Body.User = token, userBody(principal)
	return output, nil
}

func (server *Server) signOut(ctx context.Context, input *SignOutInput) (*SignOutOutput, error) {
	dependencies, _, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.SignOut(ctx, bearerToken(input.Authorization)); err != nil {
		return nil, server.internal(ctx, "sign out", err)
	}
	cleared := mediaCookie("", input.overTLS) //nolint:gosec // as mediaCookie (P20)
	cleared.MaxAge = -1
	return &SignOutOutput{SetCookie: cleared}, nil
}

// MeInput asks who a session belongs to.
type MeInput struct {
	AuthorizationHeader
	overTLS bool
}

// Resolve records whether the request came over TLS, for the media cookie.
func (input *MeInput) Resolve(ctx huma.Context) []error {
	input.overTLS = ctx.TLS() != nil
	return nil
}

// MeOutput is who the session belongs to, and the media cookie set again for it.
type MeOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      UserBody
}

// me answers who a session belongs to, and sets the media cookie to that session again: a
// client asks at start, so a browser whose cookie is missing, from an older path, or from
// another session gets the right one before it fetches an image or a video (D154, D216).
func (server *Server) me(ctx context.Context, input *MeInput) (*MeOutput, error) {
	_, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return &MeOutput{
		SetCookie: mediaCookie(bearerToken(input.Authorization), input.overTLS), Body: userBody(principal),
	}, nil
}

func userBody(principal auth.Principal) UserBody {
	return UserBody{
		ID: principal.UserID, Name: principal.Name, Role: string(principal.Role),
		IsAdmin: principal.IsAdmin(), IsOutsider: principal.IsOutsider,
	}
}

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
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Role is the name of the person's role, which may be one the admin made (D243 §4).
	Role    string `json:"role"`
	IsAdmin bool   `json:"isAdmin"`
	// CanHostWatchTogether is whether the person's role may host Watch Together, so a client offers
	// it only to them.
	CanHostWatchTogether bool `json:"canHostWatchTogether"`
	// HasNoLibrary is a person whose role the admin has given no library yet, so a client tells
	// them why their lists are empty (D243 §4).
	HasNoLibrary bool `json:"hasNoLibrary"`
	// Device is the device this session is on, so a person can see who is signed in where.
	Device *CurrentDeviceBody `json:"device,omitempty"`
}

// CurrentDeviceBody is the device a session is on; a client keeps its id to resume it (D221).
type CurrentDeviceBody struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// UserOutput answers with a user.
type UserOutput struct {
	Body UserBody
}

// SignInInput signs in with a name and password, on a device the client names (D217 §8).
type SignInInput struct {
	Body struct {
		Name     string        `json:"name"     minLength:"1"`
		Password string        `json:"password" minLength:"1"`
		Device   NewDeviceBody `json:"device"`
	}
	ClientFacts
}

// ClientFacts are what a sign-in needs to know of the request: the client's address, for the
// per-address failure limit (D194 §2), and whether it came over TLS, for the media cookie.
type ClientFacts struct {
	clientAddress string
	isOverTLS     bool
	// userAgent is the browser's own words for itself, for the log: an iPhone's player behaves
	// unlike a desktop's, and nothing said which a stalling viewer had (staging, 2026-10-06).
	userAgent string
}

// NewDeviceBody is how a client names itself: the device becomes one of the person's, listed
// and revocable.
type NewDeviceBody struct {
	Name             string `doc:"What the person will know it by: \"Chrome on Android\", \"Living room TV\"" json:"name"     maxLength:"100" minLength:"1"`                       //nolint:lll // one struct tag
	Platform         string `doc:"The kind of client: \"web\", \"cast\""                                    json:"platform" maxLength:"40"  minLength:"1"`                         //nolint:lll // one struct tag
	PreviousDeviceID int64  `doc:"The device this client was signed in as before, resumed when it is the same person's (D221)" json:"previousDeviceId,omitempty" required:"false"` //nolint:lll // one struct tag
}

func newDevice(body NewDeviceBody) auth.NewDevice {
	return auth.NewDevice{Name: body.Name, Platform: body.Platform, PreviousDeviceID: body.PreviousDeviceID}
}

// Resolve records the client's address and whether it came over TLS.
func (facts *ClientFacts) Resolve(ctx huma.Context) []error {
	host, _, err := net.SplitHostPort(ctx.RemoteAddr())
	if err != nil {
		host = ctx.RemoteAddr()
	}
	facts.clientAddress, facts.isOverTLS = host, ctx.TLS() != nil
	facts.userAgent = ctx.Header("User-Agent")
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
	isOverTLS bool
}

// Resolve records whether the request came over TLS, for clearing the media cookie.
func (input *SignOutInput) Resolve(ctx huma.Context) []error {
	input.isOverTLS = ctx.TLS() != nil
	return nil
}

// SignOutOutput clears the media cookie.
type SignOutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
}

func (server *Server) registerSetupAndAuth() {
	tags := []string{"Setup and auth"}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setup", Method: http.MethodPost, Path: "/setup", DefaultStatus: http.StatusCreated,
		Summary: "Create the first admin with the one-time setup code from the log", Tags: tags,
	}, server.setup)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "signIn", Method: http.MethodPost, Path: "/session", DefaultStatus: http.StatusCreated,
		Summary: "Sign in; returns a session token for the Authorization header", Tags: tags,
	}, server.signIn)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "signOut", Method: http.MethodDelete, Path: "/session", DefaultStatus: http.StatusNoContent,
		Summary: "Sign out, revoking this session", Tags: tags,
	}, server.signOut)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getMe", Method: http.MethodGet, Path: "/me",
		Summary: "Who this session belongs to", Tags: tags,
	}, server.getMe)
}

func (server *Server) setup(ctx context.Context, input *SetupInput) (*UserOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	admin := auth.NewUser{Name: input.Body.Name, Password: input.Body.Password}
	id, err := dependencies.Auth.CompleteSetup(ctx, dependencies.SetupCodes, input.Body.SetupCode, admin)
	if err != nil {
		return nil, server.authError(ctx, "complete setup", err)
	}
	server.logger.InfoContext(ctx, "first admin created", "userId", id)
	return &UserOutput{Body: UserBody{ID: id, Name: admin.Name, Role: string(auth.RoleAdmin), IsAdmin: true}}, nil
}

func (server *Server) signIn(ctx context.Context, input *SignInInput) (*SessionOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	count, err := dependencies.Auth.CountUsers(ctx)
	if err != nil {
		return nil, server.authError(ctx, "count users", err)
	}
	if count == 0 {
		return nil, newError(http.StatusConflict, CodeSetupRequired,
			"no one can sign in until the first admin is created with the setup code")
	}
	token, err := dependencies.Auth.SignIn(
		ctx, input.Body.Name, input.Body.Password, input.clientAddress, newDevice(input.Body.Device))
	if err != nil {
		return nil, server.authError(ctx, "sign in", err)
	}
	return server.newSessionOutput(ctx, dependencies, token, input.isOverTLS)
}

// newSessionOutput answers a sign-in, a registration or a device code with the new session: its
// token, who it is, and the media cookie.
func (server *Server) newSessionOutput(
	ctx context.Context, dependencies *Dependencies, token string, isOverTLS bool,
) (*SessionOutput, error) {
	principal, err := dependencies.Auth.Authenticate(ctx, token)
	if err != nil {
		return nil, server.logInternalError(ctx, "read the new session", err)
	}
	body, err := server.userBody(ctx, dependencies, principal)
	if err != nil {
		return nil, err
	}
	output := &SessionOutput{SetCookie: mediaCookie(token, isOverTLS)}
	output.Body.Token, output.Body.User = token, body
	return output, nil
}

func (server *Server) signOut(ctx context.Context, input *SignOutInput) (*SignOutOutput, error) {
	dependencies, _, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.SignOut(ctx, bearerToken(input.Authorization)); err != nil {
		return nil, server.authError(ctx, "sign out", err)
	}
	cleared := mediaCookie("", input.isOverTLS) //nolint:gosec // as mediaCookie (P20)
	cleared.MaxAge = -1
	return &SignOutOutput{SetCookie: cleared}, nil
}

// MeInput asks who a session belongs to.
type MeInput struct {
	AuthorizationHeader
	isOverTLS bool
}

// Resolve records whether the request came over TLS, for the media cookie.
func (input *MeInput) Resolve(ctx huma.Context) []error {
	input.isOverTLS = ctx.TLS() != nil
	return nil
}

// MeOutput is who the session belongs to, and the media cookie set again for it.
type MeOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      UserBody
}

// getMe answers who a session belongs to, and sets the media cookie to that session again: a
// client asks at start, so a browser whose cookie is missing, from an older path, or from
// another session gets the right one before it fetches an image or a video (D154, D216).
func (server *Server) getMe(ctx context.Context, input *MeInput) (*MeOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	body, err := server.userBody(ctx, dependencies, principal)
	if err != nil {
		return nil, err
	}
	return &MeOutput{SetCookie: mediaCookie(bearerToken(input.Authorization), input.isOverTLS), Body: body}, nil
}

// userBody is who a session belongs to, and the device it is on.
func (server *Server) userBody(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal,
) (UserBody, error) {
	body := UserBody{
		ID: principal.UserID, Name: principal.Name, Role: string(principal.Role),
		IsAdmin: principal.IsAdmin(), CanHostWatchTogether: principal.CanHostWatchTogether(),
		HasNoLibrary: principal.Reach.ReachesNone(),
	}
	if principal.DeviceID != 0 {
		device, err := dependencies.Auth.DeviceOf(ctx, principal.UserID, principal.DeviceID)
		if err != nil && !errors.Is(err, auth.ErrNoDevice) {
			return UserBody{}, server.logInternalError(ctx, "read the session's device", err)
		}
		if err == nil {
			body.Device = &CurrentDeviceBody{ID: device.ID, Name: device.Name}
		}
	}
	return body, nil
}

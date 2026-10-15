// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
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
	Role string `json:"role"`
	// IsAdmin is whether this session administers where this request came from: an admin's role, and
	// the house's network directly, so a page away from home offers nothing it may not do (D297 §6).
	IsAdmin bool `json:"isAdmin"`
	// CanHostWatchTogether is whether the person's role may host Watch Together, so a client offers
	// it only to them.
	CanHostWatchTogether bool `json:"canHostWatchTogether"`
	// CanManageAccount is whether this session may change the account's security (its password, its
	// devices, codes it enters for others): a client offers those only when it may (D292 §4).
	CanManageAccount bool `json:"canManageAccount"`
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
	// host is the name the client reached the server by, for the log: at a discovery's names, which
	// one won a device's race says which path reached the server (discovery-protocol.md §3.3).
	host string
	// over is how the request came, for the log: "webrtc" over a data channel, whose head names no
	// host or browser, else "http".
	over string
}

// NewDeviceBody is how a client names itself: the device becomes one of the person's, listed
// and revocable.
type NewDeviceBody struct {
	Name             string `doc:"What the person will know it by: \"Chrome on Android\", \"Living room TV\"" json:"name"     maxLength:"100" minLength:"1"`                       //nolint:lll // one struct tag
	Platform         string `doc:"The kind of client: \"web\", \"cast\""                                    json:"platform" maxLength:"40"  minLength:"1"`                         //nolint:lll // one struct tag
	PreviousDeviceID int64  `doc:"The device this client was signed in as before, resumed when it is the same person's (D221)" json:"previousDeviceId,omitempty" required:"false"` //nolint:lll // one struct tag
	// PreviousDeviceProof is the device secret a sign-in on that device gave, which proves the client
	// was it (the third audit's A8).
	PreviousDeviceProof string `doc:"The device secret a sign-in on that device gave: the proof it was this client" json:"previousDeviceProof,omitempty" maxLength:"100" required:"false"` //nolint:lll // one struct tag
}

func newDevice(body NewDeviceBody) auth.NewDevice {
	return auth.NewDevice{
		Name: body.Name, Platform: body.Platform, PreviousDeviceID: body.PreviousDeviceID,
		PreviousDeviceProof: body.PreviousDeviceProof,
	}
}

// Resolve records the client's address and whether it came over TLS.
func (facts *ClientFacts) Resolve(ctx huma.Context) []error {
	host, _, err := net.SplitHostPort(ctx.RemoteAddr())
	if err != nil {
		host = ctx.RemoteAddr()
	}
	facts.clientAddress, facts.isOverTLS = host, ctx.TLS() != nil
	facts.userAgent, facts.host, facts.over = ctx.Header("User-Agent"), ctx.Host(), "http"
	if rtcapi.IsOverWebRTC(ctx.Context()) {
		facts.over = "webrtc"
	}
	return nil
}

// SessionOutput carries a new session's token. The client sends it back in the Authorization
// header; it never goes in a URL (D154). The same session is set as the media cookie, for what
// cannot send a header: an <img>.
type SessionOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		Token string `json:"token"`
		// DeviceSecret proves this client was its device at a later sign-in or code request, as
		// previousDeviceProof: kept apart from the token, and sent to this core alone (the third
		// audit's A8).
		DeviceSecret string `json:"deviceSecret"`
		// Locator is what this device finds its core by at a discovery, handed over the direct path
		// alone (discovery-core.md K7; the discovery protocol's §3.1).
		Locator string   `json:"locator"`
		User    UserBody `json:"user"`
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
		OperationID: "getSetup", Method: http.MethodGet, Path: "/setup",
		Summary: "Whether the server still needs its first admin, so the sign-in page opens on setting it up",
		Tags:    tags,
	}, server.getSetup)
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
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "freshLocator", Method: http.MethodPost, Path: "/me/locator",
		Summary: "A fresh locator for this device, by which it finds its server at a discovery", Tags: tags,
	}, server.freshLocator)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getOwnLink", Method: http.MethodGet, Path: "/me/own-link",
		Summary: "Your own link, which leads your devices back to this server through its discovery", Tags: tags,
	}, server.getOwnLink)
}

// LocatorOutput is a device's fresh locator (discovery-core.md K7).
type LocatorOutput struct {
	Body struct {
		Locator string `json:"locator"`
	}
}

// freshLocator hands the signed-in device a fresh locator in place of its last, on each visit, and
// renews the person's own link (the discovery protocol's §3.1).
func (server *Server) freshLocator(ctx context.Context, input *MeInput) (*LocatorOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	locator, err := dependencies.Auth.FreshLocator(ctx, principal)
	if err != nil {
		return nil, server.authError(ctx, "hand the device a locator", err)
	}
	output := &LocatorOutput{}
	output.Body.Locator = locator
	return output, nil
}

// SetupOutput is whether the server still needs its first admin.
type SetupOutput struct {
	Body struct {
		IsRequired bool `doc:"No one can sign in until the first admin is made with the setup code" json:"isRequired"`
	}
}

// getSetup says whether the server needs its first admin, asked before anyone signs in: the sign-in
// page then opens on setting it up (agent-failures A31). It says nothing more, and the setup code
// still guards the setup itself.
func (server *Server) getSetup(ctx context.Context, _ *struct{}) (*SetupOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	count, err := dependencies.Auth.CountUsers(ctx)
	if err != nil {
		return nil, server.authError(ctx, "count users", err)
	}
	output := &SetupOutput{}
	output.Body.IsRequired = count == 0
	return output, nil
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
	return &UserOutput{Body: UserBody{
		ID: id, Name: admin.Name, Role: string(auth.RoleAdmin), IsAdmin: true, CanManageAccount: true,
	}}, nil
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
	issued, err := dependencies.Auth.SignIn(
		ctx, input.Body.Name, input.Body.Password, input.clientAddress, newDevice(input.Body.Device))
	if err != nil {
		return nil, server.authError(ctx, "sign in", err)
	}
	return server.newSessionOutput(ctx, dependencies, issued, input.isOverTLS)
}

// newSessionOutput answers a sign-in, a registration or a device code with the new session: its
// token, who it is, and the media cookie.
func (server *Server) newSessionOutput(
	ctx context.Context, dependencies *Dependencies, issued auth.Issued, isOverTLS bool,
) (*SessionOutput, error) {
	principal, err := dependencies.Auth.Authenticate(ctx, issued.Token)
	if err != nil {
		return nil, server.logInternalError(ctx, "read the new session", err)
	}
	body, err := server.userBody(ctx, dependencies, principal)
	if err != nil {
		return nil, err
	}
	output := &SessionOutput{SetCookie: mediaCookie(issued.Token, isOverTLS)}
	output.Body.Token, output.Body.DeviceSecret, output.Body.User = issued.Token, issued.DeviceSecret, body
	output.Body.Locator = issued.Locator
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
		IsAdmin: principal.IsAdmin() && isFromHome(ctx), CanHostWatchTogether: principal.CanHostWatchTogether(),
		CanManageAccount: principal.CanManageAccount(), HasNoLibrary: principal.Reach.ReachesNone(),
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

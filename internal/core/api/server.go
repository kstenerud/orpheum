// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

// Prefix is where the API lives (cut-a §5).
const Prefix = "/api/v1"

// documentVersion is the API contract's version in the OpenAPI document. It is not the binary's
// version, so the committed document changes only when the API does (D85).
const documentVersion = "1"

// Status is what /health reports (cut-a §5, Running it).
type Status string

// The statuses. Starting and migrating answer with a success status, so a container manager does
// not kill a server that is only busy (migrations.md, While migrating: never look dead).
const (
	StatusStarting  Status = "starting"
	StatusMigrating Status = "migrating"
	StatusReady     Status = "ready"
	StatusDegraded  Status = "degraded"
)

// Dependencies are what the API needs once the database is open and migrated.
type Dependencies struct {
	Auth       *auth.Service
	Settings   *settings.Service
	Secrets    *secrets.Store
	SetupCodes auth.SetupCodes
	// Chooser is what a person does about identification: choosing a match, and overrides.
	Chooser *identify.Chooser
	// Refresher queues a person's refresh of a title or series (D213).
	Refresher *identify.Refresher
	// Libraries is what an admin does with libraries: create, list, scan, and the scanner view.
	Libraries *library.Admin
	// Browser is what a person browses and searches.
	Browser *library.Browser
	// Artwork serves the images titles show.
	Artwork *artwork.Service
	// Playback decides how a file plays and says where its bytes are.
	Playback *playback.Service
	// NodeBytes reads a file's bytes from its node.
	NodeBytes NodeBytes
	// Jobs is the one work queue, the one whose runners run the jobs, for the job view and cancel.
	Jobs *jobs.Queue
	// NodeReports is what each connected node said about itself, for the admin to see.
	NodeReports NodeReports
	// System is what the System page shows of the server itself (D225).
	System System
	// ConfigFileKeys is the config file's keys as the server runs with them, for /config to show.
	ConfigFileKeys []ConfigFileKey
}

// health is the state /health reports. It is safe for concurrent use.
type health struct {
	mutex   sync.Mutex
	status  Status   // guarded by mutex
	detail  string   // guarded by mutex
	reasons []string // guarded by mutex
}

// Server is the API. It answers /health from the moment it exists, and everything else once Ready
// has handed it its dependencies; until then those answer "migrating".
type Server struct {
	humaAPI      huma.API
	mux          *http.ServeMux
	logger       *slog.Logger
	health       health
	dependencies atomic.Pointer[Dependencies]
}

// NewServer builds the API and registers every operation. UseErrorShape must have been called.
func NewServer(logger *slog.Logger) *Server {
	config := huma.DefaultConfig("Orpheum", documentVersion)
	config.DocsPath = "" // the docs page loads scripts from a CDN; the document itself is served
	config.CreateHooks = nil
	config.Info.Description = "The Orpheum media server's client API (cut-a §5). Errors have one shape: " +
		"an object whose error holds a code, a message, a detail and, for a reachability error, the failedHop."
	mux := http.NewServeMux()
	server := &Server{
		humaAPI: humago.NewWithPrefix(mux, Prefix, config),
		mux:     mux,
		logger:  logger.With("component", "api"),
	}
	server.health.status = StatusStarting
	server.registerHealth()
	server.registerSetupAndAuth()
	server.registerSettings()
	server.registerSecrets()
	server.registerIdentification()
	server.registerLibraries()
	server.registerBrowse()
	server.registerArt()
	server.registerDoctor()
	server.registerSystem()
	server.registerPlayback()
	server.registerPreferences()
	server.registerReport()
	server.registerHome()
	server.registerAccounts()
	server.registerDevices()
	server.registerWaitingCode()
	server.registerPerson()
	server.registerJobs()
	return server
}

// Handler is the HTTP handler to serve.
func (server *Server) Handler() http.Handler {
	return server.mux
}

// Mux is the router, for the composition root to mount debug endpoints beside the API.
func (server *Server) Mux() *http.ServeMux {
	return server.mux
}

// OpenAPI is the generated document (D85).
func (server *Server) OpenAPI() *huma.OpenAPI {
	return server.humaAPI.OpenAPI()
}

// SetStatus sets what /health reports.
func (server *Server) SetStatus(status Status, detail string, reasons ...string) {
	server.health.mutex.Lock()
	defer server.health.mutex.Unlock()
	server.health.status, server.health.detail, server.health.reasons = status, detail, reasons
}

func (server *Server) currentHealth() (Status, string, []string) {
	server.health.mutex.Lock()
	defer server.health.mutex.Unlock()
	return server.health.status, server.health.detail, append([]string{}, server.health.reasons...)
}

// Ready hands the API its dependencies and reports ready.
func (server *Server) Ready(dependencies Dependencies) {
	server.dependencies.Store(&dependencies)
	server.SetStatus(StatusReady, "")
}

// readyDependencies returns the dependencies, or the "migrating" error while there are none.
func (server *Server) readyDependencies() (*Dependencies, error) {
	dependencies := server.dependencies.Load()
	if dependencies == nil {
		status, detail, _ := server.currentHealth()
		return nil, newError(http.StatusServiceUnavailable, CodeMigrating,
			"the server is "+string(status)+" and will answer shortly; "+detail)
	}
	return dependencies, nil
}

// bearerToken takes the credential from an Authorization header. The credential travels in a
// header, never in a URL (D154).
func bearerToken(header string) string {
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return ""
	}
	return strings.TrimSpace(token)
}

// requireSignedIn authenticates a request's Authorization header and returns the dependencies
// and who signed in, or the error to answer with.
func (server *Server) requireSignedIn(
	ctx context.Context, authorization string,
) (*Dependencies, auth.Principal, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, auth.Principal{}, err
	}
	principal, err := dependencies.Auth.Authenticate(ctx, bearerToken(authorization))
	if err != nil {
		return nil, auth.Principal{}, server.authError(ctx, "authenticate", err)
	}
	return dependencies, principal, nil
}

// The reasons a request is not signed in, in auth_required's detail, so a client can tell the
// person why (cut-a §5.2): a session that was ended, one that lapsed, or none.
const (
	ReasonSessionEnded   = "session_ended"
	ReasonSessionExpired = "session_expired"
	ReasonNoSession      = "no_session"
)

// notSignedInError is auth_required with why.
func notSignedInError(err error) *Error {
	failure := newError(http.StatusUnauthorized, CodeAuthRequired, "sign in first")
	reason := ReasonNoSession
	switch {
	case errors.Is(err, auth.ErrSessionEnded):
		reason = ReasonSessionEnded
		failure.ErrorBody.Error.Message = "this device was signed out: removed, or signed out from another device"
	case errors.Is(err, auth.ErrSessionExpired):
		reason = ReasonSessionExpired
		failure.ErrorBody.Error.Message = "this device's session lapsed after going unused"
	}
	failure.ErrorBody.Error.Detail = map[string]string{"reason": reason}
	return failure
}

// authError maps the auth package's refusals to API errors. Every handler that calls auth answers
// through it, so a refusal always leaves with one code and one message.
func (server *Server) authError(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, auth.ErrNotAuthenticated):
		return notSignedInError(err)
	case errors.Is(err, auth.ErrBadCredentials):
		return newError(http.StatusUnauthorized, CodeAuthRequired, "the name or password is wrong")
	case errors.Is(err, auth.ErrBadInvitation):
		return newError(http.StatusUnauthorized, CodeAuthRequired, auth.ErrBadInvitation.Error())
	case errors.Is(err, auth.ErrRateLimited):
		return newError(http.StatusTooManyRequests, CodeRateLimited, "too many failed sign-ins; wait and try again")
	case errors.Is(err, auth.ErrWrongPassword):
		return newError(http.StatusForbidden, CodeForbidden, "the current password is wrong")
	case errors.Is(err, auth.ErrWrongSetupCode):
		return newError(http.StatusForbidden, CodeForbidden,
			"the setup code is wrong; it is in the server's log, or run `orpheum setup-code`")
	case errors.Is(err, auth.ErrSetupComplete):
		return newError(http.StatusConflict, CodeConflict, "setup is already complete; sign in instead")
	case errors.Is(err, auth.ErrNameTaken):
		return newError(http.StatusConflict, CodeConflict, "someone already has that name")
	case errors.Is(err, auth.ErrNoUser):
		return newError(http.StatusNotFound, CodeNotFound, "no such person")
	case errors.Is(err, auth.ErrNoDevice):
		return newError(http.StatusNotFound, CodeNotFound, "no such device")
	}
	return server.logInternalError(ctx, action, err)
}

// requireAdmin is requireSignedIn for an administrator.
func (server *Server) requireAdmin(ctx context.Context, authorization string) (
	*Dependencies, auth.Principal, error,
) {
	dependencies, principal, err := server.requireSignedIn(ctx, authorization)
	if err != nil {
		return nil, principal, err
	}
	if !principal.IsAdmin() {
		return nil, principal, newError(http.StatusForbidden, CodeForbidden, "only an administrator may do this")
	}
	return dependencies, principal, nil
}

// logInternalError logs an unexpected error and returns the generic internal error, so nothing
// about it reaches the client.
func (server *Server) logInternalError(ctx context.Context, action string, err error) error {
	server.logger.ErrorContext(ctx, "request failed", "action", action, "err", err)
	return newError(http.StatusInternalServerError, CodeInternal, "internal error")
}

// AuthorizationHeader is embedded by every signed-in operation's input.
type AuthorizationHeader struct {
	Authorization string `doc:"Bearer <session token>" header:"Authorization"`
}

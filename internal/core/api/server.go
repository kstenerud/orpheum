// SPDX-License-Identifier: AGPL-3.0-or-later

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
	"github.com/kstenerud/orpheum/internal/core/backup"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/portmap"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/reachability"
	"github.com/kstenerud/orpheum/internal/core/renditions"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/core/turn"
	"github.com/kstenerud/orpheum/internal/platform/clock"
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
	// Chunks are a playback's film cut into chunks at each quality, made on request (D247 §10).
	Chunks *chunks.Service
	// Subtitles lists the subtitles a person can fetch for a file (D224).
	Subtitles *subtitles.Service
	// Renditions says what each node's prepares made, for the System page (D225 §3).
	Renditions *renditions.Service
	// Together holds the Watch Together sessions, in memory (D119).
	Together *together.Hub
	// Reachability checks the server's public name and voice relay from outside in (D243 §7).
	Reachability *reachability.Checker
	// Discovery keeps the core enrolled with its discovery, which an admin connects and leaves (D273).
	Discovery *discovery.Service
	// PortMapping says what the home router opened for the discovery's addresses (D273 §7).
	PortMapping *portmap.Service
	// Relay hands Watch Together's members their voice relay credentials; nil serves none (D243 §8).
	Relay *turn.Issuer
	// Backups makes and lists the backups of the server's persistent directory (D242 §5).
	Backups *backup.Service
	// Preferences keeps a person's own choices, which hold in every file (D230).
	Preferences *preferences.Service
	// NodeBytes reads a file's bytes from its node.
	NodeBytes NodeBytes
	// Jobs is the one work queue, the one whose runners run the jobs, for the job view, cancel and
	// the destinations that are down.
	Jobs *jobs.Queue
	// Providers are each outside host's provider service, for the System page (D231 §5).
	Providers []*providers.Service
	// NodeReports is what each connected node said about itself, for the admin to see.
	NodeReports NodeReports
	// NodeWork is what the core saw of the work it gave each node, for the System page.
	NodeWork NodeWork
	// System is what the System page shows of the server itself (D225).
	System System
	// ConfigFileKeys is the config file's keys as the server runs with them, for /config to show.
	ConfigFileKeys []ConfigFileKey
	// Clock is the server's time: how long an answer is kept for a repeat of its Idempotency-Key.
	Clock clock.Clock
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
	// clientScript is the served web client's script, set before the server serves.
	clientScript string
	// rtc answers WebRTC offers; nil takes none (SetRTC).
	rtc *rtcapi.Server
	// answers are the answers kept for repeats of an Idempotency-Key (D244 §4).
	answers idempotentAnswers
	// togetherSilence is how long a Watch Together device may send nothing, in nanoseconds:
	// silenceLimit, which a test shortens.
	togetherSilence atomic.Int64
}

// NewServer builds the API and registers every operation. UseErrorShape must have been called.
func NewServer(logger *slog.Logger) *Server {
	config := huma.DefaultConfig("Orpheum", documentVersion)
	config.DocsPath = "" // the docs page loads scripts from a CDN; the document itself is served
	config.CreateHooks = nil
	config.Info.Description = "The Orpheum media server's client API (cut-a §5). Errors have one shape: " +
		"an object whose error holds a code, a message, a detail and, for a reachability error, the failedHop."
	mux := http.NewServeMux()
	server := &Server{mux: mux, logger: logger.With("component", "api")}
	server.togetherSilence.Store(int64(silenceLimit))
	config.Transformers = append(config.Transformers, server.logErrorAnswer)
	server.humaAPI = humago.NewWithPrefix(mux, Prefix, config)
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
	server.registerBackups()
	server.registerNeedsAttention()
	server.registerClientSettings()
	server.registerPlayback()
	server.registerPreferences()
	server.registerReport()
	server.registerLeave()
	server.registerHome()
	server.registerAccounts()
	server.registerInvitations()
	server.registerDevices()
	server.registerPairing()
	server.registerPerson()
	server.registerRoles()
	server.registerNowPlaying()
	server.registerTogether()
	server.registerReachability()
	server.registerDiscovery()
	server.registerRTC()
	mux.HandleFunc("GET "+togetherSocketPath, server.serveTogetherSocket)
	server.registerJobs()
	return server
}

// Handler is the HTTP handler to serve: the API, a POST with an Idempotency-Key answered once,
// every answer naming the web client this server serves (ClientScriptHeader).
func (server *Server) Handler() http.Handler {
	return server.namingClient(server.idempotent(server.pacedMedia(server.mux)))
}

// ClientScriptHeader names, on every answer, the script of the web client this server serves: a
// page running another, older one, left open across an update, reloads itself when it can (the
// owner, 2026-10-08: "Maybe the client should send its version and then force a reload if there's
// a mismatch?"; a host's tab kept running a fixed bug). Absent while the server serves no client.
const ClientScriptHeader = "X-Orpheum-Client"

// SetClientScript is the name of the web client's script the server serves (webclient.ScriptName).
func (server *Server) SetClientScript(name string) {
	server.clientScript = name
}

// namingClient names the served client's script on every answer next serves.
func (server *Server) namingClient(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if server.clientScript != "" {
			writer.Header().Set(ClientScriptHeader, server.clientScript)
		}
		next.ServeHTTP(writer, request)
	})
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
	reason, message := ReasonNoSession, "sign in first"
	switch {
	case errors.Is(err, auth.ErrSessionEnded):
		reason, message = ReasonSessionEnded, "this device was signed out: removed, or signed out from another device"
	case errors.Is(err, auth.ErrSessionExpired):
		reason, message = ReasonSessionExpired, "this device's session lapsed after going unused"
	}
	return newDetailedError(http.StatusUnauthorized, message, SignedOutDetailBody{Reason: reason})
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
	case errors.Is(err, auth.ErrRemoveSelf):
		return newError(http.StatusConflict, CodeConflict, auth.ErrRemoveSelf.Error())
	case errors.Is(err, auth.ErrNoDevice):
		return newError(http.StatusNotFound, CodeNotFound, "no such device")
	}
	if refused := roleRefusal(err); refused != nil {
		return refused
	}
	if refused := pairingRefusal(err); refused != nil {
		return refused
	}
	return server.logInternalError(ctx, action, err)
}

// roleRefusal is the API's error for auth's refusal of a change to a role, or nil for another.
func roleRefusal(err error) *Error {
	switch {
	case errors.Is(err, auth.ErrNoNote):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.adminNote", auth.ErrNoNote.Error())
	case errors.Is(err, auth.ErrNoInvitation):
		return newError(http.StatusNotFound, CodeNotFound, auth.ErrNoInvitation.Error())
	case errors.Is(err, auth.ErrNoRole):
		return newError(http.StatusNotFound, CodeNotFound, "no such role")
	case errors.Is(err, auth.ErrRoleNameTaken):
		return newError(http.StatusConflict, CodeConflict, "another role already has that name")
	case errors.Is(err, auth.ErrBadRole):
		return invalidRequestAt(http.StatusBadRequest, "body", err.Error())
	case errors.Is(err, auth.ErrNoLibraryToGive):
		return invalidRequestAt(http.StatusBadRequest, "body.libraries", err.Error())
	case errors.Is(err, auth.ErrBuiltinRole):
		return newError(http.StatusConflict, CodeConflict, auth.ErrBuiltinRole.Error())
	case errors.Is(err, auth.ErrRoleHeld):
		return newError(http.StatusConflict, CodeConflict, auth.ErrRoleHeld.Error())
	}
	return nil
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

// requireAccountManager is requireSignedIn for a session that may change the account's security: its
// password, its devices, and the codes it enters for others. A device a code activated may not
// (D292 §4; the security audit's A11).
func (server *Server) requireAccountManager(ctx context.Context, authorization string) (
	*Dependencies, auth.Principal, error,
) {
	dependencies, principal, err := server.requireSignedIn(ctx, authorization)
	if err != nil {
		return nil, principal, err
	}
	if !principal.CanManageAccount() {
		return nil, principal, newError(http.StatusForbidden, CodeForbidden,
			"this device was activated by a code and may not change the account's security; do it on a "+
				"device signed in with the password")
	}
	return dependencies, principal, nil
}

// logInternalError logs an unexpected error and returns the generic internal error, so nothing
// about it reaches the client.
func (server *Server) logInternalError(ctx context.Context, action string, err error) error {
	if ctx.Err() != nil {
		// The client went away, as a player does from a segment it no longer wants: no one reads
		// the answer, and nothing failed but the asking.
		server.logger.DebugContext(ctx, "a request's client went away", "action", action, "err", err)
	} else {
		server.logger.ErrorContext(ctx, "request failed", "action", action, "err", err)
	}
	return newError(http.StatusInternalServerError, CodeInternal, "internal error")
}

// AuthorizationHeader is embedded by every signed-in operation's input.
type AuthorizationHeader struct {
	Authorization string `doc:"Bearer <session token>" header:"Authorization"`
}

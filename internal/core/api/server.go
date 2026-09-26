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
	// Identify is what a person does about identification: the chooser and overrides.
	Identify *identify.Chooser
	// Refresh queues a person's refresh of a title or series (D213).
	Refresh *identify.Refresher
	// Libraries is what an admin does with libraries: create, list, scan, and the scanner view.
	Libraries *library.Admin
	// Browse is what a person browses and searches.
	Browse *library.Browser
	// Artwork serves the images titles show.
	Artwork *artwork.Service
	// Playback decides how a file plays and says where its bytes are.
	Playback *playback.Service
	// NodeBytes reads a file's bytes from its node.
	NodeBytes NodeBytes
	// ConfigFile is the config file's keys as the server runs with them, for /config to show.
	ConfigFile []ConfigFileKey
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
	api          huma.API
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
		"{\"error\": {\"code\", \"message\", \"detail\", \"hop\"}}."
	mux := http.NewServeMux()
	server := &Server{
		api:    humago.NewWithPrefix(mux, Prefix, config),
		mux:    mux,
		logger: logger.With("component", "api"),
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
	server.registerPlayback()
	server.registerFilePrefs()
	server.registerReport()
	server.registerHome()
	server.registerAccounts()
	server.registerDevices()
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
	return server.api.OpenAPI()
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

// ready returns the dependencies, or the "migrating" error while there are none.
func (server *Server) ready() (*Dependencies, error) {
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

// signedIn authenticates a request's Authorization header.
func (server *Server) signedIn(ctx context.Context, authorization string) (*Dependencies, auth.Principal, error) {
	dependencies, err := server.ready()
	if err != nil {
		return nil, auth.Principal{}, err
	}
	principal, err := dependencies.Auth.Authenticate(ctx, bearerToken(authorization))
	if errors.Is(err, auth.ErrNotAuthenticated) {
		return nil, auth.Principal{}, newError(http.StatusUnauthorized, CodeAuthRequired, "sign in first")
	}
	if err != nil {
		return nil, auth.Principal{}, server.internal(ctx, "authenticate", err)
	}
	return dependencies, principal, nil
}

// signedInAdmin is signedIn for an administrator.
func (server *Server) signedInAdmin(ctx context.Context, authorization string) (
	*Dependencies, auth.Principal, error,
) {
	dependencies, principal, err := server.signedIn(ctx, authorization)
	if err != nil {
		return nil, principal, err
	}
	if !principal.IsAdmin() {
		return nil, principal, newError(http.StatusForbidden, CodeForbidden, "only an administrator may do this")
	}
	return dependencies, principal, nil
}

// internal logs an unexpected error and returns the generic internal error, so nothing about it
// reaches the client.
func (server *Server) internal(ctx context.Context, doing string, err error) error {
	server.logger.ErrorContext(ctx, "request failed", "doing", doing, "err", err)
	return newError(http.StatusInternalServerError, CodeInternal, "internal error")
}

// AuthorizationHeader is embedded by every signed-in operation's input.
type AuthorizationHeader struct {
	Authorization string `doc:"Bearer <session token>" header:"Authorization"`
}

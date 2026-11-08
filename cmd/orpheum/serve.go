// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/assembly"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/portmap"
	"github.com/kstenerud/orpheum/internal/core/reachability"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/core/webclient"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/localtime"
	"github.com/kstenerud/orpheum/internal/platform/pacing"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
)

// logEvent is the attribute key a few of serve's log lines carry alongside their message: a stable
// name a test waits on, so rewording the message does not break the wait (test architecture review,
// 2026-10-26, finding 12). Only the lines a test waits on by name carry it; most lines need none.
const logEvent = "logEvent"

// eventReady and eventCouldNotStart are logEvent's values for serve's two lines a test most
// commonly waits for: that it finished starting, and that it could not.
const (
	eventReady         = "ready"
	eventCouldNotStart = "couldNotStart"
)

// serve runs the core on listener until ctx ends. The API answers /health before the database is
// even open, and says "migrating" while it migrates. A migration that fails leaves the server up
// and degraded, naming the step, the error and the snapshot, rather than exiting into a restart
// loop (migrations.md, When a step fails). Once the database is ready it runs the background jobs
// (fetch, artwork, prepare, subtitle and read-through) and their loops, and serves the node channel,
// which each node, a service of its own, dials.
func serve(
	ctx context.Context, config Config, configReport ConfigReport, listener net.Listener, outside wiring,
	logger *slog.Logger,
) (err error) {
	server := newAPIServer(config, logger)
	clientFiles, hasClient := webclient.Files()
	if clientErr := serveClient(server, clientFiles, hasClient); clientErr != nil {
		return clientErr
	}
	// Requests over WebRTC data channels reach the same handler as those over TCP (D273 §8).
	rtc := webRTCOf(ctx, server, config, logger)
	defer func() { err = errors.Join(err, rtc.Close()) }()
	httpServer := &http.Server{
		Handler:           server.LANHandler(lanHostNames(config)),
		ReadHeaderTimeout: config.readHeaderTimeout(), ConnContext: pacing.ConnContext,
	}
	var serving sync.WaitGroup
	serving.Go(func() {
		if serveErr := httpServer.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the HTTP server stopped", "err", serveErr)
		}
	})
	// The front doors, HTTP and once started HTTPS, close first when ctx ends, on every path: a
	// request still in flight is finished while the databases, the jobs and the nodes it may use
	// are all still there, and none arrives after them. Deferred, they would close last, after
	// the databases, and a request then answered "internal error" (2026-10-06).
	stopHTTPS := func() error { return nil }
	stopFrontDoors := onlyOnce(func() error {
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.shutdownGrace())
		defer cancel()
		shutdownErr := errors.Join(stopHTTPS(), httpServer.Shutdown(shutdownContext))
		serving.Wait()
		return shutdownErr
	})
	defer func() { err = errors.Join(err, stopFrontDoors()) }()
	degraded := func(what string, cause error) error {
		return errors.Join(stayDegraded(ctx, server, what, cause, logger), stopFrontDoors())
	}
	logger.InfoContext(ctx, "listening", "address", listener.Addr().String())

	opened, err := openAndWire(ctx, config, configReport, server, outside, logger)
	if err != nil {
		return degraded("the server could not start", err)
	}
	defer func() { err = errors.Join(err, opened.release()) }()
	startedHTTPS, err := listenAndServeHTTPS(ctx, config, opened, httpServer.Handler, logger)
	if err != nil {
		return degraded("HTTPS could not start", err)
	}
	stopHTTPS = startedHTTPS
	stopRelay, err := startRelay(ctx, config, opened, logger)
	if err != nil {
		return degraded("the voice relay could not start", err)
	}
	defer func() { err = errors.Join(err, stopRelay()) }()
	stopBackgroundJobs, err := startBackgroundLoops(ctx, opened, logger)
	if err != nil {
		return degraded("the background jobs could not start", err)
	}
	defer stopBackgroundJobs()
	stopNodes, err := runNodeChannel(ctx, config, opened, logger)
	if err != nil {
		return degraded("the node channel could not start", err)
	}
	defer func() { err = errors.Join(err, stopNodes()) }()
	logger.InfoContext(ctx, "ready", logEvent, eventReady)
	<-ctx.Done()
	logger.InfoContext(ctx, "stopping")
	return stopFrontDoors()
}

// onlyOnce is stop, run by the first call alone; a later call does nothing and returns nil, so a
// stop called on its path and again deferred reports its error once.
func onlyOnce(stop func() error) func() error {
	stopped := false
	return func() error {
		if stopped {
			return nil
		}
		stopped = true
		return stop()
	}
}

// stayDegraded keeps the server up, saying at /api/v1/health why it could not start, until ctx ends.
func stayDegraded(ctx context.Context, server *api.Server, what string, err error, logger *slog.Logger) error {
	server.SetStatus(api.StatusDegraded, what, err.Error())
	logger.ErrorContext(ctx, "the server could not start; it stays up to report why at /api/v1/health",
		logEvent, eventCouldNotStart, "what", what, "err", err)
	<-ctx.Done()
	return err
}

// wiring is what serve is given from outside: where TMDB answers and serves its images
// (tmdb.BaseURL and tmdb.ImageBaseURL, or a test's fake).
type wiring struct {
	tmdbBaseURL       string
	tmdbImagesBaseURL string
	// openSubtitlesBaseURL is where OpenSubtitles answers (opensubtitles.BaseURL, or a test's fake).
	openSubtitlesBaseURL string
	// cloudflareBaseURL is the DNS provider's API (cloudflare.BaseURL, or a fake).
	cloudflareBaseURL string
	// waitsForDNS is whether a challenge's record is given certificate_dns_wait_seconds to reach the
	// name's servers before the CA looks (D243 §6): a test's fake DNS needs none.
	waitsForDNS bool
	// machineZone is the machine's own time zone, the server's until the admin chooses one
	// (D246 §7); a test's is UTC.
	machineZone localtime.Zone
	// logLevel is the level the log is written at, which the log_level setting changes while the core
	// runs (D347); nil, a test's, is left as it is.
	logLevel *slog.LevelVar
	// setupCodeFile is where the setup code is also written while no admin exists, for a person who
	// looks in the data folder: a compose file's start names its deployment's (D449 §3); empty writes
	// none.
	setupCodeFile string
}

// openedServices is what openAndWire opened and serve must close, and what serve runs: the core's
// services as internal/core/assembly assembles them (D351), and what the config alone makes beside
// them.
type openedServices struct {
	lock        *instancedir.Lock
	directories *instancedir.Directories
	database    *store.Database
	cache       *store.Cache
	settings    *settings.Service
	secrets     *secrets.Store
	// core is the core's services, assembled.
	core *assembly.Services
	// queue is the one job queue, which the certificates' renewals are queued on.
	queue *jobs.Queue
	// outboundClient is the one client every call to another host goes through.
	outboundClient *http.Client
	// together is Watch Together's sessions, whose memberships the voice relay admits, and whose links
	// and devices' locators a discovery finds the core by.
	together *together.Hub
	// auth keeps what a discovery finds the core by: its links and its devices' locators.
	auth *auth.Service
	// reachability checks the public path, and answers its echo for the admin's checks and for the
	// discovery's outside probe, whose nonces the heartbeat sends.
	reachability *reachability.Checker
	// certificates obtains and renews the HTTPS certificates: the server's public name's (D243 §6)
	// and its discovery's (D273).
	certificates *servedCertificates
	// discovery keeps the core enrolled with its discovery, when an admin connected one (D273).
	discovery *discovery.Service
	// portMapping keeps the core's HTTPS port open at the home router while the discovery publishes
	// it (D273 §7).
	portMapping *portmap.Service
}

// with is opened holding the core's services, and those of them that what the config makes uses.
func (opened openedServices) with(core *assembly.Services) openedServices {
	opened.core, opened.queue, opened.outboundClient = core, core.Queue, core.OutboundClient
	opened.secrets, opened.auth, opened.together = core.API.Secrets, core.API.Auth, core.API.Together
	return opened
}

func (opened openedServices) release() error {
	return errors.Join(opened.cache.Close(), opened.database.Close(), opened.lock.Release())
}

// openAndWire opens the instance's directories and the database, migrates, has the core's services
// assembled over them, and hands the API its dependencies.
func openAndWire(
	ctx context.Context, config Config, configReport ConfigReport, server *api.Server, outside wiring,
	logger *slog.Logger,
) (opened openedServices, err error) {
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		return opened, err
	}
	opened.lock, err = directories.LockInstance()
	if err != nil {
		return opened, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, opened.lock.Release())
		}
	}()
	database, err := openAndMigrateOrStartOver(ctx, config, directories, clock.System{}, server.SetStatus, logger)
	if err != nil {
		return opened, err
	}
	opened.database, opened.directories = database, directories
	opened.cache, err = openCache(ctx, config, directories, logger)
	if err != nil {
		return opened, errors.Join(err, database.Close())
	}
	opened, dependencies, err := wireCore(ctx, config, server, outside, opened, logger)
	if err == nil {
		dependencies.ConfigFileKeys, err = configFileKeys(config, configReport)
	}
	if err != nil {
		return opened, errors.Join(err, opened.cache.Close(), database.Close())
	}
	readyToServe(ctx, server, dependencies, logger)
	return opened, nil
}

// wireCore seeds the settings, loads the secret key, has the core's services assembled (D351), makes
// sure a server with no users has a setup code, and makes what the config alone makes beside them:
// the debug endpoints, the certificates, the discovery, port mapping, the reachability check and the
// relay's credentials. It returns the API's dependencies, whole but for the config file's keys.
func wireCore(
	ctx context.Context, config Config, server *api.Server, outside wiring, opened openedServices,
	logger *slog.Logger,
) (openedServices, api.Dependencies, error) {
	var err error
	if opened.settings, err = seededSettings(ctx, opened.database, outside.machineZone, logger); err != nil {
		return opened, api.Dependencies{}, err
	}
	secretKey, err := loadOrCreateSecretKey(opened.directories, logger)
	if err != nil {
		return opened, api.Dependencies{}, err
	}
	if diagnosticsErr := mountDiagnostics(
		ctx, server.Mux(), config, outside.logLevel, opened.settings, logger,
	); diagnosticsErr != nil {
		return opened, api.Dependencies{}, diagnosticsErr
	}
	core, err := assembly.Build(ctx, assembly.Parts{
		Database: opened.database, Cache: opened.cache, Directories: opened.directories, Settings: opened.settings,
		SecretKey:  secretKey,
		SetupCodes: runSetupCodes{directories: opened.directories, shownFile: outside.setupCodeFile},
		Clock:      config.servicesClock(), Random: rand.Reader, Logger: logger, Build: thisBuild(),
		TMDBBaseURL: outside.tmdbBaseURL, TMDBImagesBaseURL: outside.tmdbImagesBaseURL,
		OpenSubtitlesBaseURL: outside.openSubtitlesBaseURL,
	})
	if err != nil {
		return opened, api.Dependencies{}, err
	}
	opened = opened.with(core)
	codes := runSetupCodes{directories: opened.directories, shownFile: outside.setupCodeFile}
	if setupErr := ensureSetupCode(ctx, core.API.Auth, codes, logger); setupErr != nil {
		return opened, api.Dependencies{}, setupErr
	}
	if opened, err = openReachedFromOutside(ctx, config, opened, outside, server.RTC(), logger); err != nil {
		return opened, api.Dependencies{}, err
	}
	core.Certificates = opened.certificates
	dependencies := core.API
	dependencies.Discovery, dependencies.PortMapping = opened.discovery, opened.portMapping
	// What the discovery's release record says of this version, for the admins (security-updates.md).
	dependencies.Releases, dependencies.Version = opened.discovery, thisBuild().Version
	dependencies.PublishedImage = publishedImage
	dependencies.Reachability, dependencies.Relay = opened.reachability, newICEIssuer(config, opened, logger)
	dependencies.System = api.System{
		Build: thisBuild(), StartedAt: clock.System{}.Now(),
		PersistentDirectory: opened.directories.PersistentDirectory(),
		CacheDirectory:      opened.directories.CacheDirectory(), PersistentDatabase: opened.database,
		CacheDatabase: opened.cache, PublicName: config.HTTPS.Domain,
		Certificate: certificateExpiry(opened.certificates),
	}
	return opened, dependencies, nil
}

// readyToServe hands the API server its dependencies, which it answers with from now on, and keeps
// its WebRTC server's share of connections per network as its setting says.
func readyToServe(ctx context.Context, server *api.Server, dependencies api.Dependencies, logger *slog.Logger) {
	server.Ready(dependencies)
	if rtc := server.RTC(); rtc != nil {
		keepWebRTCShareCurrent(ctx, rtc, dependencies.Settings, logger)
	}
}

// connectionShare is what takes a network's share of WebRTC connections: the WebRTC server.
type connectionShare interface {
	SetConnectionsPerNetwork(limit int)
}

// openAndMigrateOrStartOver is openAndMigrate, and with database.when_unmigratable = "start_over"
// a persistent.db that cannot be brought forward (below the baseline, newer than this binary can
// use, or a step that differs from the one applied) is set aside in the backups directory, named
// by now, and an empty one is made in its place (D249 §4). Each step is reported through
// setStatus, when it is not nil (reportMigration).
func openAndMigrateOrStartOver(
	ctx context.Context, config Config, directories *instancedir.Directories, now clock.Clock,
	setStatus func(api.Status, string, ...string), logger *slog.Logger,
) (*store.Database, error) {
	config.slowWrites, config.slowWriteClock = loggingSlowWrites(ctx, logger), now
	config.readerWrites = loggingReaderWrites(ctx, logger)
	var beforeStep func(step, total int)
	if setStatus != nil {
		beforeStep = reportMigration(setStatus, logger)
	}
	database, err := openAndMigrate(ctx, config, directories, beforeStep)
	if err == nil || config.Database.WhenUnmigratable != whenStartOver || !isUnmigratable(err) {
		return database, err
	}
	asideName := "persistent-set-aside-" + now.Now().UTC().Format("20060102T150405Z") + ".db"
	aside, moveErr := directories.SetPersistentDatabaseAside(asideName)
	if moveErr != nil {
		return nil, errors.Join(err, moveErr)
	}
	logger.WarnContext(ctx, "the database could not be brought forward; it is set aside and the server starts "+
		"with an empty one, as database.when_unmigratable says (D249 §4)", "setAsideAt", aside, "reason", err.Error())
	return openAndMigrate(ctx, config, directories, beforeStep)
}

// isUnmigratable is whether err is a database this binary's steps cannot bring forward, as opposed
// to a step that failed or a disk that could not be read.
func isUnmigratable(err error) bool {
	var newer *store.NewerDatabaseError
	return errors.Is(err, store.ErrBeforeBaseline) || errors.Is(err, store.ErrStepChanged) || errors.As(err, &newer)
}

// openAndMigrate opens persistent.db and migrates it, calling beforeStep, when it is not nil, before
// each step starts. The migrate command and the dev seed open the database this way; serving does
// through openAndMigrateOrStartOver.
func openAndMigrate(
	ctx context.Context, config Config, directories *instancedir.Directories, beforeStep func(step, total int),
) (*store.Database, error) {
	database, err := store.Open(ctx, directories.PersistentDatabasePath(), config.openOptions())
	if err != nil {
		return nil, err
	}
	err = database.Migrate(ctx, config.migrateOptions(directories, beforeStep))
	if err != nil {
		return nil, errors.Join(err, database.Close())
	}
	return database, nil
}

// openCache opens cache.db, and discards and rebuilds one from another schema version: it is
// versioned by discard, and losing it loses nothing (schema.md, cache.db).
func openCache(
	ctx context.Context, config Config, directories *instancedir.Directories, logger *slog.Logger,
) (*store.Cache, error) {
	config.slowWrites, config.readerWrites = loggingSlowWrites(ctx, logger), loggingReaderWrites(ctx, logger)
	cache, err := store.OpenCache(ctx, directories.CacheDatabasePath(), config.openOptions())
	if !errors.Is(err, store.ErrCacheOutdated) {
		return cache, err
	}
	logger.InfoContext(ctx, "discarding cache.db: it was built by another version", "reason", err.Error())
	if err := directories.DiscardCacheDatabase(); err != nil {
		return nil, err
	}
	return store.OpenCache(ctx, directories.CacheDatabasePath(), config.openOptions())
}

// reportMigration returns the runner's BeforeStep: /health says "migrating: step 3 of 7" before
// each step starts (migrations.md, While migrating: never look dead).
func reportMigration(setStatus func(api.Status, string, ...string), logger *slog.Logger) func(step, total int) {
	return func(step, total int) {
		detail := fmt.Sprintf("step %d of %d", step, total)
		setStatus(api.StatusMigrating, detail)
		logger.Info("migrating", "step", step, "stepCount", total)
	}
}

// seededSettings is the server's settings, the machine's time zone its own until an admin chooses one
// (D246 §7), with any setting that has no row yet written before anything reads one (D201).
func seededSettings(
	ctx context.Context, database *store.Database, machineZone localtime.Zone, logger *slog.Logger,
) (*settings.Service, error) {
	settingsService := settings.NewService(database, clock.System{}, logger)
	settingsService.UseMachineZone(machineZone)
	logger.InfoContext(ctx, "the machine's time zone, the server's until one is chosen", "timeZone", machineZone.Name())
	seedReport, err := settingsService.Seed(ctx)
	if err != nil {
		return nil, err
	}
	if len(seedReport.Inserted) > 0 {
		logger.InfoContext(ctx, "wrote the starting value of settings that had none",
			slogx.EventKey, "settingsSeeded",
			"keys", seedReport.Inserted)
	}
	if len(seedReport.Updated) > 0 {
		logger.InfoContext(ctx, "this release changed the starting value of settings nobody had set; they follow it",
			slogx.EventKey, "settingsStartsFollowed",
			"keys", seedReport.Updated)
	}
	logUnusedSettings(ctx, seedReport.Unused, logger)
	return settingsService, nil
}

// logUnusedSettings logs each setting row this release does not read, left for a person to delete
// (D202). A key a release retired, which says when and why, is expected and logged as information; a
// key no release used, most likely a typo, is a warning (the production NAS warned of seven retired keys at every
// start, 2026-11-01).
func logUnusedSettings(ctx context.Context, unused []settings.UnusedSetting, logger *slog.Logger) {
	for _, setting := range unused {
		if setting.Retired != nil {
			logger.InfoContext(ctx, "a setting row a release retired; left alone, and doctor lists it",
				"key", setting.String(), "retiredIn", setting.Retired.Release, "reason", setting.Retired.Reason)
			continue
		}
		logger.WarnContext(ctx, "a setting row no release reads; left alone, and doctor lists it",
			"key", setting.String())
	}
}

// ensureSetupCode makes sure a server with no users has a setup code, which it logs at every start
// until the first admin is made, saying where it goes: the log is all a NAS's panel shows of a server
// installed from a compose file (D447), with no terminal to run `orpheum setup-code` at. It also
// writes the code to the data folder's file, when there is one, where a person browsing it looks
// (D449 §3).
func ensureSetupCode(
	ctx context.Context, authService *auth.Service, codes runSetupCodes, logger *slog.Logger,
) error {
	code, err := authService.EnsureSetupCode(ctx, codes)
	if err != nil {
		return err
	}
	if code == "" {
		return nil
	}
	if showErr := codes.show(code); showErr != nil {
		return showErr
	}
	logger.WarnContext(ctx, "no one can sign in yet: open Orpheum in a browser at home, "+
		"http://<this machine's address>:<its HTTP port>, and make the first admin there with this one-time "+
		"setup code (POST /api/v1/setup); `orpheum setup-code` prints it again", "setupCode", code)
	return nil
}

// loadOrCreateSecretKey reads secret.key, or writes a new one on first start. A key replaced or
// lost leaves every sealed secret "needs re-entering" and nothing else lost (D194 §4).
func loadOrCreateSecretKey(directories *instancedir.Directories, logger *slog.Logger) (secrets.Key, error) {
	contents, err := directories.ReadPersistentFile(secrets.KeyFileName)
	if err == nil {
		return secrets.ParseKeyFile(contents)
	}
	if !errors.Is(err, instancedir.ErrNotFound) {
		return secrets.Key{}, err
	}
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		return secrets.Key{}, err
	}
	err = directories.WritePersistentFile(secrets.KeyFileName, key.EncodeKeyFile())
	if err != nil {
		return secrets.Key{}, err
	}
	logger.Info("wrote a new secret.key; back it up with `orpheum backup --include-key` to move machines")
	return key, nil
}

// runSetupCodes keeps the setup code in the instance's run directory, mode 0600 (D242 §3), and, when
// shownFile is named, shows it there too while no admin exists, for a person who looks in the data
// folder rather than the log (D449 §3).
type runSetupCodes struct {
	directories *instancedir.Directories
	shownFile   string
}

func (codes runSetupCodes) Read() (string, error) {
	contents, err := codes.directories.ReadRunFile(auth.SetupCodeFileName)
	if errors.Is(err, instancedir.ErrNotFound) {
		return "", auth.ErrNoSetupCode
	}
	return string(contents), err
}

func (codes runSetupCodes) Write(code string) error {
	return codes.directories.WriteRunFile(auth.SetupCodeFileName, []byte(code))
}

// Remove removes the code, and the file showing it, once the first admin is made.
func (codes runSetupCodes) Remove() error {
	if err := codes.directories.RemoveRunFile(auth.SetupCodeFileName); err != nil {
		return err
	}
	if codes.shownFile == "" {
		return nil
	}
	return instancedir.RemoveFileBeside(codes.shownFile)
}

// show writes code to shownFile, readable by the server's user alone, as its run-directory copy is,
// with a line saying where it goes; with no shownFile it writes nothing.
func (codes runSetupCodes) show(code string) error {
	if codes.shownFile == "" {
		return nil
	}
	text := code + "\n\nOrpheum's one-time setup code. Open Orpheum in a browser at home, " +
		"http://<this machine's address>:8440, and enter it on the first page to make the first admin; " +
		"this file is removed once you have.\n"
	return instancedir.WriteFileBeside(codes.shownFile, []byte(text))
}

// mountDebug serves pprof on the API's mux when the should_serve_debug_pprof setting says so, and only on a
// loopback listener (cut-a §2a, D91). It reads the settings once, at start. Metrics are a setting
// too and not built yet; the log says so.
func mountDebug(
	ctx context.Context, mux *http.ServeMux, config Config, settingsService *settings.Service, logger *slog.Logger,
) error {
	shouldServePprof, err := settingsService.Bool(ctx, settings.ShouldServeDebugPProf)
	if err != nil {
		return err
	}
	shouldServeMetrics, err := settingsService.Bool(ctx, settings.ShouldServeDebugMetrics)
	if err != nil {
		return err
	}
	isLoopbackListen := isLoopback(config.Server.Listen)
	if shouldServePprof && isLoopbackListen {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		logger.InfoContext(ctx, "pprof is on", "path", "/debug/pprof/")
	}
	if (shouldServePprof || shouldServeMetrics) && !isLoopbackListen {
		logger.WarnContext(ctx, "debug endpoints are served only on a loopback listen address; they are off",
			"listen", config.Server.Listen)
	}
	if shouldServeMetrics && isLoopbackListen {
		logger.WarnContext(ctx,
			"should_serve_debug_metrics is set, but /metrics is not built yet; nothing is served there")
	}
	return nil
}

// newAPIServer is the API's server, an admin administering from the networks the config names
// (D297 §6).
func newAPIServer(config Config, logger *slog.Logger) *api.Server {
	server := api.NewServer(logger)
	server.SetHomeNetworks(config.Server.homeNetworks())
	server.SetThisMachinePort(config.Server.thisMachinePort())
	return server
}

// serveClient serves the built web client, clientFiles, at "/", and has every API answer name its
// script, so a page left open across an update reloads itself (api.ClientScriptHeader), and its
// manifest signed, which a discovery's loader checks before it runs it (D308 §6); a binary built
// without the client, hasClient false, serves the placeholder and names none.
func serveClient(server *api.Server, clientFiles fs.FS, hasClient bool) error {
	if hasClient {
		server.SetClientScript(webclient.ScriptName(clientFiles))
		manifest, err := webclient.ManifestOf(clientFiles, thisBuild().Version)
		if err != nil {
			return err
		}
		server.SetClientManifest(manifest)
	}
	server.Mux().Handle("/", webclient.Handler(clientFiles, api.Prefix))
	return nil
}

// webRTCOf is the WebRTC server over server's handler, taking WebRTC at home as config says, which
// server serves offers to.
func webRTCOf(ctx context.Context, server *api.Server, config Config, logger *slog.Logger) *rtcapi.Server {
	rtc := newWebRTC(ctx, server.Handler(), config.WebRTC, logger)
	listenForWebRTCAtHome(ctx, rtc, config, logger)
	server.SetRTC(rtc)
	return rtc
}

// keepWebRTCShareCurrent gives the WebRTC server its network's share of connections from its setting
// (webrtc_connections_per_network, the owner, 2026-10-25), now and at each change; one that cannot
// be read leaves the share as it was, and says so.
func keepWebRTCShareCurrent(
	ctx context.Context, rtc connectionShare, settingsService *settings.Service, logger *slog.Logger,
) {
	if settingsService == nil {
		return
	}
	apply := func(ctx context.Context) error {
		limit, err := settingsService.Int(ctx, settings.WebRTCConnectionsPerNetwork)
		if err != nil {
			return fmt.Errorf("read the WebRTC connections a network may hold: %w", err)
		}
		rtc.SetConnectionsPerNetwork(int(limit))
		return nil
	}
	if err := apply(ctx); err != nil {
		logger.ErrorContext(ctx, "the WebRTC connections a network may hold stay as they were", "err", err)
	}
	settingsService.OnChange(settings.WebRTCConnectionsPerNetwork, apply)
}

// listenForWebRTCAtHome names the house's networks to the WebRTC server and, when the config names a
// UDP port, has it take WebRTC there at the house's addresses (D300 §1); a port it cannot have is
// logged, and WebRTC goes on without it.
func listenForWebRTCAtHome(ctx context.Context, rtc *rtcapi.Server, config Config, logger *slog.Logger) {
	rtc.SetHomeNetworks(config.Server.homeNetworks())
	if config.WebRTC.UDPPort == 0 {
		return
	}
	lanIPs := config.Discovery.lanIPs()
	if err := rtc.ListenLAN(config.WebRTC.UDPPort, lanIPs); err != nil {
		logger.ErrorContext(ctx, "WebRTC is not taken at home", "err", err)
		return
	}
	logger.InfoContext(ctx, "taking WebRTC at home", "udpPort", config.WebRTC.UDPPort, "addresses", lanIPs)
}

// newWebRTC is the WebRTC server over the API's handler, as this build serves it, holding as many
// connections as config allows (D348): a development build lets a discovery's offers reach the
// machine's own addresses, a release never (relaxForDevelopment).
func newWebRTC(ctx context.Context, handler http.Handler, config WebRTCConfig, logger *slog.Logger) *rtcapi.Server {
	rtc := rtcapi.New(ctx, handler, logger)
	rtc.SetMaxConnections(config.MaxConnections)
	return relaxForDevelopment(rtc)
}

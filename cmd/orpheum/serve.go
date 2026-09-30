// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"path/filepath"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/webclient"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// serve runs the core on listener until ctx ends. The API answers /health before the database is
// even open, and says "migrating" while it migrates. A migration that fails leaves the server up
// and degraded, naming the step, the error and the snapshot, rather than exiting into a restart
// loop (migrations.md, When a step fails). Once the database is ready it runs the background jobs
// (fetch, artwork, prepare, subtitle and loudness) and their loops, serves the node channel, and
// supervises the node child when the wiring can start one.
func serve(
	ctx context.Context, config Config, configReport ConfigReport, listener net.Listener, outside wiring,
	logger *slog.Logger,
) (err error) {
	server := api.NewServer(logger)
	clientFiles, _ := webclient.Files()
	server.Mux().Handle("/", webclient.Handler(clientFiles, api.Prefix))
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: config.readHeaderTimeout()}
	var serving sync.WaitGroup
	serving.Go(func() {
		if serveErr := httpServer.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the HTTP server stopped", "err", serveErr)
		}
	})
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.shutdownGrace())
		defer cancel()
		err = errors.Join(err, httpServer.Shutdown(shutdownContext))
		serving.Wait()
	}()
	logger.InfoContext(ctx, "listening", "address", listener.Addr().String())

	opened, err := openAndWire(ctx, config, configReport, server, outside, logger)
	if err != nil {
		server.SetStatus(api.StatusDegraded, "the server could not start", err.Error())
		logger.ErrorContext(ctx, "the server could not start; it stays up to report why at /api/v1/health",
			"err", err)
		<-ctx.Done()
		return err
	}
	defer func() { err = errors.Join(err, opened.release()) }()
	stopBackgroundJobs := opened.jobServices.startBackgroundLoops(ctx, logger)
	defer stopBackgroundJobs()
	stopNodes, err := runNodeChannel(ctx, config, opened, outside.startNode, logger)
	if err != nil {
		server.SetStatus(api.StatusDegraded, "the node channel could not start", err.Error())
		logger.ErrorContext(ctx, "the node channel could not start; the server stays up to report why", "err", err)
		<-ctx.Done()
		return err
	}
	defer func() { err = errors.Join(err, stopNodes()) }()
	logger.InfoContext(ctx, "ready")
	<-ctx.Done()
	logger.InfoContext(ctx, "stopping")
	return nil
}

// wiring is what serve is given from outside: how to start the node child (nil in tests that
// run none), and where TMDB answers and serves its images (tmdb.BaseURL and tmdb.ImageBaseURL,
// or a test's fake).
type wiring struct {
	startNode         nodes.Starter
	tmdbBaseURL       string
	tmdbImagesBaseURL string
	// openSubtitlesBaseURL is where OpenSubtitles answers (opensubtitles.BaseURL, or a test's fake).
	openSubtitlesBaseURL string
}

// openedServices is what openAndWire opened and serve must close.
type openedServices struct {
	lock        *datadir.Lock
	database    *store.Database
	cache       *store.Cache
	settings    *settings.Service
	secrets     *secrets.Store
	jobServices jobServices
	nodes       *nodes.Service
}

func (opened openedServices) release() error {
	return errors.Join(opened.cache.Close(), opened.database.Close(), opened.lock.Release())
}

// prepareMaxSilence is how long a running prepare may go without a word from its node before its
// job is given up and retried: the node reports every two seconds while FFmpeg runs.
const prepareMaxSilence = 5 * time.Minute

// openAndWire opens the state directory and the database, migrates, and hands the API its
// dependencies.
func openAndWire(
	ctx context.Context, config Config, configReport ConfigReport, server *api.Server, outside wiring,
	logger *slog.Logger,
) (opened openedServices, err error) {
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return opened, err
	}
	opened.lock, err = directories.LockState()
	if err != nil {
		return opened, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, opened.lock.Release())
		}
	}()
	database, err := openAndMigrate(ctx, config, directories, reportMigration(server.SetStatus, logger))
	if err != nil {
		return opened, err
	}
	opened.database = database
	opened.cache, err = openCache(ctx, config, directories, logger)
	if err != nil {
		return opened, errors.Join(err, database.Close())
	}
	dependencies, err := buildDependencies(ctx, directories, database, logger)
	if err == nil {
		dependencies.ConfigFileKeys, err = configFileKeys(config, configReport)
	}
	if err == nil {
		err = mountDebug(ctx, server.Mux(), config, dependencies.Settings, logger)
	}
	if err == nil {
		opened.settings, opened.secrets = dependencies.Settings, dependencies.Secrets
		opened.jobServices, err = newJobServices(ctx, opened, outside, logger)
	}
	if err == nil {
		opened.nodes = newNodeService(opened, logger)
		opened.jobServices.artwork, err = newArtwork(ctx, opened, directories, outside.tmdbImagesBaseURL, logger)
	}
	if err != nil {
		return opened, errors.Join(err, opened.cache.Close(), database.Close())
	}
	dependencies.Chooser = opened.jobServices.chooser
	dependencies.Refresher = opened.jobServices.refresher
	dependencies.Libraries = library.NewAdmin(database, clock.System{}, opened.nodes)
	dependencies.Browser = library.NewBrowser(database, opened.settings)
	dependencies.Artwork = opened.jobServices.artwork
	dependencies.Playback = playback.NewService(playback.Parts{
		Database: database, Cache: opened.cache, Queue: opened.jobServices.queue, Nodes: opened.nodes,
		Settings: opened.settings, Clock: clock.System{}, Random: rand.Reader, Logger: logger,
		MaxSilence: prepareMaxSilence, Subtitler: opened.jobServices.subtitler, Preferences: dependencies.Preferences,
	})
	opened.jobServices.playback = dependencies.Playback
	opened.nodes.SetPrepareSink(dependencies.Playback)
	opened.nodes.SetSubtitleSink(dependencies.Playback)
	opened.nodes.SetLoudnessSink(dependencies.Playback)
	dependencies.NodeBytes = nodes.NewByteSocketClient(
		opened.nodes, filepath.Join(config.Server.SocketDir, nodeSocketName))
	dependencies.Jobs, dependencies.Providers = opened.jobServices.queue, opened.jobServices.providerServices.all()
	dependencies.NodeReports = opened.nodes
	dependencies.System = api.System{
		Build: thisBuild(), StartedAt: clock.System{}.Now(), DataDirectory: config.Server.DataDir,
		CacheDirectory: config.Server.CacheDir, LibraryDatabase: database, CacheDatabase: opened.cache,
	}
	server.Ready(dependencies)
	return opened, nil
}

// openAndMigrate opens library.db and migrates it, calling beforeStep, when it is not nil, before
// each step starts. Serving, the migrate command and the dev seed all open the database this way.
func openAndMigrate(
	ctx context.Context, config Config, directories *datadir.Directories, beforeStep func(step, total int),
) (*store.Database, error) {
	database, err := store.Open(ctx, directories.LibraryDatabasePath(), config.openOptions())
	if err != nil {
		return nil, err
	}
	err = database.Migrate(ctx, config.migrateOptions(directories.BackupDirectory(), beforeStep))
	if err != nil {
		return nil, errors.Join(err, database.Close())
	}
	return database, nil
}

// openCache opens cache.db, and discards and rebuilds one from another schema version: it is
// versioned by discard, and losing it loses nothing (schema.md, cache.db).
func openCache(
	ctx context.Context, config Config, directories *datadir.Directories, logger *slog.Logger,
) (*store.Cache, error) {
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

// buildDependencies writes any setting that has no row yet, loads the secret key, builds the
// services and makes sure a server with no users has a setup code, which it logs.
func buildDependencies(
	ctx context.Context, directories *datadir.Directories, database *store.Database, logger *slog.Logger,
) (api.Dependencies, error) {
	systemClock := clock.System{}
	settingsService := settings.NewService(database, systemClock, logger)
	seedReport, err := settingsService.Seed(ctx)
	if err != nil {
		return api.Dependencies{}, err
	}
	if len(seedReport.Inserted) > 0 {
		logger.InfoContext(ctx, "wrote the starting value of settings that had none", "keys", seedReport.Inserted)
	}
	if len(seedReport.Updated) > 0 {
		logger.InfoContext(ctx, "this release changed the starting value of settings nobody had set; they follow it",
			"keys", seedReport.Updated)
	}
	for _, unused := range seedReport.Unused {
		logger.WarnContext(ctx, "a setting row this release does not read; left alone, and doctor lists it",
			"key", unused.String())
	}
	secretKey, err := loadOrCreateSecretKey(directories, logger)
	if err != nil {
		return api.Dependencies{}, err
	}
	authService := auth.NewService(database, systemClock, rand.Reader, settingsService)
	codes := stateSetupCodes{directories: directories}
	code, err := authService.EnsureSetupCode(ctx, codes)
	if err != nil {
		return api.Dependencies{}, err
	}
	if code != "" {
		logger.WarnContext(ctx, "no one can sign in yet: create the first admin with this one-time setup code, "+
			"at POST /api/v1/setup; `orpheum setup-code` prints it again", "setupCode", code)
	}
	return api.Dependencies{
		Auth: authService, Settings: settingsService,
		Secrets: secrets.NewStore(database, secretKey, systemClock, rand.Reader, settingsService), SetupCodes: codes,
		Preferences: preferences.NewService(database, systemClock),
	}, nil
}

// loadOrCreateSecretKey reads secret.key, or writes a new one on first start. A key replaced or
// lost leaves every sealed secret "needs re-entering" and nothing else lost (D194 §4).
func loadOrCreateSecretKey(directories *datadir.Directories, logger *slog.Logger) (secrets.Key, error) {
	contents, err := directories.ReadStateFile(secrets.KeyFileName)
	if err == nil {
		return secrets.ParseKeyFile(contents)
	}
	if !errors.Is(err, datadir.ErrNotFound) {
		return secrets.Key{}, err
	}
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		return secrets.Key{}, err
	}
	err = directories.WriteStateFile(secrets.KeyFileName, key.EncodeKeyFile())
	if err != nil {
		return secrets.Key{}, err
	}
	logger.Info("wrote a new secret.key; back it up with `orpheum backup --include-key` to move machines")
	return key, nil
}

// stateSetupCodes keeps the setup code in the state directory, mode 0600.
type stateSetupCodes struct {
	directories *datadir.Directories
}

func (codes stateSetupCodes) Read() (string, error) {
	contents, err := codes.directories.ReadStateFile(auth.SetupCodeFileName)
	if errors.Is(err, datadir.ErrNotFound) {
		return "", auth.ErrNoSetupCode
	}
	return string(contents), err
}

func (codes stateSetupCodes) Write(code string) error {
	return codes.directories.WriteStateFile(auth.SetupCodeFileName, []byte(code))
}

func (codes stateSetupCodes) Remove() error {
	return codes.directories.RemoveStateFile(auth.SetupCodeFileName)
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

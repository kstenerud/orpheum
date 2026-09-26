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
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/webclient"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// serve runs the core on listener until ctx ends. The API answers /health before the database is
// even open, and says "migrating" while it migrates. A migration that fails leaves the server up
// and degraded, naming the step, the error and the snapshot, rather than exiting into a restart
// loop (migrations.md, When a step fails). Once the database is ready it runs the fetch queue,
// serves the node channel, and supervises the node child when the wiring can start one.
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

	opened, err := start(ctx, config, configReport, server, outside, logger)
	if err != nil {
		server.SetStatus(api.StatusDegraded, "the server could not start", err.Error())
		logger.ErrorContext(ctx, "the server could not start; it stays up to report why at /api/v1/health",
			"err", err)
		<-ctx.Done()
		return err
	}
	defer func() { err = errors.Join(err, opened.release()) }()
	stopFetching := opened.metadata.run(ctx, logger)
	defer stopFetching()
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
}

// started is what start opened and serve must close.
type started struct {
	lock     *datadir.Lock
	database *store.Database
	cache    *store.Cache
	settings *settings.Service
	secrets  *secrets.Store
	metadata metadata
	nodes    *nodes.Service
}

func (opened started) release() error {
	return errors.Join(opened.cache.Close(), opened.database.Close(), opened.lock.Release())
}

// prepareMaxSilence is how long a running prepare may go without a word from its node before its
// job is given up and retried: the node reports every two seconds while FFmpeg runs.
const prepareMaxSilence = 5 * time.Minute

// start opens the state directory and the database, migrates, and hands the API its
// dependencies.
func start(
	ctx context.Context, config Config, configReport ConfigReport, server *api.Server, outside wiring,
	logger *slog.Logger,
) (opened started, err error) {
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
	database, err := openAndMigrate(ctx, config, directories, server.SetStatus, logger)
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
		dependencies.ConfigFile, err = configFileKeys(config, configReport)
	}
	if err == nil {
		err = mountDebug(ctx, server.Mux(), config, dependencies.Settings, logger)
	}
	if err == nil {
		opened.settings, opened.secrets = dependencies.Settings, dependencies.Secrets
		opened.metadata, err = newMetadata(ctx, opened, outside.tmdbBaseURL, logger)
	}
	if err == nil {
		opened.nodes = newNodeService(opened, logger)
		opened.metadata.artwork, err = newArtwork(ctx, opened, directories, outside.tmdbImagesBaseURL, logger)
	}
	if err != nil {
		return opened, errors.Join(err, opened.cache.Close(), database.Close())
	}
	dependencies.Identify = opened.metadata.chooser
	dependencies.Refresh = opened.metadata.refresher
	dependencies.Libraries = library.NewAdmin(database, clock.System{}, opened.nodes)
	dependencies.Browse = library.NewBrowser(database, opened.settings)
	dependencies.Artwork = opened.metadata.artwork
	dependencies.Playback = playback.NewService(playback.Parts{
		Database: database, Cache: opened.cache, Queue: opened.metadata.queue, Nodes: opened.nodes,
		Settings: opened.settings, Clock: clock.System{}, Random: rand.Reader, Logger: logger,
		MaxSilence: prepareMaxSilence,
	})
	opened.metadata.playback = dependencies.Playback
	opened.nodes.ReceivePrepare(dependencies.Playback)
	opened.nodes.ReceiveSubtitles(dependencies.Playback)
	dependencies.NodeBytes = nodes.NewBytes(opened.nodes, filepath.Join(config.Server.SocketDir, nodeSocket))
	server.Ready(dependencies)
	return opened, nil
}

// openAndMigrate opens library.db and migrates it, reporting each step before it starts.
func openAndMigrate(
	ctx context.Context, config Config, directories *datadir.Directories,
	report func(api.Status, string, ...string), logger *slog.Logger,
) (*store.Database, error) {
	database, err := store.Open(ctx, directories.LibraryDatabase(), config.openOptions())
	if err != nil {
		return nil, err
	}
	err = database.Migrate(ctx, config.migrateOptions(directories.BackupDirectory(), reportMigration(report, logger)))
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
	cache, err := store.OpenCache(ctx, directories.CacheDatabase(), config.openOptions())
	if !errors.Is(err, store.ErrCacheOutdated) {
		return cache, err
	}
	logger.InfoContext(ctx, "discarding cache.db: it was built by another version", "why", err.Error())
	if err := directories.DiscardCacheDatabase(); err != nil {
		return nil, err
	}
	return store.OpenCache(ctx, directories.CacheDatabase(), config.openOptions())
}

// reportMigration returns the runner's BeforeStep: /health says "migrating: step 3 of 7" before
// each step starts (migrations.md, While migrating: never look dead).
func reportMigration(report func(api.Status, string, ...string), logger *slog.Logger) func(step, total int) {
	return func(step, total int) {
		detail := fmt.Sprintf("step %d of %d", step, total)
		report(api.StatusMigrating, detail)
		logger.Info("migrating", "step", step, "of", total)
	}
}

// buildDependencies writes any setting that has no row yet, loads the secret key, builds the
// services and makes sure a server with no users has a setup code, which it logs.
func buildDependencies(
	ctx context.Context, directories *datadir.Directories, database *store.Database, logger *slog.Logger,
) (api.Dependencies, error) {
	systemClock := clock.System{}
	settingsService := settings.NewService(database, systemClock, logger)
	report, err := settingsService.Seed(ctx)
	if err != nil {
		return api.Dependencies{}, err
	}
	if len(report.Written) > 0 {
		logger.InfoContext(ctx, "wrote the starting value of settings that had none", "keys", report.Written)
	}
	if len(report.Updated) > 0 {
		logger.InfoContext(ctx, "this release changed the starting value of settings nobody had set; they follow it",
			"keys", report.Updated)
	}
	for _, unused := range report.Unused {
		logger.WarnContext(ctx, "a setting row this release does not read; left alone, and doctor lists it",
			"key", unused.String())
	}
	key, err := loadOrCreateKey(directories, logger)
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
		Secrets: secrets.NewStore(database, key, systemClock, rand.Reader, settingsService), SetupCodes: codes,
	}, nil
}

// loadOrCreateKey reads secret.key, or writes a new one on first start. A key replaced or lost
// leaves every sealed secret "needs re-entering" and nothing else lost (D194 §4).
func loadOrCreateKey(directories *datadir.Directories, logger *slog.Logger) (secrets.Key, error) {
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

// mountDebug serves pprof on the API's mux when the serve_debug_pprof setting says so, and only on a
// loopback listener (cut-a §2a, D91). It reads the settings once, at start. Metrics are a setting
// too and not built yet; the log says so.
func mountDebug(
	ctx context.Context, mux *http.ServeMux, config Config, settingsService *settings.Service, logger *slog.Logger,
) error {
	pprofOn, err := settingsService.Bool(ctx, settings.ServeDebugPProf)
	if err != nil {
		return err
	}
	metricsOn, err := settingsService.Bool(ctx, settings.ServeDebugMetrics)
	if err != nil {
		return err
	}
	loopback := isLoopback(config.Server.Listen)
	if pprofOn && loopback {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		logger.InfoContext(ctx, "pprof is on", "path", "/debug/pprof/")
	}
	if (pprofOn || metricsOn) && !loopback {
		logger.WarnContext(ctx, "debug endpoints are served only on a loopback listen address; they are off",
			"listen", config.Server.Listen)
	}
	if metricsOn && loopback {
		logger.WarnContext(ctx, "serve_debug_metrics is set, but /metrics is not built yet; nothing is served there")
	}
	return nil
}

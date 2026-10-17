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
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/backup"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/loudness"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/reachability"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/core/renditions"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/core/webclient"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/localtime"
	"github.com/kstenerud/orpheum/internal/platform/pacing"
	"github.com/kstenerud/orpheum/internal/store"
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
	stopBackgroundJobs, err := opened.jobServices.startBackgroundLoops(ctx, logger)
	if err != nil {
		return degraded("the background jobs could not start", err)
	}
	defer stopBackgroundJobs()
	stopNodes, err := runNodeChannel(ctx, config, opened, logger)
	if err != nil {
		return degraded("the node channel could not start", err)
	}
	defer func() { err = errors.Join(err, stopNodes()) }()
	logger.InfoContext(ctx, "ready")
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
		"what", what, "err", err)
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
	// cloudflareBaseURL is the DNS provider's API (cloudflare.BaseURL, or a fake), and
	// dnsPropagationWait how long a challenge's record is given (D243 §6).
	cloudflareBaseURL  string
	dnsPropagationWait time.Duration
	// machineZone is the machine's own time zone, the server's until the admin chooses one
	// (D246 §7); a test's is UTC.
	machineZone localtime.Zone
}

// openedServices is what openAndWire opened and serve must close.
type openedServices struct {
	lock        *instancedir.Lock
	directories *instancedir.Directories
	database    *store.Database
	cache       *store.Cache
	settings    *settings.Service
	secrets     *secrets.Store
	jobServices jobServices
	nodes       *nodes.Service
	// exchanges are how a job asks a node for work; the node channel hands them the results.
	exchanges *nodes.Exchanges
	// together is Watch Together's sessions, whose memberships the voice relay admits, and whose links
	// and devices' locators a discovery finds the core by.
	together *together.Hub
	// auth keeps what a discovery finds the core by: its links and its devices' locators.
	auth *auth.Service
	// reachability checks the public path, and answers its echo for the admin's checks and for the
	// discovery's outside probe, whose nonces the heartbeat sends.
	reachability *reachability.Checker
}

// with is opened holding what the API's dependencies opened that the services opened after them use,
// and the Watch Together sessions, made before the discovery so that it may find their links.
func (opened openedServices) with(
	dependencies *api.Dependencies, directories *instancedir.Directories, logger *slog.Logger,
) openedServices {
	opened.settings, opened.secrets, opened.directories = dependencies.Settings, dependencies.Secrets, directories
	opened.auth = dependencies.Auth
	opened.together = newTogether(dependencies, logger)
	return opened
}

func (opened openedServices) release() error {
	return errors.Join(opened.cache.Close(), opened.database.Close(), opened.lock.Release())
}

// prepareMaxSilence is how long a running prepare may go without a word from its node before its
// job is given up and retried: the node reports every two seconds while FFmpeg runs.
const prepareMaxSilence = 5 * time.Minute

// openAndWire opens the instance's directories and the database, migrates, and hands the API its
// dependencies.
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
	opened.database = database
	opened.cache, err = openCache(ctx, config, directories, logger)
	if err != nil {
		return opened, errors.Join(err, database.Close())
	}
	dependencies, err := buildDependencies(ctx, directories, database, outside.machineZone, logger)
	if err == nil {
		dependencies.ConfigFileKeys, err = configFileKeys(config, configReport)
	}
	if err == nil {
		err = mountDebug(ctx, server.Mux(), config, dependencies.Settings, logger)
	}
	if err == nil {
		opened = opened.with(&dependencies, directories, logger)
		opened.jobServices, err = newJobServices(ctx, opened, outside, logger)
	}
	if err == nil {
		opened = openNodeWork(opened, logger)
		opened.jobServices.artwork, err = newArtwork(ctx, opened, directories, outside.tmdbImagesBaseURL, logger)
	}
	if err == nil {
		opened, err = openReachedFromOutside(ctx, config, opened, outside, server.RTC(), logger)
	}
	if err != nil {
		return opened, errors.Join(err, opened.cache.Close(), database.Close())
	}
	dependencies.Chooser, dependencies.Refresher = opened.jobServices.chooser, opened.jobServices.refresher
	dependencies.Libraries = library.NewAdmin(database, clock.System{}, opened.nodes, opened.jobServices.scans,
		library.Retirement{
			Work: opened.jobServices.queue, Titles: opened.jobServices.pass,
		})
	dependencies.Browser = library.NewBrowser(database, opened.settings)
	dependencies.Browser.SetNodeConnections(opened.nodes)
	opened.jobServices, dependencies.Playback = openPlayback(opened, database, dependencies.Preferences, logger)
	dependencies.Subtitles, dependencies.Renditions, dependencies.Artwork, dependencies.Chunks, dependencies.Discovery =
		opened.jobServices.subtitles, opened.jobServices.renditions, opened.jobServices.artwork,
		opened.jobServices.chunks, opened.jobServices.discovery
	opened.jobServices.nodes = opened.nodes
	opened.nodes.SetRenditionIndex(opened.jobServices.renditions)
	opened = wireJobsInto(&dependencies, opened)
	dependencies.NodeReports, dependencies.NodeWork = opened.nodes, opened.exchanges
	dependencies.NodeBytes, dependencies.Reachability, dependencies.Relay = openByteSocket(opened.nodes),
		opened.reachability, newRelayIssuer(config, opened)
	dependencies.System = api.System{
		Build: thisBuild(), StartedAt: clock.System{}.Now(), PersistentDirectory: directories.PersistentDirectory(),
		CacheDirectory: directories.CacheDirectory(), PersistentDatabase: database, CacheDatabase: opened.cache,
		PublicName: config.HTTPS.Domain, Certificate: certificateExpiry(opened.jobServices.certificates),
	}
	server.Ready(dependencies)
	return opened, nil
}

// newTogether is the Watch Together sessions, whose host may change what plays only to what they reach
// (D243 §4), read from dependencies when asked: the hub is made before the library, so the discovery
// may find its links (discovery-core.md K7).
func newTogether(dependencies *api.Dependencies, logger *slog.Logger) *together.Hub {
	mayPlay := func(ctx context.Context, userID, fileID int64) bool {
		reach, err := dependencies.Auth.ReachOfUser(ctx, userID)
		return err == nil && dependencies.Browser.ReachFile(ctx, reach, fileID) == nil
	}
	return together.NewHub(clock.System{}, rand.Reader, mayPlay, logger)
}

// wireJobsInto gives the API the job queue, the outside hosts' provider services and the port
// mapping's report, and makes a
// backup a job, the scheduled ones and an admin's (D243 §11); and the Watch Together sessions.
func wireJobsInto(dependencies *api.Dependencies, opened openedServices) openedServices {
	dependencies.Together = opened.together
	dependencies.Together.SnapSeeksBy(dependencies.Chunks.KeyframeAtOrBefore)
	// The last resort when the server's upload cannot carry its players at their lowest (D286 §5).
	dependencies.Playback.SendAwayBy(func() (int64, int64, bool) {
		return dependencies.Together.SendAwayNewest(playback.UploadFullReason)
	})
	// A group's players, put on at once, share the upload evenly from their start (D286 §5).
	dependencies.Playback.GroupPlayersBy(dependencies.Together.PlayersHostedBy)
	dependencies.Jobs, dependencies.Providers = opened.jobServices.queue, opened.jobServices.providerServices.all()
	dependencies.PortMapping = opened.jobServices.portMapping
	dependencies.Backups.UseQueue(opened.jobServices.queue, opened.settings)
	opened.jobServices.backups = dependencies.Backups
	return opened
}

// openPlayback is what plays a file and the work a play asks of the nodes: the read-through for its
// sounds' loudness and its picture's timeline, its subtitles, its prepared copies and its chunks, which it
// returns among the job services, and the playback service.
func openPlayback(
	opened openedServices, database *store.Database, preferencesService *preferences.Service, logger *slog.Logger,
) (jobServices, *playback.Service) {
	services, queue := opened.jobServices, opened.jobServices.queue
	services.readThrough = readthrough.NewService(readthrough.Parts{
		Database: database, Queue: queue, Nodes: opened.nodes, Exchanges: opened.exchanges, Clock: clock.System{},
	})
	levels := loudness.NewService(loudness.Parts{Database: database, Settings: opened.settings})
	services.subtitles = subtitles.NewService(subtitles.Parts{
		Database: database, Queue: queue, Nodes: opened.nodes, Exchanges: opened.exchanges,
		Subtitler: services.subtitler, Clock: clock.System{}, MaxSilence: prepareMaxSilence,
	})
	services.renditions = renditions.NewService(renditions.Parts{
		Database: database, Cache: opened.cache, Queue: queue, Nodes: opened.nodes, Exchanges: opened.exchanges,
		Settings: opened.settings, Loudness: levels, Clock: clock.System{}, Random: rand.Reader,
		Logger: logger,
	})
	services.chunks = chunks.NewService(chunks.Parts{
		Database: database, Cache: opened.cache, Settings: opened.settings, Queue: queue, Nodes: opened.nodes,
		Exchanges: opened.exchanges, Loudness: levels, ReadThrough: services.readThrough, Logger: logger,
		Clock: clock.System{},
	})
	services.playback = playback.NewService(playback.Parts{
		Database: database, Nodes: opened.nodes, Settings: opened.settings, Clock: clock.System{},
		Random: rand.Reader, Renditions: services.renditions, Subtitles: services.subtitles,
		Preferences: preferencesService, Loudness: levels, Chunks: services.chunks, Reads: opened.jobServices.scans,
	})
	// A run its node cannot make fast enough moves its players down a quality, before it is evicted
	// (D286 §4, D309 §6); and the runs that go on are those the players want (D309).
	services.chunks.MovePlayersBy(services.playback.MoveOffStream)
	services.chunks.InterestsBy(services.playback.InterestsIn)
	return services, services.playback
}

// openExchanges are how the server gives its nodes work and takes their word on it, wired to the
// node channel and the job runners (D234 §7).
func openExchanges(opened openedServices, logger *slog.Logger) *nodes.Exchanges {
	exchanges := nodes.NewExchanges(nodes.ExchangeParts{
		Sender: opened.nodes, Clock: clock.System{}, Random: rand.Reader, Logger: logger, Cache: opened.cache,
		Settings: opened.settings, MaxSilence: prepareMaxSilence, Rooms: opened.nodes,
	})
	opened.nodes.SetResultSink(exchanges)
	return exchanges
}

// openNodeWork opens the node channel's service, the exchanges that give the nodes their work, and the
// scans of their roots.
func openNodeWork(opened openedServices, logger *slog.Logger) openedServices {
	var ingester *library.Ingester
	opened.nodes, ingester = newNodeService(opened, logger)
	opened.exchanges = openExchanges(opened, logger)
	opened.jobServices.exchanges = opened.exchanges
	opened.jobServices.scans = openScans(opened, ingester, logger)
	opened.jobServices.scanSchedule = library.NewScanSchedule(
		opened.database, opened.jobServices.scans, opened.settings, clock.System{}, logger)
	opened.nodes.SetChanges(opened.jobServices.scanSchedule)
	return opened
}

// openScans are the scans of the nodes' roots, which a node's hello asks for (D240), and a stall on a
// root's disk asks for too (D233 §10).
func openScans(opened openedServices, ingester *library.Ingester, logger *slog.Logger) *library.Scans {
	scans := library.NewScans(library.ScansParts{
		Cache: opened.cache, Ingester: ingester, Jobs: opened.jobServices.queue, Assigner: opened.exchanges,
		Settings: opened.settings, Clock: clock.System{}, Logger: logger,
	})
	opened.nodes.SetScans(scans)
	opened.exchanges.SetDiskStalls(scans)
	holdSlowReadsForQuickPasses(opened.exchanges, scans)
	return scans
}

// holdSlowReadsForQuickPasses has a background read of whole films wait while a scan is on its quick
// pass of the film's disk, so a library is listed and playable before any film is read through
// (D255 §3).
func holdSlowReadsForQuickPasses(exchanges *nodes.Exchanges, scans nodes.QuickPasses) {
	exchanges.SetQuickPasses(scans)
}

// openByteSocket is the core's client of the node's byte socket: media bytes for the API, and
// results the node holds, too large to post, for the node channel (D234 §5).
func openByteSocket(service *nodes.Service) *nodes.ByteSocketClient {
	byteSocket := nodes.NewByteSocketClient(service)
	service.SetHeldResultReader(byteSocket)
	return byteSocket
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
	err = database.Migrate(ctx, config.migrateOptions(directories.BackupDirectory(), beforeStep))
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

// buildDependencies writes any setting that has no row yet, loads the secret key, builds the
// services and makes sure a server with no users has a setup code, which it logs.
func buildDependencies(
	ctx context.Context, directories *instancedir.Directories, database *store.Database,
	machineZone localtime.Zone, logger *slog.Logger,
) (api.Dependencies, error) {
	systemClock := clock.System{}
	settingsService := settings.NewService(database, systemClock, logger)
	settingsService.UseMachineZone(machineZone)
	logger.InfoContext(ctx, "the machine's time zone, the server's until one is chosen", "timeZone", machineZone.Name())
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
	secretStore := secrets.NewStore(database, secretKey, systemClock, rand.Reader, settingsService)
	authService := auth.NewService(database, systemClock, rand.Reader, settingsService)
	// Each link keeps its secret sealed, so a person's own link can be shown to them again (K7).
	authService.SealLinksWith(secretStore)
	codes := runSetupCodes{directories: directories}
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
		Secrets: secretStore, SetupCodes: codes,
		Preferences:   preferences.NewService(database, systemClock),
		Backups:       backup.NewService(database, directories, systemClock),
		FilingReports: identify.NewFilingReports(database, directories, thisBuild().Version, systemClock),
		Clock:         systemClock,
	}, nil
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

// runSetupCodes keeps the setup code in the instance's run directory, mode 0600 (D242 §3).
type runSetupCodes struct {
	directories *instancedir.Directories
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

func (codes runSetupCodes) Remove() error {
	return codes.directories.RemoveRunFile(auth.SetupCodeFileName)
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
	rtc := newWebRTC(ctx, server.Handler(), logger)
	listenForWebRTCAtHome(ctx, rtc, config, logger)
	server.SetRTC(rtc)
	return rtc
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

// newWebRTC is the WebRTC server over the API's handler, as this build serves it: a development build
// lets a discovery's offers reach the machine's own addresses, a release never (relaxForDevelopment).
func newWebRTC(ctx context.Context, handler http.Handler, logger *slog.Logger) *rtcapi.Server {
	return relaxForDevelopment(rtcapi.New(ctx, handler, logger))
}

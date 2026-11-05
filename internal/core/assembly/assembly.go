// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/backup"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/interest"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/core/resourceuse"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/core/waits"
	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/platform/selfuse"
	"github.com/kstenerud/orpheum/internal/store"
)

// Parts are what the core's services are assembled over: what only the composition root can open or
// read (the stores, the settings seeded at start, the secret key, the clock, the randomness, the
// log, this build), where the outside hosts answer, and, for a test, its fakes of what reaches a node
// or another host, each nil in production, which assembles the core's own (D351 §3).
type Parts struct {
	Database    *store.Database
	Cache       *store.Cache
	Directories *instancedir.Directories
	// Settings are the server's settings, seeded before anything reads one (D201).
	Settings *settings.Service
	// SecretKey seals the secrets (D194 §4).
	SecretKey secrets.Key
	// SetupCodes keep the setup code a server with no users is set up by (D242 §3).
	SetupCodes auth.SetupCodes
	Clock      clock.Clock
	// Random is every service's randomness: tokens, keys, nonces, and the job queue's jitter unless
	// Jitter is given.
	Random io.Reader
	// Jitter is the job queue's alone; nil is Random. A test's makes every wait exact.
	Jitter io.Reader
	Logger *slog.Logger
	// Build is what this binary was built from: the outbound client names it to other hosts, and a
	// filing report its version.
	Build build.Provenance
	// TMDBBaseURL and TMDBImagesBaseURL are where TMDB answers and serves its images
	// (tmdb.BaseURL and tmdb.ImageBaseURL, or a test's fake); OpenSubtitlesBaseURL is where
	// OpenSubtitles answers.
	TMDBBaseURL, TMDBImagesBaseURL, OpenSubtitlesBaseURL string

	// Nodes is a test's fake of the node channel; nil is the core's own, nodes.Service, which a node
	// dials (NodeChannel).
	Nodes Nodes
	// NodeBytes is a test's fake of the node's bytes; nil reads them over the core's own node
	// channel's byte socket. A faked Nodes needs it faked too.
	NodeBytes api.NodeBytes
	// ScanRequests is a test's fake of how an admin asks for a scan; nil is the core's own scans.
	ScanRequests library.ScanRequests
	// FileReads is a test's fake of how a play has a file not yet read taken first; nil is the
	// core's own scans.
	FileReads playback.FileReads
	// Subtitler is a test's fake of OpenSubtitles; nil is OpenSubtitles at OpenSubtitlesBaseURL.
	Subtitler subtitles.Subtitler
	// Metadata are a test's fake metadata providers; nil is TMDB at TMDBBaseURL.
	Metadata identify.Providers
	// ReadOwnUse is a test's fake of what the core reads of its own use of its machine; nil is
	// selfuse.Read.
	ReadOwnUse func() (selfuse.Sample, error)
}

// Services are the core's services, assembled: the API's dependencies, and what the composition root
// runs: the node channel it listens with, the job queue's runners and sweeps (Runners, Sweeps), and
// the loops beside them.
type Services struct {
	// API is what the API answers with. Its caller adds what the config alone makes: the discovery,
	// the port mapping, the reachability check, the voice relay, the System page's facts and the
	// config file's keys.
	API api.Dependencies
	// NodeChannel is the core's side of the node channel, which each node dials; nil when a test
	// faked the nodes.
	NodeChannel *nodes.Service
	// OutboundClient is the one client every call to another host goes through, sharing one
	// transport and its connections.
	OutboundClient *http.Client
	// Queue is the one job queue.
	Queue *jobs.Queue
	// Exchanges give node jobs to nodes, and resolve each by its node's word (D234 §7).
	Exchanges *nodes.Exchanges
	// Scans run the scans of the nodes' roots: a listing, then the batches it leaves (D240).
	Scans *library.Scans
	// Interest orders the work by what people play and look at (D324).
	Interest *interest.Raiser
	// ReadThrough reads each video through, for its sounds' loudness and its picture's timeline
	// (D226, D247 §10).
	ReadThrough *readthrough.Service
	// Certificates renew the HTTPS certificates; the composition root, which makes them from its
	// config, sets them before the runners start. Nil holds none.
	Certificates Certificates

	clock     clock.Clock
	settings  *settings.Service
	pass      *identify.Pass
	matcher   *identify.Matcher
	chooser   *identify.Chooser
	refresher *identify.Refresher
	// rescans are an admin's rescans: a scan, then a match again of every film and series in its
	// scope (D357).
	rescans *identify.Rescans
	artwork *artwork.Service
	// chunks cuts films into chunks made on request, run by run (D247 §10).
	chunks *chunks.Service
	// playback plays files, and counts what its players need of the server's upload (D286 §5).
	playback *playback.Service
	// subtitles has a played video's subtitles made and fetches them on request (D217 §6, D224).
	subtitles *subtitles.Service
	// subtitler fetches subtitles on request (D224).
	subtitler subtitles.Subtitler
	// nodes is the node channel as the services ask it: NodeChannel, or a test's fake.
	nodes Nodes
	// ingester stores each scan's batches, and hands each scanned library to identification.
	ingester *library.Ingester
	// scanSchedule asks for the scans libraries that notice changes by themselves are due (D27,
	// D272 §1).
	scanSchedule *library.ScanSchedule
	// copies confirms the files found at a second place that are copies of one found before (D363 §2).
	copies *library.Copies
	// backups makes the backups, scheduled and asked for, and prunes them (D243 §11).
	backups *backup.Service
	// providerServices are each outside host's provider service, which every call to it asks
	// first (D231 §5): TMDB's API, TMDB's image server and OpenSubtitles, in that order.
	providerServices hostProviders
	// invariants checks persistent.db's invariants, at start and daily (D303, M7).
	invariants invariantChecker
	// credentials deletes the credentials ended past their keeping, daily (D304 §6).
	credentials credentialPurger
	// removedFiles deletes the files removed past their keeping, daily (D304 §6, D306 §8).
	removedFiles removedFilePurger
}

// Certificates are the HTTPS certificates the server holds, as the jobs renew them.
type Certificates interface {
	// QueueRenewal asks for the renewal of each certificate that is due, and says how many it asked
	// for.
	QueueRenewal(ctx context.Context) (int, error)
	// HandleRenew renews each certificate that is due: the renewal's job.
	HandleRenew(ctx context.Context, job jobs.Job) error
}

// ErrNoPublicName is a renewal asked of a server with no name to hold a certificate for.
var ErrNoPublicName = errors.New("the server has no public name (https.domain) and no discovery, " +
	"so no certificate to renew")

// nodeWorkMaxSilence is how long running work may go without a word from its node before its job
// is given up and retried: the node reports at every status tick, and a run of chunks at each chunk.
const nodeWorkMaxSilence = 5 * time.Minute

// Build assembles the core's services over parts and wires them to each other, and returns the jobs
// a stop left running to the queue. It starts nothing: the composition root runs the node channel,
// the runners and the loops.
func Build(ctx context.Context, parts Parts) (*Services, error) {
	services := &Services{clock: parts.Clock, settings: parts.Settings}
	services.API = buildAccounts(ctx, parts)
	// The Watch Together sessions are made before the discovery, which finds the core by their links.
	services.API.Together = newTogether(&services.API, parts)
	if err := services.openJobs(ctx, parts); err != nil {
		return nil, err
	}
	services.openNodeWork(parts)
	if err := services.openArtwork(ctx, parts); err != nil {
		return nil, err
	}
	dependencies := &services.API
	dependencies.Chooser, dependencies.Refresher = services.chooser, services.refresher
	dependencies.Libraries = services.libraryAdmin(parts)
	services.rescans = identify.NewRescans(services.matcher, services.Queue, dependencies.Libraries)
	services.rescans.OnRequested(func() { services.Queue.RequestSweeps(sweepRescans) })
	dependencies.Rescans = services.rescans
	dependencies.Browser = library.NewBrowser(parts.Database, parts.Settings)
	dependencies.Browser.SetNodeConnections(services.nodes)
	services.openPlayback(parts)
	dependencies.Playback, dependencies.Subtitles, dependencies.Artwork, dependencies.Chunks =
		services.playback, services.subtitles, services.artwork, services.chunks
	if err := services.wireJobsInto(parts); err != nil {
		return nil, err
	}
	dependencies.NodeReports, dependencies.NodeWork = services.nodes, services.Exchanges
	nodeBytes, err := services.nodeBytes(parts)
	if err != nil {
		return nil, err
	}
	dependencies.NodeBytes = nodeBytes
	// The node's folders, for an admin choosing a library folder, are listed on its byte socket (D359 §2).
	if lister, isLister := nodeBytes.(library.FolderLister); isLister {
		dependencies.Libraries.ListFoldersThrough(lister)
	}
	return services, nil
}

// buildAccounts is the accounts and what is kept beside them: the secrets, sealed by the secret key,
// the sign-ins, a person's preferences, the backups and the filing reports.
func buildAccounts(ctx context.Context, parts Parts) api.Dependencies {
	secretStore := secrets.NewStore(parts.Database, parts.SecretKey, parts.Clock, parts.Random, parts.Settings)
	authService := auth.NewService(parts.Database, parts.Clock, parts.Random, parts.Settings)
	// Each link keeps its secret sealed, so a person's own link can be shown to them again (K7).
	authService.SealLinksWith(secretStore)
	authService.OnSignInLimitFull(warnSignInLimitFull(ctx, parts.Logger))
	return api.Dependencies{
		Auth: authService, Settings: parts.Settings,
		Secrets: secretStore, SetupCodes: parts.SetupCodes,
		Preferences:   preferences.NewService(parts.Database, parts.Clock),
		Backups:       backup.NewService(parts.Database, parts.Directories, parts.Clock),
		FilingReports: identify.NewFilingReports(parts.Database, parts.Directories, parts.Build.Version, parts.Clock),
		Clock:         parts.Clock,
	}
}

// warnSignInLimitFull warns, once a minute at most as auth tells it, that the failed-sign-in limit is
// full and refuses sign-ins it holds no count for: only an attack from many networks fills it.
func warnSignInLimitFull(ctx context.Context, logger *slog.Logger) func() {
	return func() {
		logger.WarnContext(ctx, "the failed-sign-in limit is full, so sign-ins from places with no recent failure "+
			"are refused until its counts age out: many networks are failing sign-ins at once")
	}
}

// endCastsOfSession ends, as a Watch Together session ends, every cast whose receiver was handed its
// person's part in it: a member's cast of the group's film, the host's included (D389 §1). Each
// cast's session ends as sessionEnded, so its receiver goes home and its sender's page is told.
func endCastsOfSession(casts *live.Registry, hub *together.Hub, logger *slog.Logger) func(sessionID int64) {
	return func(sessionID int64) {
		for _, ended := range casts.EndPartIn(sessionID) {
			_ = hub.EndCast(ended.ID, ended.UserID, together.CastSessionEnded) //nolint:errcheck // ended either way
			logger.Info("cast: a cast ended", "castId", ended.ID, "userId", ended.UserID,
				"to", ended.ReceiverDeviceID, "sessionId", sessionID, "reason", together.CastSessionEnded)
		}
	}
}

// newTogether is the Watch Together sessions, whose host may change what plays only to what they reach
// (D243 §4), read from dependencies when asked: the hub is made before the library, so the discovery
// may find its links (discovery-core.md K7).
func newTogether(dependencies *api.Dependencies, parts Parts) *together.Hub {
	mayPlay := func(ctx context.Context, userID, fileID int64) bool {
		reach, err := dependencies.Auth.ReachOfUser(ctx, userID)
		return err == nil && dependencies.Browser.ReachFile(ctx, reach, fileID) == nil
	}
	return together.NewHub(parts.Clock, parts.Random, mayPlay, parts.Logger)
}

// openJobs builds the outbound client, the job queue, the outside hosts' provider services, the
// identification pass and its matcher, and the subtitler, and returns the jobs a crash left running
// to the queue.
func (services *Services) openJobs(ctx context.Context, parts Parts) error {
	providerTimeout, err := parts.Settings.Duration(ctx, settings.ProviderTimeoutSeconds)
	if err != nil {
		return err
	}
	services.OutboundClient, err = outbound.NewClient(providerTimeout, parts.Build.String())
	if err != nil {
		return err
	}
	metadata := newMetadataJobs(metadataParts{
		database: parts.Database, cache: parts.Cache, settings: parts.Settings, secrets: services.API.Secrets,
		clock: parts.Clock, random: jitterOf(parts), httpClient: services.OutboundClient,
		tmdbBaseURL: parts.TMDBBaseURL, providers: parts.Metadata, logger: parts.Logger,
	})
	services.Queue, services.providerServices, services.pass = metadata.queue, metadata.hosts, metadata.pass
	services.matcher, services.refresher = metadata.matcher, metadata.refresher
	services.subtitler = parts.Subtitler
	if services.subtitler == nil {
		services.subtitler, err = opensubtitles.New(services.OutboundClient, parts.OpenSubtitlesBaseURL,
			services.API.Secrets, parts.Clock, metadata.hosts.openSubtitles)
		if err != nil {
			return err
		}
	}
	requeued, err := services.Queue.RequeueRunning(ctx)
	if err != nil {
		return err
	}
	if requeued > 0 {
		parts.Logger.InfoContext(ctx, "returned jobs a stop left running to the queue", "requeuedCount", requeued)
	}
	if err := keepSortAndSearchKeysCurrent(ctx, services.pass, parts.Settings, parts.Logger); err != nil {
		return err
	}
	services.pass.AfterPass(func(ctx context.Context, report identify.Report) {
		queueOwedFetches(ctx, services.Queue, report, parts.Logger)
		// A pass follows every scan: new files may want measuring (D226 §2), and new items their
		// matches. Every sweep but the pass's own runs after it (D231 §3).
		services.Queue.RequestSweeps(sweepsAfterAPass()...)
	})
	wakeOwedPasses(services.pass, services.Queue)
	services.invariants, services.credentials = parts.Database, services.API.Auth
	services.removedFiles = library.NewFilePurger(parts.Database)
	services.chooser = identify.NewChooser(services.matcher, services.pass)
	return nil
}

// jitterOf is the job queue's randomness: Jitter when a test gave one, Random otherwise.
func jitterOf(parts Parts) io.Reader {
	if parts.Jitter != nil {
		return parts.Jitter
	}
	return parts.Random
}

// libraryAdmin is the library admin service, reading the server's settings.
func (services *Services) libraryAdmin(parts Parts) *library.Admin {
	scans := parts.ScanRequests
	if scans == nil {
		scans = services.Scans
	}
	admin := library.NewAdmin(parts.Database, parts.Clock, services.nodes, scans,
		library.Retirement{Work: services.Queue})
	admin.UseSettings(parts.Settings)
	admin.UseLogger(parts.Logger)
	return admin
}

// wireJobsInto gives the API the job queue, the outside hosts' provider services, and makes a backup
// a job, the scheduled ones and an admin's (D243 §11); the Watch Together sessions; and what orders
// the work by interest (D324).
func (services *Services) wireJobsInto(parts Parts) error {
	dependencies := &services.API
	dependencies.Together.SnapSeeksBy(dependencies.Chunks.KeyframeAtOrBefore)
	// Which of each person's devices are open now, and the casts between them (D337).
	dependencies.Live = live.NewRegistry(dependencies.Together.CastCredential, parts.Clock)
	// A cast's receiver plays the Watch Together session its person hosts, if any (D364 §11).
	dependencies.Live.TakesPartBy(dependencies.Together.TogetherPart)
	// A Watch Together session's end ends every cast whose receiver takes a part in it (D389 §1).
	dependencies.Together.OnSessionEnded(endCastsOfSession(dependencies.Live, dependencies.Together, parts.Logger))
	// The last resort when the server's upload cannot carry its players at their lowest (D286 §5).
	dependencies.Playback.SendAwayBy(func() (int64, int64, bool) {
		return dependencies.Together.SendAwayNewest(playback.UploadFullReason)
	})
	// A group's players, put on at once, share the upload evenly from their start (D286 §5).
	dependencies.Playback.GroupPlayersBy(dependencies.Together.PlayersHostedBy)
	dependencies.Jobs, dependencies.Providers = services.Queue, services.providerServices.all()
	dependencies.Backups.UseQueue(services.Queue, parts.Settings)
	services.backups = dependencies.Backups
	var err error
	if services.Interest, err = services.newInterestRaiser(parts); err != nil {
		return err
	}
	// How long the work a viewer waits on will take, timing the read-throughs as they end (D324 §6).
	dependencies.Waits, err = waits.NewEstimator(waits.Parts{
		Database: parts.Database, Cache: parts.Cache, Attempts: services.Exchanges, Nodes: services.nodes,
		ReadThroughs: services.ReadThrough, Playing: playingFiles(dependencies.Playback), Clock: parts.Clock,
	})
	if err != nil {
		return fmt.Errorf("say how long work will take: %w", err)
	}
	// What the core and each node use of their machines, beside the plays and the work running.
	readOwnUse := parts.ReadOwnUse
	if readOwnUse == nil {
		readOwnUse = selfuse.Read
	}
	dependencies.ResourceUse = resourceuse.NewRecorder(parts.Cache, parts.Clock, parts.Logger, resourceuse.Sources{
		ReadOwn: readOwnUse, Nodes: services.nodes, Work: services.Exchanges,
		Plays: func() int { return len(dependencies.Playback.NowPlaying()) },
	})
	return nil
}

// newInterestRaiser is what orders the work by what people play and look at (D324), its register
// handed to the API.
func (services *Services) newInterestRaiser(parts Parts) (*interest.Raiser, error) {
	dependencies := &services.API
	dependencies.Interest = interest.NewRegister(parts.Clock)
	raiser, err := interest.NewRaiser(interest.Parts{
		Register: dependencies.Interest, Jobs: jobs.NewInterestJobs(parts.Cache, parts.Clock),
		Nodes: services.Exchanges, Waiting: services.Queue, Reads: dependencies.Playback,
		Playing:  playingFiles(dependencies.Playback),
		Database: parts.Database, Settings: parts.Settings, Clock: parts.Clock, Logger: parts.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("order the work by interest: %w", err)
	}
	return raiser, nil
}

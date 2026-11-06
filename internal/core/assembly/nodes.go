// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"context"
	"errors"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/core/resourceuse"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/core/waits"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// Nodes are the node channel as the core's services ask it, nodes.Service or a test's fake: the work
// they give the nodes, what they read of them, and the setters that wire them to it (D351 §3).
type Nodes interface {
	nodeWork
	nodeFacts
	nodeWiring
}

// nodeWork is how the services give the nodes their work, as each one's own package asks it.
type nodeWork interface {
	nodes.Sender
	nodes.Rooms
	artwork.Nodes
	readthrough.Nodes
	subtitles.Nodes
}

// nodeFacts are what the services read of the nodes, as each one's own package asks it.
type nodeFacts interface {
	chunks.Encoders
	waits.Nodes
	library.Nodes
	library.SystemFolders
	api.NodeReports
	resourceuse.Nodes
}

// nodeWiring are the setters that wire the services to the node channel.
type nodeWiring interface {
	// SetResultSink is where the node's results on the work it was given go: the exchanges.
	SetResultSink(sink nodes.ResultSink)
	// SetScans are the scans a node's hello asks for: the scans of its roots (D240).
	SetScans(scans nodes.ScanRequester)
	// SetChanges takes what a node's watch saw change: the scan schedule (D272 §1).
	SetChanges(changes nodes.ChangeTaker)
	// SetStoreTiers say how strongly the players want each chunk store, which a node keeps them by
	// (D309 §8).
	SetStoreTiers(tiers nodes.StoreTiers)
}

// commandPollHold is how long a node's long-poll for commands is held when there is nothing to
// say. It is shorter than any idle timeout between two processes on one machine, and long
// enough that an idle node costs nothing.
const commandPollHold = 25 * time.Second

// errNodeBytesNotFaked is a test that faked the nodes but not their bytes, which only the core's own
// node channel reads.
var errNodeBytesNotFaked = errors.New("a faked node channel needs its bytes faked too (Parts.NodeBytes)")

// openNodeWork opens the node channel's service, unless a test faked it, the exchanges that give the
// nodes their work, and the scans of their roots.
func (services *Services) openNodeWork(parts Parts) {
	services.ingester = newIngester(services, parts)
	services.nodes = parts.Nodes
	if services.nodes == nil {
		services.NodeChannel = nodes.NewService(nodes.ServiceParts{
			Database: parts.Database, Clock: parts.Clock, Settings: parts.Settings, Destinations: services.Queue,
			Logger: parts.Logger, PollHold: commandPollHold,
		})
		services.nodes = services.NodeChannel
	}
	services.Exchanges = openExchanges(services.nodes, parts)
	services.Scans = openScans(services, parts)
	services.scanSchedule = library.NewScanSchedule(
		parts.Database, services.Scans, parts.Settings, parts.Clock, parts.Logger)
	services.nodes.SetChanges(services.scanSchedule)
	services.copies = openCopies(services, parts)
}

// openCopies is the confirmation of the files found at a second place: what other packages hold of a
// copy moves onto its file, and the libraries that cover it owe a pass (D363 §2).
func openCopies(services *Services, parts Parts) *library.Copies {
	pass := services.pass
	return library.NewCopies(library.CopiesParts{
		Database: parts.Database, Jobs: services.Queue, Assigner: services.Exchanges, Nodes: services.nodes,
		Work: services.Queue, RetireVersion: identify.RetireVersionInto,
		Folds: []library.CopyFold{
			playback.MoveWatchingToCopysFile, preferences.MoveChoicesToCopysFile, subtitles.MoveFetchedToCopysFile,
		},
		Passes: func(_ context.Context, libraryID int64) { pass.OwePass(libraryID) },
		Clock:  parts.Clock, Logger: parts.Logger,
	})
}

// newIngester is the ingest, which hands each scanned library to identification and brings back the
// work parked on each root or disk a scan finds.
func newIngester(services *Services, parts Parts) *library.Ingester {
	logger, pass, queue := parts.Logger, services.pass, services.Queue
	ingester := library.NewIngester(parts.Database, parts.Clock, parts.Settings, logger)
	ingester.SetOnScanEnded(func(ctx context.Context, libraryID int64) {
		report, err := pass.IdentifyLibrary(ctx, libraryID)
		if err != nil {
			logger.ErrorContext(ctx, "identification failed; the scan is stored and the next one tries again",
				"libraryId", libraryID, "err", err)
			return
		}
		logger.InfoContext(ctx, "identified a library", slogx.EventKey, "libraryIdentified", "libraryId", libraryID,
			"listed", report.ListedCount, "extras", report.ExtraCount,
			"unidentified", report.UnidentifiedCount, "awaitingMatch", len(report.AwaitingMatch))
	})
	// A row stored unread whose read finds a file stored before gives up the title the pass made
	// from its path (D304 §5).
	ingester.SetVersionRetirer(identify.RetireVersionInto)
	askForAPassOnNewFiles(ingester, pass)
	ingester.SetOnPathsFound(func(ctx context.Context, nodeID int64, paths []string) {
		for _, path := range paths {
			queue.Unpark(ctx, jobs.RootDestination(nodeID, path)) // a root back is a root's probe (D231 §6)
		}
	})
	return ingester
}

// sweepRequester asks for sweeps to run soon: the job queue.
type sweepRequester interface {
	RequestSweeps(names ...string)
}

// passOwer marks a library as owing an identification pass: the pass, which asks for its owed
// passes' sweep.
type passOwer interface {
	OwePass(libraryID int64)
}

// askForAPassOnNewFiles has a batch of a scan that stored a new film ask for its library's
// identification pass while the scan goes on, so what is found early is playable early (D236 §1):
// owed, as a match or a refresh asks for one, so the sweeper runs it once however many batches
// asked (D303, DL14), and every other library's pass is left alone (audit 2026-10-25, C8).
func askForAPassOnNewFiles(ingester *library.Ingester, passes passOwer) {
	ingester.SetOnNewFiles(func(_ context.Context, libraryID int64) { passes.OwePass(libraryID) })
}

// openExchanges are how the server gives its nodes work and takes their word on it, wired to the
// node channel and the job runners (D234 §7).
func openExchanges(channel Nodes, parts Parts) *nodes.Exchanges {
	exchanges := nodes.NewExchanges(nodes.ExchangeParts{
		Sender: channel, Clock: parts.Clock, Random: parts.Random, Logger: parts.Logger, Cache: parts.Cache,
		Settings: parts.Settings, MaxSilence: nodeWorkMaxSilence, Rooms: channel,
	})
	channel.SetResultSink(exchanges)
	return exchanges
}

// openScans are the scans of the nodes' roots, which a node's hello asks for (D240), and a stall on a
// root's disk asks for too (D233 §10).
func openScans(services *Services, parts Parts) *library.Scans {
	scans := library.NewScans(library.ScansParts{
		Cache: parts.Cache, Ingester: services.ingester, Jobs: services.Queue, Assigner: services.Exchanges,
		Settings: parts.Settings, Clock: parts.Clock, Logger: parts.Logger,
	})
	services.nodes.SetScans(scans)
	// A rescan waits for its library's scan to be over; it is told the moment one ends.
	scans.OnEnded(func() { services.Queue.RequestSweeps(sweepRescans) })
	services.Exchanges.SetDiskStalls(scans)
	holdSlowReadsForQuickPasses(services.Exchanges, scans)
	return scans
}

// holdSlowReadsForQuickPasses has a background read of whole films wait while a scan is on its quick
// pass of the film's disk, so a library is listed and playable before any film is read through
// (D255 §3).
func holdSlowReadsForQuickPasses(exchanges *nodes.Exchanges, scans nodes.QuickPasses) {
	exchanges.SetQuickPasses(scans)
}

// nodeBytes are how the API reads a file's bytes from its node: a test's fake, or the core's client
// of the node's byte socket.
func (services *Services) nodeBytes(parts Parts) (api.NodeBytes, error) {
	if parts.NodeBytes != nil {
		return parts.NodeBytes, nil
	}
	if services.NodeChannel == nil {
		return nil, errNodeBytesNotFaked
	}
	return openByteSocket(services.NodeChannel, services.Scans), nil
}

// openByteSocket is the core's client of the node's byte socket: media bytes for the API, results
// the node holds, too large to post, for the node channel (D234 §5), and the probes of roots the
// scans ask, outside every queue (D327).
func openByteSocket(service *nodes.Service, scans *library.Scans) *nodes.ByteSocketClient {
	byteSocket := nodes.NewByteSocketClient(service, scans)
	service.SetHeldResultReader(byteSocket)
	if scans != nil {
		scans.SetProber(byteSocket)
	}
	return byteSocket
}

// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// commandPollHold is how long a node's long-poll for commands is held when there is nothing to
// say. It is shorter than any idle timeout between two processes on one machine, and long
// enough that an idle node costs nothing.
const commandPollHold = 25 * time.Second

// runNodeChannel serves the node channel on the core's socket, which each node dials; the returned
// stop ends it. The core starts no node: each is a service of its own (D233 §2, D235 §4).
func runNodeChannel(
	ctx context.Context, config Config, opened openedServices, logger *slog.Logger,
) (stop func() error, err error) {
	service := opened.nodes
	listener, err := unixsocket.Listen(config.Server.SocketDir, coreSocketName)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: service.Handler(), ReadHeaderTimeout: config.readHeaderTimeout()}
	var running sync.WaitGroup
	running.Go(func() {
		if serveErr := server.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the node channel stopped", "err", serveErr)
		}
	})
	return func() error {
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.shutdownGrace())
		defer cancel()
		err := server.Shutdown(shutdownContext)
		if errors.Is(err, context.DeadlineExceeded) {
			err = server.Close() // a report stream that did not end in time
		}
		running.Wait()
		return err
	}, nil
}

// newNodeService builds the core's side of the node channel: the ingest, which hands each scanned
// library to identification and brings back the work parked on each root or disk a scan finds.
// It serves nothing until runNodeChannel listens.
func newNodeService(opened openedServices, logger *slog.Logger) *nodes.Service {
	ingester := library.NewIngester(opened.database, clock.System{}, opened.settings, logger)
	pass := opened.jobServices.pass
	ingester.SetOnScanEnded(func(ctx context.Context, libraryID int64) {
		report, err := pass.IdentifyLibrary(ctx, libraryID)
		if err != nil {
			logger.ErrorContext(ctx, "identification failed; the scan is stored and the next one tries again",
				"libraryId", libraryID, "err", err)
			return
		}
		logger.InfoContext(ctx, "identified a library", "libraryId", libraryID,
			"listed", report.ListedCount, "extras", report.ExtraCount,
			"unidentified", report.UnidentifiedCount, "awaitingMatch", len(report.AwaitingMatch))
	})
	queue := opened.jobServices.queue
	askForAPassOnNewFiles(ingester, queue)
	ingester.SetOnPathsFound(func(ctx context.Context, nodeID int64, paths []string) {
		for _, path := range paths {
			queue.Unpark(ctx, jobs.RootDestination(nodeID, path)) // a root back is a root's probe (D231 §6)
		}
	})
	sink := reportSink{ingester: ingester, logger: logger}
	return nodes.NewService(nodes.ServiceParts{
		Database: opened.database, Clock: clock.System{}, Settings: opened.settings, Sink: sink, Destinations: queue,
		Logger: logger, PollHold: commandPollHold,
	})
}

// sweepRequester asks for sweeps to run soon: the job queue.
type sweepRequester interface {
	RequestSweeps(names ...string)
}

// askForAPassOnNewFiles has a batch of a scan that stored a new film ask for an identification
// pass while the scan goes on, so what is found early is playable early (D236 §1). The sweeper
// runs it within a minute, once however many batches asked.
func askForAPassOnNewFiles(ingester *library.Ingester, sweeps sweepRequester) {
	ingester.SetOnNewFiles(func(context.Context) { sweeps.RequestSweeps(sweepPasses) })
}

// reportSink stores what a node reports in persistent.db, then logs the scan's milestones.
type reportSink struct {
	ingester *library.Ingester
	logger   *slog.Logger
}

func (sink reportSink) IngestRecords(ctx context.Context, nodeID int64, records []nodeproto.Record) error {
	if err := sink.ingester.IngestRecords(ctx, nodeID, records); err != nil {
		return err
	}
	counts := map[nodeproto.RecordType]int{}
	for _, record := range records {
		counts[record.Type]++
		switch record.Type {
		case nodeproto.RecordScanBegin:
			sink.logger.InfoContext(ctx, "a node began a scan", "nodeId", nodeID, "rootPath", record.ScanBegin.RootPath)
		case nodeproto.RecordScanComplete:
			scanComplete := record.ScanComplete
			sink.logger.InfoContext(ctx, "a node finished a scan", "nodeId", nodeID, "rootPath", scanComplete.RootPath,
				"observedCount", scanComplete.ObservedCount, "sidecarCount", scanComplete.SidecarCount,
				"skippedCount", scanComplete.SkippedCount, "errorCount", scanComplete.ErrorCount,
				"isStoppedByErrors", scanComplete.IsStoppedByErrors)
		case nodeproto.RecordError:
			failure := record.Error
			sink.logger.WarnContext(ctx, "a node could not read something", "nodeId", nodeID,
				"path", failure.Path.Text, "scope", failure.Scope, "code", failure.Code,
				"occurrenceCount", failure.OccurrenceCount, "platformText", failure.PlatformError.Text)
		case nodeproto.RecordObserved, nodeproto.RecordSkip, nodeproto.RecordStatus, nodeproto.RecordMount,
			nodeproto.RecordSidecar:
		}
	}
	sink.logger.DebugContext(ctx, "a node reported", "nodeId", nodeID, "recordCount", len(records),
		"observedCount", counts[nodeproto.RecordObserved], "skippedCount", counts[nodeproto.RecordSkip])
	return nil
}

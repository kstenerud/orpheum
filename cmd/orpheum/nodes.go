// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// commandPollHold is how long a node's long-poll for commands is held when there is nothing to
// say. It is shorter than any idle timeout between two processes on one machine, and long
// enough that an idle node costs nothing.
const commandPollHold = 25 * time.Second

// selfStarter starts the node child: orpheum's own binary in node mode, with the same config
// file (D207). Its output joins the core's.
func selfStarter(config Config, output io.Writer) nodes.Starter {
	return func(ctx context.Context) (nodes.Process, error) {
		executable, err := os.Executable()
		if err != nil {
			return nil, err
		}
		child, err := execx.StartSelf(ctx, executable, []string{"node", "--config", config.path}, output)
		if err != nil {
			return nil, err
		}
		return child, nil
	}
}

// runNodeChannel serves the node channel on the core's socket and, when startNode is set,
// supervises the node child. The returned stop ends both: the child first, so its requests end,
// then the socket's server.
func runNodeChannel(
	ctx context.Context, config Config, opened openedServices, startNode nodes.Starter, logger *slog.Logger,
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
	supervisorContext, stopSupervising := context.WithCancel(ctx)
	if startNode != nil {
		ticker := time.NewTicker(time.Second)
		supervisor := nodes.NewSupervisor(startNode, service, config.Node.Name, clock.System{}, opened.settings, logger)
		running.Go(func() {
			defer ticker.Stop()
			if runErr := supervisor.Run(supervisorContext, ticker.C); runErr != nil {
				logger.ErrorContext(ctx, "the node supervisor stopped", "err", runErr)
			}
		})
	}
	return func() error {
		stopSupervising()
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

// reportSink stores what a node reports in library.db, then logs the scan's milestones.
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

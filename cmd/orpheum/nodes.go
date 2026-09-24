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

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
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

// runNodeChannel serves the node channel on the core's socket and, when start is set,
// supervises the node child. The returned stop ends both: the child first, so its requests end,
// then the socket's server.
func runNodeChannel(
	ctx context.Context, config Config, database *store.Database, settingsService *settings.Service,
	start nodes.Starter, logger *slog.Logger,
) (stop func() error, err error) {
	listener, err := datadir.ListenUnix(config.Server.SocketDir, coreSocket)
	if err != nil {
		return nil, err
	}
	sink := reportSink{ingester: library.NewIngester(database, clock.System{}, settingsService, logger), logger: logger}
	service := nodes.NewService(database, clock.System{}, settingsService, sink, logger, commandPollHold)
	server := &http.Server{Handler: service.Handler(), ReadHeaderTimeout: config.readHeaderTimeout()}
	var running sync.WaitGroup
	running.Go(func() {
		if serveErr := server.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the node channel stopped", "err", serveErr)
		}
	})
	supervising, stopSupervising := context.WithCancel(ctx)
	if start != nil {
		ticker := time.NewTicker(time.Second)
		supervisor := nodes.NewSupervisor(start, service, config.Node.Name, clock.System{}, settingsService, logger)
		running.Go(func() {
			defer ticker.Stop()
			if runErr := supervisor.Run(supervising, ticker.C); runErr != nil {
				logger.ErrorContext(ctx, "the node supervisor stopped", "err", runErr)
			}
		})
	}
	return func() error {
		stopSupervising()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.shutdownGrace())
		defer cancel()
		err := server.Shutdown(shutdown)
		if errors.Is(err, context.DeadlineExceeded) {
			err = server.Close() // a report stream that did not end in time
		}
		running.Wait()
		return err
	}, nil
}

// reportSink stores what a node reports in library.db, then logs the scan's milestones.
type reportSink struct {
	ingester *library.Ingester
	logger   *slog.Logger
}

func (sink reportSink) Records(ctx context.Context, nodeID int64, records []nodeproto.Record) error {
	if err := sink.ingester.Records(ctx, nodeID, records); err != nil {
		return err
	}
	counts := map[nodeproto.RecordType]int{}
	for _, record := range records {
		counts[record.Type]++
		switch record.Type {
		case nodeproto.RecordScanBegin:
			sink.logger.InfoContext(ctx, "a node began a scan", "nodeId", nodeID, "root", record.ScanBegin.Root)
		case nodeproto.RecordScanComplete:
			done := record.ScanComplete
			sink.logger.InfoContext(ctx, "a node finished a scan", "nodeId", nodeID, "root", done.Root,
				"observed", done.Observed, "skipped", done.Skipped, "errors", done.Errors, "stopped", done.Stopped)
		case nodeproto.RecordError:
			failure := record.Error
			sink.logger.WarnContext(ctx, "a node could not read something", "nodeId", nodeID,
				"path", failure.Path.Text, "scope", failure.Scope, "code", failure.Code, "count", failure.Count,
				"detail", failure.Raw.Text)
		case nodeproto.RecordObserved, nodeproto.RecordSkip, nodeproto.RecordStatus:
		}
	}
	sink.logger.DebugContext(ctx, "a node reported", "nodeId", nodeID, "records", len(records),
		"observed", counts[nodeproto.RecordObserved], "skipped", counts[nodeproto.RecordSkip])
	return nil
}

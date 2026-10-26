// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// runNodeChannel serves the node channel on the core's socket, which each node dials; the returned
// stop ends it. The core starts no node: each is a service of its own (D233 §2, D235 §4).
func runNodeChannel(
	ctx context.Context, config Config, opened openedServices, logger *slog.Logger,
) (stop func() error, err error) {
	service := opened.core.NodeChannel
	listener, err := unixsocket.Listen(config.Instance.RunDir, coreSocketName)
	if err != nil {
		return nil, err
	}
	handler, err := keepSocketKey(ctx, listener.Addr().String(), service.Handler(), logger)
	if err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: config.readHeaderTimeout()}
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

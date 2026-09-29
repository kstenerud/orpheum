// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/tmdb"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// newArtwork builds the artwork service (D212): it queues the images each pass and match leaves
// owing, fetches them from TMDB's image server, and takes the node's answers, which are the
// artwork store's one way in.
func newArtwork(
	ctx context.Context, opened started, directories *datadir.Directories, imageBase string, logger *slog.Logger,
) (*artwork.Service, error) {
	providerTimeoutSeconds, err := opened.settings.Int(ctx, settings.ProviderTimeoutS)
	if err != nil {
		return nil, err
	}
	httpClient, err := outbound.NewClient(time.Duration(providerTimeoutSeconds)*time.Second, version)
	if err != nil {
		return nil, err
	}
	service := artwork.NewService(artwork.Parts{
		Database: opened.database, Queue: opened.metadata.queue, Nodes: opened.nodes, ArtFiles: directories,
		Cache:    opened.cache,
		Settings: opened.settings,
		Fetchers: map[string]artwork.Fetcher{"tmdb": tmdb.NewImageFetcher(httpClient, imageBase)},
		Clock:    clock.System{}, Logger: logger,
	})
	if err := service.LoadHoldings(ctx); err != nil {
		return nil, err
	}
	opened.nodes.SetArtworkSink(service)
	opened.metadata.pass.AfterPass(func(ctx context.Context, report identify.Report) {
		service.QueueOwed(ctx, report.OwedArtwork)
	})
	return service, nil
}

// keepArtwork holds the artwork cache to its ceiling until ctx ends (D212 §6): a sweep when the
// server starts, daily, and whenever a write takes it past the ceiling; and when each image was
// served is written hourly and when it stops.
func keepArtwork(ctx context.Context, service *artwork.Service, logger *slog.Logger) {
	sweep := func() {
		if _, err := service.Sweep(ctx); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "the artwork cache sweep failed", "err", err)
		}
	}
	flush := func(ctx context.Context) {
		if err := service.Flush(ctx); err != nil {
			logger.ErrorContext(ctx, "could not write when artwork was served", "err", err)
		}
	}
	sweep()
	daily, hourly := time.NewTicker(24*time.Hour), time.NewTicker(time.Hour)
	defer daily.Stop()
	defer hourly.Stop()
	for {
		select {
		case <-ctx.Done():
			flush(context.WithoutCancel(ctx))
			return
		case <-service.SweepWakes():
			sweep()
		case <-daily.C:
			sweep()
		case <-hourly.C:
			flush(ctx)
		}
	}
}

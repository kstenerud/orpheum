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
	timeout, err := opened.settings.Int(ctx, settings.ProviderTimeoutS)
	if err != nil {
		return nil, err
	}
	httpClient, err := outbound.NewClient(time.Duration(timeout) * time.Second)
	if err != nil {
		return nil, err
	}
	service := artwork.NewService(artwork.Parts{
		Database: opened.database, Queue: opened.metadata.queue, Nodes: opened.nodes, Store: directories,
		Settings: opened.settings, Fetchers: map[string]artwork.Fetcher{"tmdb": tmdb.NewImages(httpClient, imageBase)},
		Clock: clock.System{}, Logger: logger,
	})
	opened.nodes.ReceiveArtwork(service)
	opened.metadata.pass.AfterPass(func(ctx context.Context, report identify.Report) {
		service.Owe(ctx, report.Artwork)
	})
	return service, nil
}

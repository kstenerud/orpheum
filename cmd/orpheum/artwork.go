// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// newArtwork builds the artwork service (D212): it takes the images each pass and match found,
// fetches what is owed from TMDB's image server, and takes the node's answers, which are the
// artwork store's one way in. The two packages never import each other: here identify's report
// is handed to the service, the service is what a refresh asks to look at images again, and the
// refresher is what the service asks for a recheck.
func newArtwork(
	ctx context.Context, opened openedServices, directories *datadir.Directories,
	imageBaseURL string, logger *slog.Logger,
) (*artwork.Service, error) {
	httpClient := opened.jobServices.outboundClient
	images := tmdb.NewImageFetcher(httpClient, imageBaseURL, opened.jobServices.providerServices.tmdbImages)
	service := artwork.NewService(artwork.Parts{
		Database: opened.database, Queue: opened.jobServices.queue, Rechecker: opened.jobServices.refresher,
		Nodes: opened.nodes, ArtFiles: directories,
		Cache:    opened.cache,
		Settings: opened.settings,
		Fetchers: map[string]artwork.Fetcher{"tmdb": images},
		Clock:    clock.System{}, Logger: logger,
	})
	if err := service.LoadHoldings(ctx); err != nil {
		return nil, err
	}
	opened.nodes.SetArtworkSink(service)
	opened.jobServices.pass.AfterPass(func(ctx context.Context, report identify.Report) {
		service.Take(ctx, foundImages(report))
	})
	opened.jobServices.matcher.SetImageRetrier(service)
	return service, nil
}

// foundImages is what a pass or a match found of items' images, as the artwork service takes it.
func foundImages(report identify.Report) artwork.Found {
	found := artwork.Found{Rematched: itemImages(report.Rematched), Named: itemImages(report.NamedImages)}
	if report.BesideTheMedia != nil {
		found.BesideTheMedia = &artwork.LocalImages{LibraryID: report.BesideTheMedia.LibraryID}
		for _, image := range report.BesideTheMedia.Images {
			found.BesideTheMedia.Images = append(found.BesideTheMedia.Images, artwork.LocalImage(image))
		}
	}
	return found
}

// itemImages are the images items' sources name, as the artwork service takes them.
func itemImages(named []identify.NamedImages) []artwork.ItemImages {
	images := make([]artwork.ItemImages, 0, len(named))
	for _, item := range named {
		refs := make([]artwork.Ref, 0, len(item.Refs))
		for _, ref := range item.Refs {
			refs = append(refs, artwork.Ref{Kind: ref.Kind, SourceRef: ref.Ref})
		}
		images = append(images, artwork.ItemImages{ItemID: item.ItemID, Source: item.Source, Refs: refs})
	}
	return images
}

// maintainArtworkCache holds the artwork cache to its ceiling until ctx ends (D212 §6): a sweep
// when the server starts, daily, and whenever a write takes it past the ceiling; and when each
// image was served is written hourly and when it stops.
func maintainArtworkCache(ctx context.Context, service *artwork.Service, logger *slog.Logger) {
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

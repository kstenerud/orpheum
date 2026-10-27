// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
)

// openArtwork builds the artwork service (D212): it takes the images each pass and match found,
// fetches what is owed from TMDB's image server, and takes the node's answers, which are the
// artwork store's one way in. The two packages never import each other: here identify's report
// is handed to the service, the service is what a refresh asks to look at images again, and the
// rechecks it says are due are what the refresh sweep queues (Services.Sweeps).
func (services *Services) openArtwork(ctx context.Context, parts Parts) error {
	images := tmdb.NewImageFetcher(services.OutboundClient, parts.TMDBImagesBaseURL,
		services.providerServices.tmdbImages)
	service := artwork.NewService(artwork.Parts{
		Database: parts.Database, Queue: services.Queue, Nodes: services.nodes,
		Exchanges: services.Exchanges, ArtFiles: parts.Directories,
		Cache:    parts.Cache,
		Settings: parts.Settings,
		Fetchers: map[string]artwork.Fetcher{"tmdb": images},
		Clock:    parts.Clock, Logger: parts.Logger,
	})
	if err := service.LoadHoldings(ctx); err != nil {
		return err
	}
	services.pass.AfterPass(func(ctx context.Context, report identify.Report) {
		service.Take(ctx, foundImages(report))
	})
	services.matcher.SetImageRetrier(service)
	services.artwork = service
	return nil
}

// foundImages is what a pass or a match found of items' images, as the artwork service takes it.
func foundImages(report identify.Report) artwork.Found {
	found := artwork.Found{
		Rematched: itemImages(report.Rematched), Named: itemImages(report.NamedImages),
		AwaitingMatch: report.AwaitingMatch, AwaitingListings: report.AwaitingListings,
	}
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

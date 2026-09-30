// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// cleaningProvider is a provider whose every answer is cleaned where it enters the system (principles
// §22, D211): a provider's text is community-edited, and one day a plugin's. The fetched bodies
// are bytes, stored as bytes, and cleaned when they are parsed, here.
type cleaningProvider struct {
	Provider
}

// cleanProviders wraps every provider so nothing reads a provider's answer uncleaned.
func cleanProviders(providers Providers) Providers {
	wrapped := make(Providers, len(providers))
	for name, provider := range providers {
		if _, already := provider.(cleaningProvider); already {
			wrapped[name] = provider
			continue
		}
		wrapped[name] = cleaningProvider{Provider: provider}
	}
	return wrapped
}

// Search cleans each candidate.
func (provider cleaningProvider) Search(ctx context.Context, query Query) ([]Candidate, error) {
	candidates, err := provider.Provider.Search(ctx, query)
	for index, candidate := range candidates {
		candidates[index] = Candidate{
			ID: textindex.Line(candidate.ID), Name: textindex.Line(candidate.Name),
			OriginalName: textindex.Line(candidate.OriginalName), Year: candidate.Year,
			ProviderPosterPath: textindex.Line(candidate.ProviderPosterPath),
		}
	}
	return candidates, err //nolint:wrapcheck // the provider's own error, passed through as it is
}

// Find cleans the id found.
func (provider cleaningProvider) Find(ctx context.Context, kind, source, id string) (string, error) {
	found, err := provider.Provider.Find(ctx, kind, source, id)
	return textindex.Line(found), err
}

// Parse cleans every text of the work.
func (provider cleaningProvider) Parse(kind string, body []byte) (Metadata, error) {
	metadata, err := provider.Provider.Parse(kind, body)
	ids := make(map[string]string, len(metadata.IDs))
	for source, id := range metadata.IDs {
		ids[textindex.Line(source)] = textindex.Line(id)
	}
	artwork := make([]ImageRef, 0, len(metadata.Artwork))
	for _, image := range metadata.Artwork {
		artwork = append(artwork, ImageRef{Kind: textindex.Line(image.Kind), Ref: textindex.Line(image.Ref)})
	}
	return Metadata{
		Artwork: artwork,
		ID:      textindex.Line(metadata.ID), Name: textindex.Line(metadata.Name),
		OriginalName: textindex.Line(metadata.OriginalName), OriginalLang: textindex.Line(metadata.OriginalLang),
		Overview: textindex.Prose(metadata.Overview), Tagline: textindex.Line(metadata.Tagline),
		Year: metadata.Year, RuntimeMs: metadata.RuntimeMs, IDs: ids,
	}, err //nolint:wrapcheck // the provider's own error, passed through as it is
}

// ParseSeason cleans every episode's text.
func (provider cleaningProvider) ParseSeason(body []byte) ([]Episode, error) {
	episodes, err := provider.Provider.ParseSeason(body)
	for index, episode := range episodes {
		episode.ID, episode.Name = textindex.Line(episode.ID), textindex.Line(episode.Name)
		episode.Overview, episode.StillRef = textindex.Prose(episode.Overview), textindex.Line(episode.StillRef)
		episodes[index] = episode
	}
	return episodes, err //nolint:wrapcheck // the provider's own error, passed through as it is
}

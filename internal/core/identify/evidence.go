// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"maps"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
)

// identificationAmbiguous is a title whose search found candidates and no single match (D172).
const identificationAmbiguous = "ambiguous"

// matchEvidence gives a spec what its title's match says: the stored response a person chose,
// over any NFO; and, for a spec with no NFO behind it, the stored response it was matched to, its
// ambiguity, or that its provider found nothing. A pass reads these and never calls the provider
// (D170, D172, D182).
func (filer *libraryFiler) matchEvidence(ctx context.Context, titleID int64, spec *titleSpec) error {
	if spec.kind == library.TitleKindEpisode || spec.kind == library.TitleKindOther {
		// An episode's evidence is its series' listing, read where it is filed (D209); an other
		// video is never matched (D349 section 10).
		return nil
	}
	matchRow, found, err := filer.titleMatchState(ctx, titleID)
	if err != nil || !found {
		return err
	}
	isChosen := matchRow.identificationState == identificationConfirmed && matchRow.followedSource != nil &&
		*matchRow.followedSource != sourceNFO &&
		*matchRow.followedSource != sourceNone
	if isChosen {
		return filer.fromResponse(ctx, titleID, *matchRow.followedSource, spec)
	}
	if spec.source != "" {
		return nil
	}
	switch {
	case matchRow.identificationState == identificationAmbiguous:
		spec.identification, spec.reason = identificationAmbiguous, ""
	case matchRow.followedSource == nil:
	case *matchRow.followedSource == sourceNFO:
		// The NFO it followed is gone, deleted or its video renamed from it: the path speaks for it
		// now. Read as a provider's answer, it failed the library's every pass (found by
		// TestPass_OverRandomLibrariesAgainWritesNothingAndMakesNoTitleEachPass).
	case *matchRow.followedSource == sourceNone:
		spec.source, spec.reason = sourceNone, noMatchReason(matchRow.metadataSource)
	default:
		return filer.fromResponse(ctx, titleID, *matchRow.followedSource, spec)
	}
	return nil
}

// maxStoredResponseBytes bounds a stored provider answer as it is read back: the provider clients
// read no more than 8 MiB of an answer, so nothing kept is larger.
const maxStoredResponseBytes = 8 << 20

// fromResponse takes a title's stored provider response as its source.
func (filer *libraryFiler) fromResponse(
	ctx context.Context, titleID int64, providerName string, spec *titleSpec,
) error {
	provider, known := filer.providers[providerName]
	if !known {
		return fmt.Errorf("title %d follows %s, which this server has no client for", titleID, providerName)
	}
	bodyGzip, err := filer.ownResponse(ctx, titleID, providerName)
	if err != nil {
		return fmt.Errorf("read title %d's %s response: %w", titleID, providerName, err)
	}
	body, err := bounded.Gunzip(bodyGzip, maxStoredResponseBytes)
	if err != nil {
		return fmt.Errorf("title %d's %s response: %w", titleID, providerName, err)
	}
	metadata, err := provider.Parse(spec.kind, body)
	if err != nil {
		return fmt.Errorf("title %d's %s response: %w", titleID, providerName, err)
	}
	spec.fromMetadata(providerName, metadata)
	return nil
}

// fromMetadata takes a provider's answer as the title's source.
func (spec *titleSpec) fromMetadata(provider string, metadata Metadata) {
	spec.name, spec.year, spec.releaseDate = metadata.Name, metadata.Year, metadata.ReleaseDate
	spec.overview, spec.tagline = metadata.Overview, metadata.Tagline
	if metadata.RuntimeMs > 0 {
		spec.runtimeMs = metadata.RuntimeMs
	}
	merged := maps.Clone(spec.ids)
	if merged == nil {
		merged = map[string]string{}
	}
	maps.Copy(merged, metadata.IDs)
	merged[provider] = metadata.ID
	spec.ids = merged
	spec.source, spec.identification, spec.reason = provider, identificationIdentified, ""
	spec.artwork = metadata.Artwork
}

// noMatchReason is why a film its provider found nothing for is unidentified.
func noMatchReason(metadataSource string) string {
	if metadataSource == "nfo_then_tmdb" {
		return "no NFO file, and TMDB found no single match"
	}
	return "TMDB found no single match"
}

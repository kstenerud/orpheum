// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// identificationAmbiguous is a title whose search found candidates and no single match (D172).
const identificationAmbiguous = "ambiguous"

// matchEvidence gives a spec what its title's match says: the stored response a person chose,
// over any NFO; and, for a spec with no NFO behind it, the stored response it was matched to, its
// ambiguity, or that its provider found nothing. A pass reads these and never calls the provider
// (D170, D172, D182).
func (filer *libraryFiler) matchEvidence(ctx context.Context, titleID int64, spec *titleSpec) error {
	if spec.kind == kindEpisode {
		return nil // an episode's evidence is its series' listing, read where it is filed (D209)
	}
	matchRow, err := store.OrNotFound(filer.queries.GetTitleForMatch(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read title %d's match: %w", titleID, err)
	}
	isChosen := matchRow.IdentificationState == identificationConfirmed && matchRow.FollowedSource != nil &&
		*matchRow.FollowedSource != sourceNFO &&
		*matchRow.FollowedSource != sourceNone
	if isChosen {
		return filer.fromResponse(ctx, titleID, *matchRow.FollowedSource, spec)
	}
	if spec.source != "" {
		return nil
	}
	switch {
	case matchRow.IdentificationState == identificationAmbiguous:
		spec.identification, spec.reason = identificationAmbiguous, ""
	case matchRow.FollowedSource == nil:
	case *matchRow.FollowedSource == sourceNone:
		spec.source, spec.reason = sourceNone, noMatchReason(matchRow.MetadataSource)
	default:
		return filer.fromResponse(ctx, titleID, *matchRow.FollowedSource, spec)
	}
	return nil
}

// fromResponse takes a title's stored provider response as its source.
func (filer *libraryFiler) fromResponse(
	ctx context.Context, titleID int64, providerName string, spec *titleSpec,
) error {
	provider, known := filer.providers[providerName]
	if !known {
		return fmt.Errorf("title %d follows %s, which this server has no client for", titleID, providerName)
	}
	bodyGzip, err := filer.queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		ItemID: titleID, Provider: providerName, Part: "",
	})
	if err != nil {
		return fmt.Errorf("read title %d's %s response: %w", titleID, providerName, err)
	}
	body, err := decompress(bodyGzip)
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
	spec.name, spec.year = metadata.Name, metadata.Year
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

func compress(body []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(body); err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	return buffer.Bytes(), nil
}

func decompress(bodyGzip []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(bodyGzip))
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	return body, nil
}

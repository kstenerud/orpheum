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

// identAmbiguous is a title whose search found candidates and no single match (D172).
const identAmbiguous = "ambiguous"

// matchEvidence gives a spec with no NFO behind it what its title's match says: the stored
// response it was matched to, its ambiguity, or that its provider found nothing. A pass reads
// these and never calls the provider (D170, D182).
func (work *run) matchEvidence(ctx context.Context, titleID int64, spec *titleSpec) error {
	match, err := store.One(work.queries.GetTitleForMatch(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read title %d's match: %w", titleID, err)
	}
	switch {
	case match.Ident == identAmbiguous:
		spec.ident, spec.reason = identAmbiguous, ""
	case match.Source == nil:
	case *match.Source == sourceNone:
		spec.source, spec.reason = sourceNone, noMatchReason(match.MetadataSource)
	default:
		return work.fromResponse(ctx, titleID, *match.Source, spec)
	}
	return nil
}

// fromResponse takes a title's stored provider response as its source.
func (work *run) fromResponse(ctx context.Context, titleID int64, providerName string, spec *titleSpec) error {
	provider, known := work.providers[providerName]
	if !known {
		return fmt.Errorf("title %d follows %s, which this server has no client for", titleID, providerName)
	}
	stored, err := work.queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		TitleID: titleID, Provider: providerName,
	})
	if err != nil {
		return fmt.Errorf("read title %d's %s response: %w", titleID, providerName, err)
	}
	body, err := decompress(stored)
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
	spec.source, spec.ident, spec.reason = provider, identIdentified, ""
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

func decompress(stored []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(stored))
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	return body, nil
}

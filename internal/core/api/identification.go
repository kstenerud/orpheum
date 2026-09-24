// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
)

const tagIdentification = "Identification"

// ItemBody is one title or series in a list.
type ItemBody struct {
	ID        int64  `json:"id"`
	LibraryID int64  `json:"libraryId"`
	Kind      string `enum:"movie,episode,extra,series" json:"kind"`
	Name      string `json:"name"`
	Year      int64  `json:"year,omitempty"`
	Ident     string `enum:"identified,ambiguous,unidentified,confirmed" json:"ident"`
	Source    string `doc:"Where its own values come from: nfo, tmdb, or none" json:"source,omitempty"`
	SourceID  string `doc:"Its id at its provider" json:"sourceId,omitempty"`
}

// ItemsInput asks for the titles and series in one state of identification.
type ItemsInput struct {
	AuthorizationHeader
	Ident string `enum:"identified,ambiguous,unidentified,confirmed" query:"ident" required:"true"`
}

// ItemsOutput lists titles and series.
type ItemsOutput struct {
	Body struct {
		Items []ItemBody `json:"items"`
	}
}

// ItemInput names one title or series.
type ItemInput struct {
	AuthorizationHeader
	ID int64 `path:"id"`
}

// CandidateBody is one search result a person may choose (D172).
type CandidateBody struct {
	ProviderID   string `json:"providerId"`
	Name         string `json:"name"`
	OriginalName string `json:"originalName,omitempty"`
	Year         int    `json:"year,omitempty"`
	PosterRef    string `doc:"The provider's path for the poster" json:"posterRef,omitempty"`
}

// CandidatesOutput is a film's or series' candidates, best first.
type CandidatesOutput struct {
	Body struct {
		Provider   string          `json:"provider,omitempty"`
		Candidates []CandidateBody `json:"candidates"`
	}
}

// MatchInput is a person's choice of match.
type MatchInput struct {
	AuthorizationHeader
	ID   int64 `path:"id"`
	Body struct {
		Provider   string `example:"tmdb" json:"provider"`
		ProviderID string `json:"providerId"      minLength:"1"`
	}
}

// OverrideInput names one field of an item.
type OverrideInput struct {
	AuthorizationHeader
	ID    int64  `path:"id"`
	Field string `enum:"name,overview,tagline,year,runtime" path:"field"`
}

// PutOverrideInput sets a field to a person's value.
type PutOverrideInput struct {
	AuthorizationHeader
	ID    int64  `path:"id"`
	Field string `enum:"name,overview,tagline,year,runtime" path:"field"`
	Body  struct {
		Value string `doc:"The value; year and runtime (milliseconds) are whole numbers" json:"value"`
	}
}

func (server *Server) registerIdentification() {
	tags := []string{tagIdentification}
	huma.Register(server.api, huma.Operation{
		OperationID: "getItems", Method: http.MethodGet, Path: "/items",
		Summary: "The titles and series in one state of identification", Tags: tags,
	}, server.getItems)
	huma.Register(server.api, huma.Operation{
		OperationID: "getItemCandidates", Method: http.MethodGet, Path: "/items/{id}/candidates",
		Summary: "An ambiguous film's or series' search results, to choose from (edit_metadata)", Tags: tags,
	}, server.getCandidates)
	huma.Register(server.api, huma.Operation{
		OperationID: "postItemMatch", Method: http.MethodPost, Path: "/items/{id}/match",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Choose a film's or series' match; permanent (edit_metadata)",
	}, server.postMatch)
	huma.Register(server.api, huma.Operation{
		OperationID: "putOverride", Method: http.MethodPut, Path: "/items/{id}/overrides/{field}",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Set a field to a person's value, kept over every source (edit_metadata)",
	}, server.putOverride)
	huma.Register(server.api, huma.Operation{
		OperationID: "deleteOverride", Method: http.MethodDelete, Path: "/items/{id}/overrides/{field}",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Remove a field's override, so it follows its source again (edit_metadata)",
	}, server.deleteOverride)
}

func (server *Server) getItems(ctx context.Context, input *ItemsInput) (*ItemsOutput, error) {
	dependencies, _, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	items, err := dependencies.Identify.Items(ctx, input.Ident)
	if err != nil {
		return nil, server.internal(ctx, "list items", err)
	}
	output := &ItemsOutput{}
	output.Body.Items = make([]ItemBody, 0, len(items))
	for _, item := range items {
		output.Body.Items = append(output.Body.Items, ItemBody{
			ID: item.ID, LibraryID: item.LibraryID, Kind: item.Kind, Name: item.Name, Year: item.Year,
			Ident: item.Ident, Source: item.Source, SourceID: item.SourceID,
		})
	}
	return output, nil
}

func (server *Server) getCandidates(ctx context.Context, input *ItemInput) (*CandidatesOutput, error) {
	dependencies, _, err := server.metadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	candidates, provider, err := dependencies.Identify.Candidates(ctx, input.ID)
	if err != nil {
		return nil, server.identifyError(ctx, "list candidates", err)
	}
	output := &CandidatesOutput{}
	output.Body.Provider = provider
	output.Body.Candidates = make([]CandidateBody, 0, len(candidates))
	for _, candidate := range candidates {
		output.Body.Candidates = append(output.Body.Candidates, CandidateBody{
			ProviderID: candidate.ID, Name: candidate.Name, OriginalName: candidate.OriginalName,
			Year: candidate.Year, PosterRef: candidate.PosterRef,
		})
	}
	return output, nil
}

func (server *Server) postMatch(ctx context.Context, input *MatchInput) (*struct{}, error) {
	dependencies, _, err := server.metadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Identify.Choose(ctx, input.ID, input.Body.Provider, input.Body.ProviderID)
	return &struct{}{}, server.identifyError(ctx, "choose a match", err)
}

func (server *Server) putOverride(ctx context.Context, input *PutOverrideInput) (*struct{}, error) {
	dependencies, principal, err := server.metadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Identify.SetOverride(ctx, input.ID, input.Field, input.Body.Value, principal.UserID)
	return &struct{}{}, server.identifyError(ctx, "set an override", err)
}

func (server *Server) deleteOverride(ctx context.Context, input *OverrideInput) (*struct{}, error) {
	dependencies, _, err := server.metadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Identify.ClearOverride(ctx, input.ID, input.Field)
	return &struct{}{}, server.identifyError(ctx, "remove an override", err)
}

// metadataEditor is signedIn for a role that may edit metadata.
func (server *Server) metadataEditor(ctx context.Context, authorization string) (
	*Dependencies, auth.Principal, error,
) {
	dependencies, principal, err := server.signedIn(ctx, authorization)
	if err != nil {
		return nil, principal, err
	}
	if !principal.CanEditMetadata() {
		return nil, principal, newError(http.StatusForbidden, CodeForbidden, "your role may not edit metadata")
	}
	return dependencies, principal, nil
}

// identifyError maps identification's refusals and a provider's to API errors.
func (server *Server) identifyError(ctx context.Context, doing string, err error) error {
	limited, isLimited := errors.AsType[*identify.RateLimitedError](err)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, identify.ErrNoItem):
		return newError(http.StatusNotFound, CodeNotFound, "no such title or series")
	case errors.Is(err, identify.ErrNoOverride):
		return newError(http.StatusNotFound, CodeNotFound, "the field has no override")
	case errors.Is(err, identify.ErrUnknownProvider), errors.Is(err, identify.ErrUnknownField),
		errors.Is(err, identify.ErrBadValue), errors.Is(err, identify.ErrNotChoosable):
		return newError(http.StatusUnprocessableEntity, CodeInvalidRequest, err.Error())
	case errors.Is(err, identify.ErrNoMatch):
		return newError(http.StatusUnprocessableEntity, CodeInvalidRequest, "the provider has no such work")
	case isLimited:
		failure := newError(http.StatusServiceUnavailable, CodeRateLimited, "the provider asks us to wait")
		failure.ErrorBody.Error.Detail = map[string]string{"retryAfter": limited.After.String()}
		return failure
	case errors.Is(err, identify.ErrNoKey), errors.Is(err, identify.ErrKeyRefused):
		return newError(http.StatusServiceUnavailable, CodeProviderError,
			"the provider's key is not set or was refused; an admin can set it under secrets")
	}
	return server.internal(ctx, doing, err)
}

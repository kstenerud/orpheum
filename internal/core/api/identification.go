// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

const tagIdentification = "Identification"

// ItemBody is one title or series in a list.
type ItemBody struct {
	ID                  int64  `json:"id"`
	LibraryID           int64  `json:"libraryId"`
	Kind                string `enum:"movie,episode,extra,series" json:"kind"`
	Name                string `json:"name"`
	Year                int64  `json:"year,omitempty"`
	IdentificationState string `enum:"identified,ambiguous,unidentified,confirmed" json:"identificationState"`
	FollowedSource      string `doc:"Where its own values come from: nfo, tmdb, or none" json:"followedSource,omitempty"` //nolint:lll // one struct tag
	FollowedSourceID    string `doc:"Its id at its provider" json:"followedSourceId,omitempty"`
}

// ItemsInput asks for the titles and series in one state of identification.
type ItemsInput struct {
	AuthorizationHeader
	IdentificationState string `enum:"identified,ambiguous,unidentified,confirmed" query:"identificationState" required:"true"` //nolint:lll // one struct tag
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
	ProviderID         string `json:"providerId"`
	Name               string `json:"name"`
	OriginalName       string `json:"originalName,omitempty"`
	Year               int    `json:"year,omitempty"`
	ProviderPosterPath string `doc:"The provider's path for the poster" json:"providerPosterPath,omitempty"`
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
	Field string `enum:"name,overview,tagline,year,runtime_ms" path:"field"`
}

// PutOverrideInput sets a field to a person's value.
type PutOverrideInput struct {
	AuthorizationHeader
	ID    int64  `path:"id"`
	Field string `enum:"name,overview,tagline,year,runtime_ms" path:"field"`
	Body  struct {
		Value string `doc:"The value; year and runtime_ms are whole numbers" json:"value"`
	}
}

// RefreshOutput says whether a refresh was queued, or one was already waiting.
type RefreshOutput struct {
	Status int
	Body   struct {
		IsQueued bool `doc:"False when a fetch for it was already waiting" json:"isQueued"`
	}
}

func (server *Server) registerIdentification() {
	tags := []string{tagIdentification}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getItemsByIdentification", Method: http.MethodGet, Path: "/items",
		Summary: "The titles and series in one state of identification", Tags: tags,
	}, server.getItemsByIdentification)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getItemCandidates", Method: http.MethodGet, Path: "/items/{id}/candidates",
		Summary: "An ambiguous film's or series' search results, to choose from (edit_metadata)", Tags: tags,
	}, server.getItemCandidates)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "postItemMatch", Method: http.MethodPost, Path: "/items/{id}/match",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Choose a film's or series' match; permanent (edit_metadata)",
	}, server.postItemMatch)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "putOverride", Method: http.MethodPut, Path: "/items/{id}/overrides/{field}",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Set a field to a person's value, kept over every source (edit_metadata)",
	}, server.putOverride)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "deleteOverride", Method: http.MethodDelete, Path: "/items/{id}/overrides/{field}",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Remove a field's override, so it follows its source again (edit_metadata)",
	}, server.deleteOverride)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "postItemRefresh", Method: http.MethodPost, Path: "/items/{id}/refresh",
		DefaultStatus: http.StatusAccepted, Tags: tags,
		Summary: "Fetch a film's or series' data and its missing images again, whenever a person likes " +
			"(edit_metadata; D213)",
	}, server.postItemRefresh)
}

func (server *Server) getItemsByIdentification(ctx context.Context, input *ItemsInput) (*ItemsOutput, error) {
	dependencies, _, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	items, err := dependencies.Chooser.Items(ctx, input.IdentificationState)
	if err != nil {
		return nil, server.identifyError(ctx, "list items", err)
	}
	output := &ItemsOutput{}
	output.Body.Items = make([]ItemBody, 0, len(items))
	for _, item := range items {
		output.Body.Items = append(output.Body.Items, ItemBody{
			ID: item.ID, LibraryID: item.LibraryID, Kind: item.Kind, Name: item.Name, Year: item.Year,
			IdentificationState: item.IdentificationState,
			FollowedSource:      item.FollowedSource, FollowedSourceID: item.FollowedSourceID,
		})
	}
	return output, nil
}

func (server *Server) getItemCandidates(ctx context.Context, input *ItemInput) (*CandidatesOutput, error) {
	dependencies, _, err := server.requireMetadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	candidates, provider, err := dependencies.Chooser.Candidates(ctx, input.ID)
	if err != nil {
		return nil, server.identifyError(ctx, "list candidates", err)
	}
	output := &CandidatesOutput{}
	output.Body.Provider = provider
	output.Body.Candidates = make([]CandidateBody, 0, len(candidates))
	for _, candidate := range candidates {
		output.Body.Candidates = append(output.Body.Candidates, CandidateBody{
			ProviderID: candidate.ID, Name: candidate.Name, OriginalName: candidate.OriginalName,
			Year: candidate.Year, ProviderPosterPath: candidate.ProviderPosterPath,
		})
	}
	return output, nil
}

func (server *Server) postItemMatch(ctx context.Context, input *MatchInput) (*struct{}, error) {
	dependencies, _, err := server.requireMetadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Chooser.Choose(ctx, input.ID, input.Body.Provider, input.Body.ProviderID)
	return &struct{}{}, server.identifyError(ctx, "choose a match", err)
}

func (server *Server) postItemRefresh(ctx context.Context, input *ItemInput) (*RefreshOutput, error) {
	dependencies, _, err := server.requireMetadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isQueued, err := dependencies.Refresher.Request(ctx, input.ID)
	if err != nil {
		return nil, server.identifyError(ctx, "queue a refresh", err)
	}
	output := &RefreshOutput{Status: http.StatusAccepted}
	output.Body.IsQueued = isQueued
	return output, nil
}

func (server *Server) putOverride(ctx context.Context, input *PutOverrideInput) (*struct{}, error) {
	dependencies, principal, err := server.requireMetadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Chooser.SetOverride(ctx, input.ID, input.Field, input.Body.Value, principal.UserID)
	return &struct{}{}, server.identifyError(ctx, "set an override", err)
}

func (server *Server) deleteOverride(ctx context.Context, input *OverrideInput) (*struct{}, error) {
	dependencies, _, err := server.requireMetadataEditor(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Chooser.ClearOverride(ctx, input.ID, input.Field)
	return &struct{}{}, server.identifyError(ctx, "remove an override", err)
}

// requireMetadataEditor is requireSignedIn for a role that may edit metadata.
func (server *Server) requireMetadataEditor(ctx context.Context, authorization string) (
	*Dependencies, auth.Principal, error,
) {
	dependencies, principal, err := server.requireSignedIn(ctx, authorization)
	if err != nil {
		return nil, principal, err
	}
	if !principal.CanEditMetadata() {
		return nil, principal, newError(http.StatusForbidden, CodeForbidden, "your role may not edit metadata")
	}
	return dependencies, principal, nil
}

// identifyError maps identification's refusals and a provider's to API errors.
func (server *Server) identifyError(ctx context.Context, action string, err error) error {
	limited, isLimited := errors.AsType[*outbound.RateLimitedError](err)
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
		return providerRateLimited(limited)
	case errors.Is(err, identify.ErrNoKey):
		return providerRefusal(ReasonNoKey,
			"the metadata provider has no key set; an admin can set it in Settings")
	case errors.Is(err, identify.ErrKeyRefused):
		return providerRefusal(ReasonKeyRefused,
			"the metadata provider refused its key; an admin can change it in Settings")
	case errors.Is(err, outbound.ErrUnavailable):
		return providerRefusal(ReasonUnavailable, "the metadata provider is not answering; try again later")
	}
	return server.logInternalError(ctx, action, err)
}

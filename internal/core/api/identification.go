// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

const tagIdentification = "Identification"

// ItemBody is one title or series in a list.
type ItemBody struct {
	ID                  int64             `json:"id"`
	LibraryID           int64             `json:"libraryId"`
	Kind                string            `enum:"movie,episode,extra,series" json:"kind"`
	Name                string            `json:"name"`
	Year                int64             `json:"year,omitempty"`
	IdentificationState string            `enum:"identified,ambiguous,unidentified,confirmed" json:"identificationState"`            //nolint:lll // one struct tag
	FollowedSource      string            `doc:"Where its own values come from: nfo, tmdb, or none" json:"followedSource,omitempty"` //nolint:lll // one struct tag
	FollowedSourceID    string            `doc:"Its id at its provider" json:"followedSourceId,omitempty"`
	FetchFailure        *FetchFailureBody `json:"fetchFailure,omitempty"`
	RefreshRequestedAt  int64             `doc:"When a person asked for it to be refreshed, while no fetch has answered yet" json:"refreshRequestedAt,omitempty"` //nolint:lll // one struct tag
}

// FetchFailureBody is a film's or series' own failure to be fetched, not its provider's being
// down (D231 §4): its page shows it, and a refresh, "try again", clears it.
type FetchFailureBody struct {
	Failure  string `doc:"What the provider or the job said" json:"failure"`
	FailedAt int64  `json:"failedAt"`
}

// fetchFailureBody is a failure as a row holds it, or nil.
func fetchFailureBody(failure *string, failedAt *int64) *FetchFailureBody {
	if failure == nil {
		return nil
	}
	return &FetchFailureBody{Failure: *failure, FailedAt: ptr.ValueOrZero(failedAt)}
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
		// WaitingFor says, while TMDB is not answering, what the refresh waits for and since when;
		// absent when it is answering (D231 §12).
		WaitingFor *DestinationBody `json:"waitingFor,omitempty"`
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
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	items, err := dependencies.Chooser.Items(ctx, principal.Reach, input.IdentificationState)
	if err != nil {
		return nil, server.identifyError(ctx, "list items", err)
	}
	output := &ItemsOutput{}
	output.Body.Items = make([]ItemBody, 0, len(items))
	for _, item := range items {
		body := ItemBody{
			ID: item.ID, LibraryID: item.LibraryID, Kind: item.Kind, Name: item.Name, Year: item.Year,
			IdentificationState: item.IdentificationState,
			FollowedSource:      item.FollowedSource, FollowedSourceID: item.FollowedSourceID,
			RefreshRequestedAt: unixMillisOrZero(item.RefreshRequestedAt),
		}
		if failure := item.FetchFailure; failure != nil {
			body.FetchFailure = &FetchFailureBody{Failure: failure.Failure, FailedAt: failure.FailedAt.UnixMilli()}
		}
		output.Body.Items = append(output.Body.Items, body)
	}
	return output, nil
}

func (server *Server) getItemCandidates(ctx context.Context, input *ItemInput) (*CandidatesOutput, error) {
	dependencies, _, err := server.requireMetadataEditorReaching(ctx, input.Authorization, input.ID)
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
	dependencies, _, err := server.requireMetadataEditorReaching(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	err = dependencies.Chooser.Choose(ctx, input.ID, input.Body.Provider, input.Body.ProviderID)
	return &struct{}{}, server.identifyError(ctx, "choose a match", err)
}

func (server *Server) postItemRefresh(ctx context.Context, input *ItemInput) (*RefreshOutput, error) {
	dependencies, _, err := server.requireMetadataEditorReaching(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	isQueued, err := dependencies.Refresher.Request(ctx, input.ID)
	if err != nil {
		return nil, server.identifyError(ctx, "queue a refresh", err)
	}
	output := &RefreshOutput{Status: http.StatusAccepted}
	output.Body.IsQueued = isQueued
	if parked, isParked := dependencies.Jobs.DestinationState(jobs.DestinationTMDBAPI); isParked {
		waiting := destinationBody(parked, server.nodeNamer(ctx, dependencies))
		output.Body.WaitingFor = &waiting
	}
	return output, nil
}

func (server *Server) putOverride(ctx context.Context, input *PutOverrideInput) (*struct{}, error) {
	dependencies, principal, err := server.requireMetadataEditorReaching(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	err = dependencies.Chooser.SetOverride(ctx, input.ID, input.Field, input.Body.Value, principal.UserID)
	return &struct{}{}, server.identifyError(ctx, "set an override", err)
}

func (server *Server) deleteOverride(ctx context.Context, input *OverrideInput) (*struct{}, error) {
	dependencies, _, err := server.requireMetadataEditorReaching(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	err = dependencies.Chooser.ClearOverride(ctx, input.ID, input.Field)
	return &struct{}{}, server.identifyError(ctx, "remove an override", err)
}

// requireMetadataEditorReaching is requireMetadataEditor for a request about one title or series,
// in a library the editor reaches; one that is not is answered as an item that does not exist
// (D243 §4).
func (server *Server) requireMetadataEditorReaching(ctx context.Context, authorization string, itemID int64) (
	*Dependencies, auth.Principal, error,
) {
	dependencies, principal, err := server.requireMetadataEditor(ctx, authorization)
	if err != nil {
		return nil, principal, err
	}
	if err = dependencies.Browser.ReachItem(ctx, principal.Reach, itemID); err != nil {
		return nil, principal, server.identifyError(ctx, "reach an item", err)
	}
	return dependencies, principal, nil
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
	case errors.Is(err, identify.ErrNoItem), errors.Is(err, library.ErrNoItem): // one not reached too
		return newError(http.StatusNotFound, CodeNotFound, "no such title or series")
	case errors.Is(err, identify.ErrNoOverride):
		return newError(http.StatusNotFound, CodeNotFound, "the field has no override")
	case errors.Is(err, identify.ErrUnknownProvider): // a chosen match names its provider
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.provider", err.Error())
	case errors.Is(err, identify.ErrUnknownField): // an override names its field in the path
		return invalidRequestAt(http.StatusUnprocessableEntity, "path.field", err.Error())
	case errors.Is(err, identify.ErrBadValue):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.value", err.Error())
	case errors.Is(err, identify.ErrNotChoosable): // an episode, named in the path
		return invalidRequestAt(http.StatusUnprocessableEntity, "path.id", err.Error())
	case errors.Is(err, identify.ErrNoMatch):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.providerId", "the provider has no such work")
	case isLimited:
		return providerRateLimited(limited)
	case errors.Is(err, identify.ErrNoKey):
		return providerRefusal(ReasonNoKey,
			"the metadata provider has no key set; an admin can set it in Settings")
	case errors.Is(err, identify.ErrKeyRefused):
		return providerRefusal(ReasonKeyRefused,
			"the metadata provider refused its key; an admin can change it in Settings")
	case errors.Is(err, outbound.ErrUnavailable):
		return server.providerNotAnswering(ctx, jobs.DestinationTMDBAPI,
			"the metadata provider is not answering; try again later")
	}
	return server.logInternalError(ctx, action, err)
}

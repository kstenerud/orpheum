// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/identify"
)

// RescanStartedOutput says whether a rescan started, or one of the same scope already runs.
type RescanStartedOutput struct {
	Body struct {
		// IsStarted is false while a rescan of the same scope, or of the library it is in, runs: a
		// second press adds nothing (D357 §5).
		IsStarted bool `json:"isStarted"`
	}
}

// RescansStartedOutput says how many libraries' rescans started.
type RescansStartedOutput struct {
	Body struct {
		// StartedCount is how many libraries of films or series started a rescan; one already
		// being rescanned starts none.
		StartedCount int `json:"startedCount"`
	}
}

// LibraryRescanBody is how far a library's rescans are (D357 §5): scanning, then matching its films
// and series again.
type LibraryRescanBody struct {
	IsScanning   bool  `doc:"Whether a rescan of it waits for its scan before it lists its films and series" json:"isScanning"` //nolint:lll // one struct tag
	ItemCount    int64 `doc:"The films and series its rescans listed to match again" json:"itemCount"`
	MatchedCount int64 `doc:"Of them, those matched again" json:"matchedCount"`
}

func (server *Server) registerRescans() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "postLibrariesRescan", Method: http.MethodPost, Path: "/libraries/rescan",
		DefaultStatus: http.StatusAccepted, Tags: []string{tagLibraries},
		Summary: "Rescan every library of films or of series: scan it, then match each film and series " +
			"in it again from its files, leaving alone what a person set (admin; D357)",
	}, server.postLibrariesRescan)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "postLibraryRescan", Method: http.MethodPost, Path: "/libraries/{id}/rescan",
		DefaultStatus: http.StatusAccepted, Tags: []string{tagLibraries},
		Summary: "Rescan a library of films or of series: scan it, then match each film and series in it " +
			"again from its files, leaving alone what a person set (admin; D357)",
	}, server.postLibraryRescan)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "postItemRescan", Method: http.MethodPost, Path: "/items/{id}/rescan",
		DefaultStatus: http.StatusAccepted, Tags: []string{tagIdentification},
		Summary: "Rescan a film or series: scan its files, then match it again from them, unless a person " +
			"chose its match (admin; D357)",
	}, server.postItemRescan)
}

func (server *Server) postLibrariesRescan(ctx context.Context, input *AuthorizationHeader) (
	*RescansStartedOutput, error,
) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	started, err := dependencies.Rescans.RescanEverything(ctx)
	if err != nil {
		return nil, server.rescanError(ctx, "rescan every library", err)
	}
	output := &RescansStartedOutput{}
	output.Body.StartedCount = started
	return output, nil
}

func (server *Server) postLibraryRescan(ctx context.Context, input *LibraryInput) (*RescanStartedOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isStarted, err := dependencies.Rescans.RescanLibrary(ctx, input.ID)
	if err != nil {
		return nil, server.rescanError(ctx, "rescan a library", err)
	}
	output := &RescanStartedOutput{}
	output.Body.IsStarted = isStarted
	return output, nil
}

func (server *Server) postItemRescan(ctx context.Context, input *ItemInput) (*RescanStartedOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isStarted, err := dependencies.Rescans.RescanItem(ctx, input.ID)
	if err != nil {
		return nil, server.rescanError(ctx, "rescan an item", err)
	}
	output := &RescanStartedOutput{}
	output.Body.IsStarted = isStarted
	return output, nil
}

// rescanError maps a rescan's refusals to API errors: a library or item it does not take, and the
// library's and identification's own.
func (server *Server) rescanError(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, identify.ErrNotRescannable), errors.Is(err, identify.ErrNotAWork):
		return invalidRequestAt(http.StatusUnprocessableEntity, "path.id", err.Error())
	case errors.Is(err, identify.ErrNoItem):
		return server.identifyError(ctx, action, err)
	}
	return server.libraryError(ctx, action, err)
}

// rescanBodyOf is a library's rescans in progress as the API says them, or nil for none.
func rescanBodyOf(progress map[int64]identify.RescanProgress, libraryID int64) *LibraryRescanBody {
	held, isRescanning := progress[libraryID]
	if !isRescanning {
		return nil
	}
	return &LibraryRescanBody{
		IsScanning: held.IsScanning, ItemCount: held.ItemCount, MatchedCount: held.MatchedCount,
	}
}

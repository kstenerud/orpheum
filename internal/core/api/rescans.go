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
		// StartedCount is how many libraries started a rescan; one already being rescanned starts
		// none.
		StartedCount int `json:"startedCount"`
	}
}

// A rescan is of one kind, D392's full one (D535 §5): every file in its scope read again, changed on
// disk or not, and what the differences touch thrown out, before its match; the light rescan of D357
// and its full flag are gone.

// DiscardInput is a rescan's choice to throw out everything the app made or fetched for its scope,
// whatever differs, rather than what the differences touch alone (D535 §6). Only an admin rescans.
type DiscardInput struct {
	IsDiscarding bool `doc:"Throw out everything made or fetched for its files, whatever differs; never what a person did" query:"discard"` //nolint:lll // one struct tag
}

// RescanEverythingInput asks for a rescan of every library (D535 §4).
type RescanEverythingInput struct {
	AuthorizationHeader
	DiscardInput
}

// LibraryRescanInput asks for a rescan of a library.
type LibraryRescanInput struct {
	LibraryInput
	DiscardInput
}

// ItemRescanInput asks for a rescan of a film, a series, an episode or an other video (D535 §3).
type ItemRescanInput struct {
	ItemInput
	DiscardInput
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
		OperationID: "rescanLibraries", Method: http.MethodPost, Path: "/libraries/rescan",
		DefaultStatus: http.StatusAccepted, Tags: []string{tagLibraries},
		Summary: "Rescan every library, whatever it holds (D535 §4, §5): read every file again, changed or " +
			"not, throwing out what its differences touch (D392), then match each film and series again " +
			"from its files, leaving alone what a person set (D357)",
		Extensions: callableBy(capabilityAdmin),
	}, server.rescanLibraries)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "rescanLibrary", Method: http.MethodPost, Path: "/libraries/{id}/rescan",
		DefaultStatus: http.StatusAccepted, Tags: []string{tagLibraries},
		Summary: "Rescan a library: read every file again, changed or not, throwing out what its " +
			"differences touch (D392, D535 §5), then match each film and series in it again from its " +
			"files, leaving alone what a person set (D357); a library of other videos has nothing matched " +
			"(D535 §4)",
		Extensions: callableBy(capabilityAdmin),
	}, server.rescanLibrary)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "rescanItem", Method: http.MethodPost, Path: "/items/{id}/rescan",
		DefaultStatus: http.StatusAccepted, Tags: []string{tagIdentification},
		Summary: "Rescan a film, series, episode or other video: read its files again, a series' every " +
			"episode's, throwing out what their differences touch (D392, D535 §3, §5), then match a film " +
			"or series again from them, unless a person chose its match (D357); an extra is refused",
		Extensions: callableBy(capabilityAdmin),
	}, server.rescanItem)
}

func (server *Server) rescanLibraries(ctx context.Context, input *RescanEverythingInput) (
	*RescansStartedOutput, error,
) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	started, err := dependencies.Rescans.RescanEverything(ctx, input.IsDiscarding)
	if err != nil {
		return nil, server.rescanError(ctx, "rescan every library", err)
	}
	output := &RescansStartedOutput{}
	output.Body.StartedCount = started
	return output, nil
}

func (server *Server) rescanLibrary(ctx context.Context, input *LibraryRescanInput) (
	*RescanStartedOutput, error,
) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isStarted, err := dependencies.Rescans.RescanLibrary(ctx, input.ID, input.IsDiscarding)
	if err != nil {
		return nil, server.rescanError(ctx, "rescan a library", err)
	}
	output := &RescanStartedOutput{}
	output.Body.IsStarted = isStarted
	return output, nil
}

func (server *Server) rescanItem(ctx context.Context, input *ItemRescanInput) (*RescanStartedOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isStarted, err := dependencies.Rescans.RescanItem(ctx, input.ID, input.IsDiscarding)
	if err != nil {
		return nil, server.rescanError(ctx, "rescan an item", err)
	}
	output := &RescanStartedOutput{}
	output.Body.IsStarted = isStarted
	return output, nil
}

// rescanError maps a rescan's refusals to API errors: an item it does not take, an extra, and the
// library's and identification's own.
func (server *Server) rescanError(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, identify.ErrNotRescannable):
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

// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// RepointInput is a person saying folder Path on node NodeName is now folder NewPath on node NewNodeName
// (D429 §2; repoint-library.md, "The spec").
type RepointInput struct {
	AuthorizationHeader
	Body struct {
		NodeName    string `doc:"The node the folder was on" json:"nodeName" minLength:"1"`
		Path        string `doc:"The folder, by a clean absolute path: any folder, at any level" json:"path" minLength:"1"` //nolint:lll // one struct tag
		NewNodeName string `doc:"The node it is on now: the same, or another" json:"newNodeName" minLength:"1"`
		NewPath     string `doc:"Where it is now: one of that node's configured roots, or a folder inside one" json:"newPath" minLength:"1"` //nolint:lll // one struct tag
	}
}

// RepointMovedPathBody is a place's path now and after the move.
type RepointMovedPathBody struct {
	Path    string `json:"path"`
	NewPath string `json:"newPath"`
}

// RepointRootBody is a library's folder a repoint moves with the folder.
type RepointRootBody struct {
	LibraryID   int64  `json:"libraryId"`
	LibraryName string `json:"libraryName"`
	Path        string `json:"path"`
	NewPath     string `json:"newPath"`
	IsLive      bool   `doc:"Whether the folder is in use; a retired one moves too" json:"isLive"`
}

// RepointLibraryBody is how many live files a library lists now and would not after the move, and
// the reverse.
type RepointLibraryBody struct {
	LibraryID   int64  `json:"libraryId"`
	LibraryName string `json:"libraryName"`
	LosesCount  int    `json:"losesCount"`
	GainsCount  int    `json:"gainsCount"`
}

// RepointPreviewOutput is a repoint's dry run: what it would move and change, what is in its way, and
// each reason it would be refused. It changes nothing.
type RepointPreviewOutput struct {
	Body struct {
		LivePlaceCount    int                    `json:"livePlaceCount"`
		RemovedPlaceCount int                    `doc:"Places of files already removed, which move to keep their history" json:"removedPlaceCount"` //nolint:lll // one struct tag
		Examples          []RepointMovedPathBody `doc:"A few of the places that move" json:"examples"`
		Roots             []RepointRootBody      `doc:"The library folders at or under the folder, which move with it" json:"roots"`               //nolint:lll // one struct tag
		Libraries         []RepointLibraryBody   `doc:"Each library that would list files it does not now, or stop listing some" json:"libraries"` //nolint:lll // one struct tag
		IsCrossNode       bool                   `json:"isCrossNode"`
		// TargetState: how the new folder's node lists it; only readable lets the move go ahead.
		TargetState      string   `enum:"readable,notAnswering,missing,failed" json:"targetState"`
		TargetFolders    []string `doc:"The folders in the new folder, as its node lists them" json:"targetFolders"`
		TargetFailure    string   `doc:"Why the new folder could not be listed, in the node's words" json:"targetFailure,omitempty"`                //nolint:lll // one struct tag
		ConflictCount    int      `doc:"Places and library folders already known under the new folder, which refuse the move" json:"conflictCount"` //nolint:lll // one struct tag
		ConflictExamples []string `json:"conflictExamples"`
		LookAlikeCount   int      `doc:"Places spelled like the folder in another Unicode form: another folder on disk, which stays" json:"lookAlikeCount"` //nolint:lll // one struct tag
		// Refusals are each reason the move is refused; none, and it may go ahead.
		Refusals []string `doc:"nothingToMove, sameFolder, targetNotReadable, targetHoldsKnown, libraryWouldNest, scanInProgress" json:"refusals"` //nolint:lll // one struct tag
	}
}

// RepointedOutput is what a repoint moved.
type RepointedOutput struct {
	Body struct {
		// LivePlaceCount is how many live files took the new path; the next scan confirms each.
		LivePlaceCount    int `json:"livePlaceCount"`
		RemovedPlaceCount int `json:"removedPlaceCount"`
		RootCount         int `json:"rootCount"`
	}
}

func (server *Server) registerRepoints() {
	tags := []string{tagLibraries}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "previewRepoint", Method: http.MethodPost, Path: "/repoints/preview", Tags: tags,
		Summary:    "What saying a folder moved would do, and why it would be refused; changes nothing (D429)",
		Extensions: callableBy(capabilityAdmin),
	}, server.previewRepoint)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "repoint", Method: http.MethodPost, Path: "/repoints", Tags: tags,
		Summary:    "Say a folder moved: everything known under it takes the new path with its history, and a scan confirms each file (D429)", //nolint:lll // one summary
		Extensions: callableBy(capabilityAdmin),
	}, server.repoint)
}

func folderMoveOf(input *RepointInput) library.FolderMove {
	return library.FolderMove{
		NodeName: input.Body.NodeName, Path: input.Body.Path,
		NewNodeName: input.Body.NewNodeName, NewPath: input.Body.NewPath,
	}
}

func (server *Server) previewRepoint(ctx context.Context, input *RepointInput) (*RepointPreviewOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	preview, err := dependencies.Libraries.PreviewRepoint(ctx, folderMoveOf(input))
	if err != nil {
		return nil, server.repointError(ctx, "preview a repoint", err)
	}
	return repointPreviewOutput(preview), nil
}

func (server *Server) repoint(ctx context.Context, input *RepointInput) (*RepointedOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	repointed, err := dependencies.Libraries.Repoint(ctx, folderMoveOf(input), principal.UserID)
	if err != nil {
		return nil, server.repointError(ctx, "repoint a folder", err)
	}
	output := &RepointedOutput{}
	output.Body.LivePlaceCount = repointed.LivePlaceCount
	output.Body.RemovedPlaceCount = repointed.RemovedPlaceCount
	output.Body.RootCount = repointed.RootCount
	return output, nil
}

// repointError is a repoint's failure as the API says it: a refusal is a conflict naming its reasons,
// which the dry run shows in full.
func (server *Server) repointError(ctx context.Context, action string, err error) error {
	var refused *library.RefusedError
	switch {
	case errors.As(err, &refused):
		reasons := make([]string, 0, len(refused.Preview.Refusals))
		for _, refusal := range refused.Preview.Refusals {
			reasons = append(reasons, string(refusal))
		}
		return newError(http.StatusConflict, CodeConflict,
			fmt.Sprintf("the repoint is refused: %s", strings.Join(reasons, ", ")))
	case errors.Is(err, library.ErrNotAFolderPath):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body", err.Error())
	case errors.Is(err, library.ErrNotANodeRoot):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.newPath", err.Error())
	}
	return server.libraryError(ctx, action, err)
}

func repointPreviewOutput(preview library.RepointPreview) *RepointPreviewOutput {
	output := &RepointPreviewOutput{}
	body := &output.Body
	body.LivePlaceCount, body.RemovedPlaceCount = preview.LivePlaceCount, preview.RemovedPlaceCount
	body.Examples = make([]RepointMovedPathBody, 0, len(preview.Examples))
	for _, example := range preview.Examples {
		body.Examples = append(body.Examples, RepointMovedPathBody{Path: example.Path, NewPath: example.NewPath})
	}
	body.Roots = make([]RepointRootBody, 0, len(preview.Roots))
	for _, root := range preview.Roots {
		body.Roots = append(body.Roots, RepointRootBody{
			LibraryID: root.LibraryID, LibraryName: root.LibraryName, Path: root.Path, NewPath: root.NewPath,
			IsLive: root.IsLive,
		})
	}
	body.Libraries = make([]RepointLibraryBody, 0, len(preview.Libraries))
	for _, change := range preview.Libraries {
		body.Libraries = append(body.Libraries, RepointLibraryBody{
			LibraryID: change.LibraryID, LibraryName: change.LibraryName,
			LosesCount: change.LosesCount, GainsCount: change.GainsCount,
		})
	}
	body.IsCrossNode = preview.IsCrossNode
	body.TargetState, body.TargetFailure = string(preview.Target.State), preview.Target.Failure
	body.TargetFolders = append([]string{}, preview.Target.Folders...)
	body.ConflictCount = preview.ConflictCount
	body.ConflictExamples = append([]string{}, preview.ConflictExamples...)
	body.LookAlikeCount = preview.LookAlikeCount
	body.Refusals = make([]string, 0, len(preview.Refusals))
	for _, refusal := range preview.Refusals {
		body.Refusals = append(body.Refusals, string(refusal))
	}
	return output
}

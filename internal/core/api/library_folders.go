// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// NodeFoldersInput names a folder on a node whose folders an admin browses: one of the node's
// configured roots, or a folder inside one (D359 §2).
type NodeFoldersInput struct {
	AuthorizationHeader
	Name string `doc:"The node's configured name" path:"name"`
	Path string `doc:"One of the node's configured roots, or a folder inside one, by a clean absolute path" minLength:"1" query:"path" required:"true"` //nolint:lll // one struct tag
}

// NodeFoldersOutput is the folders in a folder on a node, or why they could not be listed.
type NodeFoldersOutput struct {
	Body struct {
		Path string `json:"path"`
		// State: readable, with its folders; notAnswering, its disk or share not answering in time;
		// missing, not there; failed, any other failure, a link in its path among them.
		State   string   `enum:"readable,notAnswering,missing,failed" json:"state"`
		Folders []string `doc:"The names of the folders in it, in byte order: no link and no hidden folder" json:"folders"` //nolint:lll // one struct tag
		Failure string   `doc:"Why it could not be listed, in the node's words" json:"failure,omitempty"`
	}
}

// AddLibraryRootInput is a folder an admin adds to a library: one of a node's configured roots or a
// folder inside one, which no library covers (D359 §3).
type AddLibraryRootInput struct {
	AuthorizationHeader
	ID   int64 `path:"id"`
	Body struct {
		NodeName string `doc:"The node's configured name" json:"nodeName" minLength:"1"`
		Path     string `doc:"One of the node's configured roots, or a folder inside one" json:"path" minLength:"1"`
	}
}

func (server *Server) registerLibraryFolders() {
	tags := []string{tagLibraries}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getNodeFolders", Method: http.MethodGet, Path: "/nodes/{name}/folders", Tags: tags,
		Summary:    "The folders in a node's root or a folder inside it, to choose a library folder (D359)",
		Extensions: callableBy(capabilityAdmin),
	}, server.getNodeFolders)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "addLibraryRoot", Method: http.MethodPost, Path: "/libraries/{id}/roots", Tags: tags,
		DefaultStatus: http.StatusAccepted,
		Summary:       "Add a folder to a library, and scan it (D359)",
		Extensions:    callableBy(capabilityAdmin),
	}, server.addLibraryRoot)
}

func (server *Server) getNodeFolders(ctx context.Context, input *NodeFoldersInput) (*NodeFoldersOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	listing, err := dependencies.Libraries.Folders(ctx, input.Name, input.Path)
	if errors.Is(err, library.ErrNotANodeRoot) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "query.path", err.Error())
	}
	if err != nil {
		return nil, server.libraryError(ctx, "list a node's folders", err)
	}
	output := &NodeFoldersOutput{}
	output.Body.Path, output.Body.State, output.Body.Failure = listing.Path, string(listing.State), listing.Failure
	output.Body.Folders = listing.Folders
	if output.Body.Folders == nil {
		output.Body.Folders = []string{}
	}
	return output, nil
}

func (server *Server) addLibraryRoot(ctx context.Context, input *AddLibraryRootInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Libraries.AddRoot(ctx, input.ID, input.Body.NodeName, input.Body.Path)
	if errors.Is(err, library.ErrNotANodeRoot) || errors.Is(err, library.ErrRootCovered) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.path", err.Error())
	}
	if err != nil {
		return nil, server.libraryError(ctx, "add a folder to a library", err)
	}
	return &struct{}{}, nil
}

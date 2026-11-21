// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
)

// FolderInput asks for one folder of an other library.
type FolderInput struct {
	AuthorizationHeader
	ID     int64  `path:"id"`
	Folder string `doc:"Its path below the library's folders, as a listed folder's path says it; absent for their top" maxLength:"4096" query:"folder"` //nolint:lll // one struct tag
	Sort   string `doc:"The order its videos come in" enum:"recorded,name,fileName,added" query:"sort" required:"true"`
	// A page holds the folder's folders first, then its videos: limit of them in all (D432 §3); with
	// no limit, the whole folder in one answer (D475).
	PageOrWholeInput
	Order string `doc:"The way the sort runs, as GET /titles takes it" enum:"ascending,descending" query:"order"`
}

// SubfolderBody is a folder in a folder.
type SubfolderBody struct {
	Name       string `json:"name"`
	Path       string `doc:"What GET /libraries/{id}/folders takes as its folder" json:"path"`
	VideoCount int64  `doc:"How many videos it holds, in the folders inside it too" json:"videoCount"`
}

// FolderOutput is one folder of an other library: the folders in it, and its own videos.
type FolderOutput struct {
	Body struct {
		Folder     string          `doc:"Its path below the library's folders; empty at their top" json:"folder"`
		Subfolders []SubfolderBody `json:"subfolders"`
		Titles     []ItemEntryBody `json:"titles"`
		NextCursor string          `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"`
	}
}

func (server *Server) registerLibraryBrowsing() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getLibraryFolder", Method: http.MethodGet, Path: "/libraries/{id}/folders",
		Summary: "One folder of an other library: the folders in it, and its videos (D349)", Tags: tags,
		Extensions: callableBy(capabilitySignedIn),
	}, server.getLibraryFolder)
}

// listOf is which list a request asks for: its kinds, and one library's or 0.
type listOf struct {
	kinds     []library.ListKind
	libraryID int64
}

// listReach is the libraries a list holds: one library, which the person must reach; else every one
// they reach, less, for films and series, those they left out of them (D349 §8).
func (server *Server) listReach(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, list listOf,
) (auth.Reach, error) {
	switch {
	case list.libraryID != 0:
		narrowed, isReached := principal.Reach.Narrowed(list.libraryID)
		if !isReached {
			return auth.Reach{}, newError(http.StatusNotFound, CodeNotFound, "no such library")
		}
		return narrowed, nil
	case !isFilmsOrSeriesAlone(list.kinds):
		return principal.Reach, nil
	}
	leftOut, err := dependencies.Preferences.LeftOutLibraries(ctx, principal.UserID)
	if err != nil {
		return auth.Reach{}, server.logInternalError(ctx, "read the libraries a person left out", err)
	}
	return server.reachWithout(ctx, dependencies, principal, leftOut)
}

// isFilmsOrSeriesAlone is whether a list names films alone or series alone, the lists a person
// leaves libraries out of (D349 §8). A list of several kinds is All libraries' and leaves none out,
// as the "any" it replaced did (D432 §1).
func isFilmsOrSeriesAlone(kinds []library.ListKind) bool {
	return len(kinds) > 0 && (kinds[0] == library.ListFilms || kinds[0] == library.ListSeries) &&
		!slices.ContainsFunc(kinds, func(kind library.ListKind) bool { return kind != kinds[0] })
}

// isEveryListKind is whether a list names every kind there is, and no other.
func isEveryListKind(kinds []library.ListKind) bool {
	named := slices.Clone(kinds)
	slices.Sort(named)
	every := library.ListKinds()
	slices.Sort(every)
	return slices.Equal(slices.Compact(named), every)
}

// homeReach is the libraries Home's rows draw on: every one the person reaches less those they left
// out of Home (D366 §5). An item two libraries reach is on it while either is left in, as each list
// holds an item any library of its reach reaches (D363 §6).
func (server *Server) homeReach(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal,
) (auth.Reach, error) {
	leftOut, err := dependencies.Preferences.LeftOutOfHome(ctx, principal.UserID)
	if err != nil {
		return auth.Reach{}, server.logInternalError(ctx, "read the libraries a person left out of Home", err)
	}
	return server.reachWithout(ctx, dependencies, principal, leftOut)
}

// reachWithout is the libraries the person reaches less those leftOut names.
func (server *Server) reachWithout(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, leftOut []int64,
) (auth.Reach, error) {
	if len(leftOut) == 0 {
		return principal.Reach, nil
	}
	live, err := dependencies.Browser.LiveLibraryIDs(ctx)
	if err != nil {
		return auth.Reach{}, server.logInternalError(ctx, "list the libraries", err)
	}
	return principal.Reach.Without(leftOut, live), nil
}

func (server *Server) getLibraryFolder(ctx context.Context, input *FolderInput) (*FolderOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	_, offset, err := decodeCursor(input.Cursor)
	if err != nil || offset < 0 {
		return nil, errBadCursor()
	}
	folder, err := dependencies.Browser.Folder(ctx, library.FolderQuery{
		LibraryID: input.ID, Path: input.Folder, Sort: input.Sort, Order: input.Order, Reach: principal.Reach,
		UserID: principal.UserID, Offset: offset, Limit: input.Limit,
	})
	if errors.Is(err, library.ErrNoFolders) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "path.id", err.Error())
	}
	if err != nil {
		return nil, server.libraryError(ctx, "read a library's folder", err)
	}
	output := &FolderOutput{}
	output.Body.Folder = folder.Path
	output.Body.Subfolders = make([]SubfolderBody, 0, len(folder.Subfolders))
	for _, subfolder := range folder.Subfolders {
		output.Body.Subfolders = append(output.Body.Subfolders, SubfolderBody{
			Name: subfolder.Name, Path: subfolder.Path, VideoCount: subfolder.VideoCount,
		})
	}
	output.Body.Titles = make([]ItemEntryBody, 0, len(folder.Entries))
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, entry := range folder.Entries {
		output.Body.Titles = append(output.Body.Titles, itemEntryBody(entry, nodeName))
	}
	// A folder is read whole and sorted afresh for each page, so its cursor is how many of its
	// entries the pages before gave.
	given := len(folder.Subfolders) + len(folder.Entries)
	output.Body.NextCursor = nextCursorOf(given, input.Limit, func() string {
		return library.EncodeCursor(nil, offset+int64(given))
	})
	return output, nil
}

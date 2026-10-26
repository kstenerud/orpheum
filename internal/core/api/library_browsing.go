// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
)

// LettersInput asks which letters of the alphabet rail a list by name or file name has titles under.
type LettersInput struct {
	AuthorizationHeader
	Kind    string `doc:"As GET /titles takes it" enum:"movie,series,other,any" query:"kind" required:"true"`
	Sort    string `doc:"The order the rail follows: by name, or an other video's by file name" enum:"name,file_name" query:"sort" required:"true"` //nolint:lll // one struct tag
	Library int64  `doc:"As GET /titles takes it" minimum:"0" query:"library"`
}

// LettersOutput are the rail's letters the list has titles under.
type LettersOutput struct {
	Body struct {
		Letters []string `doc:"In order, # (before A) then A to Z, only those some title is under: the first at or after the letter, before the next" json:"letters"` //nolint:lll // one struct tag
	}
}

// FolderInput asks for one folder of an other library.
type FolderInput struct {
	AuthorizationHeader
	ID     int64  `path:"id"`
	Folder string `doc:"Its path below the library's folders, as a listed folder's path says it; absent for their top" maxLength:"4096" query:"folder"` //nolint:lll // one struct tag
	Sort   string `doc:"The order its videos come in" enum:"recorded,name,file_name,added" query:"sort" required:"true"`
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
		Titles     []EntryBody     `json:"titles"`
	}
}

func (server *Server) registerLibraryBrowsing() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getTitleLetters", Method: http.MethodGet, Path: "/titles/letters",
		Summary: "The alphabet rail's letters a list by name or file name has titles under (D349)", Tags: tags,
	}, server.getTitleLetters)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getLibraryFolder", Method: http.MethodGet, Path: "/libraries/{id}/folders",
		Summary: "One folder of an other library: the folders in it, and its videos (D349)", Tags: tags,
	}, server.getLibraryFolder)
}

// listReach is the libraries a list holds: one library, which the person must reach, or every one
// they reach, less, for films and series, those they left out of them (D349 §8).
func (server *Server) listReach(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, kind string, libraryID int64,
) (auth.Reach, error) {
	if libraryID != 0 {
		narrowed, isReached := principal.Reach.Narrowed(libraryID)
		if !isReached {
			return auth.Reach{}, newError(http.StatusNotFound, CodeNotFound, "no such library")
		}
		return narrowed, nil
	}
	if kind != library.ListFilms && kind != library.ListSeries {
		return principal.Reach, nil
	}
	leftOut, err := dependencies.Preferences.LeftOutLibraries(ctx, principal.UserID)
	if err != nil {
		return auth.Reach{}, server.logInternalError(ctx, "read the libraries a person left out", err)
	}
	if len(leftOut) == 0 {
		return principal.Reach, nil
	}
	live, err := dependencies.Browser.LiveLibraryIDs(ctx)
	if err != nil {
		return auth.Reach{}, server.logInternalError(ctx, "list the libraries", err)
	}
	return principal.Reach.Without(leftOut, live), nil
}

func (server *Server) getTitleLetters(ctx context.Context, input *LettersInput) (*LettersOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	reach, err := server.listReach(ctx, dependencies, principal, input.Kind, input.Library)
	if err != nil {
		return nil, err
	}
	letters, err := dependencies.Browser.Letters(ctx,
		library.ListQuery{Kind: input.Kind, Sort: input.Sort, Reach: reach})
	if err != nil {
		return nil, server.libraryError(ctx, "list a list's letters", err)
	}
	output := &LettersOutput{}
	output.Body.Letters = letters
	return output, nil
}

func (server *Server) getLibraryFolder(ctx context.Context, input *FolderInput) (*FolderOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	folder, err := dependencies.Browser.Folder(ctx, library.FolderQuery{
		LibraryID: input.ID, Path: input.Folder, Sort: input.Sort, Reach: principal.Reach, UserID: principal.UserID,
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
	output.Body.Titles = make([]EntryBody, 0, len(folder.Entries))
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, entry := range folder.Entries {
		output.Body.Titles = append(output.Body.Titles, entryBody(entry, nodeName))
	}
	return output, nil
}

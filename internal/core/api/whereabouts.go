// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
)

// LibraryNameBody is a library a title or a series is listed in, by id and name.
type LibraryNameBody struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// libraryNameBodies are libraries as the API says them.
func libraryNameBodies(libraries []library.ListingLibrary) []LibraryNameBody {
	bodies := make([]LibraryNameBody, 0, len(libraries))
	for _, each := range libraries {
		bodies = append(bodies, LibraryNameBody{ID: each.ID, Name: each.Name})
	}
	return bodies
}

// FilmOfExtraBody is the film an extra belongs to, which its page and its playing go up to (D367 §2).
type FilmOfExtraBody struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// whereabouts is where the item itemID sits above its page (D367 §2, §3), reach having been checked.
func (server *Server) whereabouts(
	ctx context.Context, dependencies *Dependencies, reach auth.Reach, itemID int64, kind string,
) (library.Whereabouts, error) {
	found, err := dependencies.Browser.Whereabouts(ctx, reach, itemID, kind == library.TitleKindExtra)
	if err != nil {
		return library.Whereabouts{}, server.libraryError(ctx, "read where an item sits", err)
	}
	return found, nil
}

// filmOfExtraBody is an extra's film as the API says it; nil for none.
func filmOfExtraBody(film *library.FilmOfExtra) *FilmOfExtraBody {
	if film == nil {
		return nil
	}
	return &FilmOfExtraBody{ID: film.ID, Name: film.Name}
}

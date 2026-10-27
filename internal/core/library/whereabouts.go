// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
)

// Whereabouts is where an item sits above its page, which its page goes up to (D367 §2, §3): the
// libraries reach reaches that list it, and, for an extra, the film it belongs to.
type Whereabouts struct {
	// Libraries are the libraries reach reaches that list the item, a title by one of its versions, a
	// series by one of its episodes', by name.
	Libraries []ListingLibrary
	// FilmOfExtra is the title an extra belongs to; nil for any other item, or an extra whose title is
	// gone.
	FilmOfExtra *FilmOfExtra
}

// ListingLibrary is a library that lists an item, by id and name.
type ListingLibrary struct {
	ID   int64
	Name string
}

// FilmOfExtra is the title an extra belongs to, by id and name.
type FilmOfExtra struct {
	ID   int64
	Name string
}

// Whereabouts is where the item itemID sits above its page, as reach sees it. The caller has checked
// reach reaches the item.
func (browser *Browser) Whereabouts(
	ctx context.Context, reach auth.Reach, itemID int64, isExtra bool,
) (Whereabouts, error) {
	queries := browser.database.Read()
	listing, err := queries.ListLibrariesOfItem(ctx, itemID)
	if err != nil {
		return Whereabouts{}, fmt.Errorf("list the libraries of item %d: %w", itemID, err)
	}
	whereabouts := Whereabouts{Libraries: []ListingLibrary{}}
	for _, library := range listing {
		if reach.Reaches(&library.ID) {
			whereabouts.Libraries = append(whereabouts.Libraries, ListingLibrary{ID: library.ID, Name: library.Name})
		}
	}
	if !isExtra {
		return whereabouts, nil
	}
	film, err := store.OrNotFound(queries.GetFilmOfExtra(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return whereabouts, nil
	}
	if err != nil {
		return Whereabouts{}, fmt.Errorf("read the film of extra %d: %w", itemID, err)
	}
	whereabouts.FilmOfExtra = &FilmOfExtra{ID: film.OwnerTitle, Name: ptr.ValueOrZero(film.Name)}
	return whereabouts, nil
}

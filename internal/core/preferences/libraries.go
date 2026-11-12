// SPDX-License-Identifier: AGPL-3.0-or-later

package preferences

import (
	"context"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// axisLeftOutLibraries is a person's axis at the user scope: the libraries they leave out of the
// Films and Series pages, which gather every other library of those kinds (D349 §8).
const axisLeftOutLibraries = "left_out_libraries"

// axisLeftOutOfHome is a person's axis at the user scope: the libraries they leave out of Home,
// whose rows draw on every other library they reach (D366 §5).
const axisLeftOutOfHome = "left_out_of_home"

// LeftOutLibraries are the libraries userID leaves out of Films and Series, in id order; none until
// they leave one out.
func (service *Service) LeftOutLibraries(ctx context.Context, userID int64) ([]int64, error) {
	return service.leftOut(ctx, userID, axisLeftOutLibraries)
}

// SetLeftOutLibraries keeps the libraries userID leaves out of Films and Series, each once, in id
// order: a library they do not reach is kept too, and leaves nothing out until it is given them.
func (service *Service) SetLeftOutLibraries(ctx context.Context, userID int64, libraryIDs []int64) error {
	return service.setLeftOut(ctx, userID, axisLeftOutLibraries, libraryIDs)
}

// LeftOutOfHome are the libraries userID leaves out of Home, in id order; none until they leave one
// out, every library they reach on Home by default (D366 §5).
func (service *Service) LeftOutOfHome(ctx context.Context, userID int64) ([]int64, error) {
	return service.leftOut(ctx, userID, axisLeftOutOfHome)
}

// SetLeftOutOfHome keeps the libraries userID leaves out of Home, as SetLeftOutLibraries keeps
// those left out of Films and Series.
func (service *Service) SetLeftOutOfHome(ctx context.Context, userID int64, libraryIDs []int64) error {
	return service.setLeftOut(ctx, userID, axisLeftOutOfHome, libraryIDs)
}

// leftOut are the libraries userID leaves out of the place axis names, in id order.
func (service *Service) leftOut(ctx context.Context, userID int64, axis string) ([]int64, error) {
	personal, err := service.database.Read().ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeUser, ScopeID: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("read user %d's preferences: %w", userID, err)
	}
	leftOut := []int64{}
	for _, preference := range personal {
		if preference.Axis != axis {
			continue
		}
		if err := strictjson.Unmarshal([]byte(preference.ValueJSON), &leftOut); err != nil {
			return nil, fmt.Errorf("read user %d's %s: %w", userID, axis, err)
		}
	}
	return leftOut, nil
}

// setLeftOut keeps the libraries userID leaves out of the place axis names, each once, in id order.
func (service *Service) setLeftOut(ctx context.Context, userID int64, axis string, libraryIDs []int64) error {
	kept := slices.Compact(slices.Sorted(slices.Values(libraryIDs)))
	if kept == nil {
		kept = []int64{}
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return service.set(ctx, queries, replacing, place{userID: userID, scope: scopeUser, axis: axis}, kept)
	})
	if err != nil {
		return fmt.Errorf("keep user %d's %s: %w", userID, axis, err)
	}
	return nil
}

// axisLibrarySort is a library's axis at its library scope: the sort its person lists it in, and
// the way it runs (X5, the owner, 2026-11-12).
const axisLibrarySort = "library_sort"

// LibrarySort is the order a person lists a library in: a sort by its name as GET /items takes it
// ("name", "added", "recorded", "fileName" or "released"), and the way it runs, "ascending" or
// "descending" (D366 §2).
type LibrarySort struct {
	Sort  string `json:"sort"`
	Order string `json:"order"`
}

// ChooseLibrarySort keeps the order userID lists a library in, chosen on its page, so every device
// they sign in on opens it so. The caller has found they reach it; whether its kind offers the sort
// is the page's to judge as it opens it.
func (service *Service) ChooseLibrarySort(ctx context.Context, userID, libraryID int64, chosen LibrarySort) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		inLibrary := place{userID: userID, scope: scopeLibrary, scopeID: libraryID, axis: axisLibrarySort}
		return service.set(ctx, queries, replacing, inLibrary, chosen)
	})
	if err != nil {
		return fmt.Errorf("keep the sort user %d lists library %d in: %w", userID, libraryID, err)
	}
	return nil
}

// LibrarySorts are the orders userID chose to list libraries in, by library id; a library they
// chose none for is absent. A deleted library's stays, read by nothing, as it is listed no more.
func (service *Service) LibrarySorts(ctx context.Context, userID int64) (map[int64]LibrarySort, error) {
	kept, err := service.database.Read().ListPreferencesOfAxisAtScope(ctx,
		sqlcgen.ListPreferencesOfAxisAtScopeParams{UserID: userID, Scope: scopeLibrary, Axis: axisLibrarySort})
	if err != nil {
		return nil, fmt.Errorf("read the sorts user %d lists libraries in: %w", userID, err)
	}
	sorts := make(map[int64]LibrarySort, len(kept))
	for _, preference := range kept {
		var chosen LibrarySort
		if err := strictjson.Unmarshal([]byte(preference.ValueJSON), &chosen); err != nil {
			return nil, fmt.Errorf("read the sort user %d lists library %d in: %w", userID, preference.ScopeID, err)
		}
		sorts[preference.ScopeID] = chosen
	}
	return sorts, nil
}

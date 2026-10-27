// SPDX-License-Identifier: AGPL-3.0-or-later

package preferences

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
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
		if err := json.Unmarshal([]byte(preference.ValueJSON), &leftOut); err != nil {
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

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

// LeftOutLibraries are the libraries userID leaves out of Films and Series, in id order; none until
// they leave one out.
func (service *Service) LeftOutLibraries(ctx context.Context, userID int64) ([]int64, error) {
	personal, err := service.database.Read().ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeUser, ScopeID: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("read user %d's preferences: %w", userID, err)
	}
	leftOut := []int64{}
	for _, preference := range personal {
		if preference.Axis != axisLeftOutLibraries {
			continue
		}
		if err := json.Unmarshal([]byte(preference.ValueJSON), &leftOut); err != nil {
			return nil, fmt.Errorf("read user %d's libraries left out: %w", userID, err)
		}
	}
	return leftOut, nil
}

// SetLeftOutLibraries keeps the libraries userID leaves out of Films and Series, each once, in id
// order: a library they do not reach is kept too, and leaves nothing out until it is given them.
func (service *Service) SetLeftOutLibraries(ctx context.Context, userID int64, libraryIDs []int64) error {
	kept := slices.Compact(slices.Sorted(slices.Values(libraryIDs)))
	if kept == nil {
		kept = []int64{}
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return service.set(ctx, queries, replacing, place{userID: userID, scope: scopeUser, axis: axisLeftOutLibraries},
			kept)
	})
	if err != nil {
		return fmt.Errorf("keep user %d's libraries left out: %w", userID, err)
	}
	return nil
}

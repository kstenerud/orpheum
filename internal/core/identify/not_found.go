// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A film or series found to match nothing is searched again once the rules it was searched by
// change (D513 §5).

// searchRulesVersion is the version of the rules a film or series is searched by. A change to how
// names are parsed or searched raises it, and each title found to match nothing under an older one
// is searched again, a few at a time (D513 §5). 1 is D513 §1–§4's rules, the first recorded: every
// "not found" from before, recorded under none, is searched again under them.
const searchRulesVersion = 1

// notFoundSearchedAgainPerSweep is how many titles found to match nothing under older rules one
// sweep returns to their first match: enough that a library's few hundred are searched again within
// an hour or two of a release's start, the sweep running hourly as the first matches' does, yet
// never a flood of searches, each costing at most some four (D513 §5; 200, the owner, 2026-11-18).
const notFoundSearchedAgainPerSweep = 200

// SearchNotFoundAgain returns to their first match the films and series found to match nothing
// under older search rules than the matcher's, or under none, at most notFoundSearchedAgainPerSweep
// a sweep: the first-match sweep then searches each under the rules of now (D513 §5). A library
// read from NFO files alone, which searches nothing, is left alone, and so is a person's choice. It
// says how many it returned.
func (matcher *Matcher) SearchNotFoundAgain(ctx context.Context) (int, error) {
	owed, err := matcher.database.Read().ListNotFoundUnderOlderRules(ctx, sqlcgen.ListNotFoundUnderOlderRulesParams{
		CurrentVersion: new(matcher.searchRules), MaxRows: notFoundSearchedAgainPerSweep,
	})
	if err != nil {
		return 0, fmt.Errorf("list the titles found to match nothing under older search rules: %w", err)
	}
	if len(owed) == 0 {
		return 0, nil
	}
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, id := range owed {
			if returnErr := queries.ReturnNotFoundToItsFirstMatch(ctx, id); returnErr != nil {
				return fmt.Errorf("return item %d to its first match: %w", id, returnErr)
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("return the titles found to match nothing under older rules: %w", err)
	}
	return len(owed), nil
}

// recordNotFoundUnderRules records the search rules' version a film or series was just found to
// match nothing under, so it is searched again once they change (D513 §5).
func (matcher *Matcher) recordNotFoundUnderRules(ctx context.Context, queries *sqlcgen.Queries, itemID int64) error {
	err := queries.RecordNotFoundUnderRules(ctx, sqlcgen.RecordNotFoundUnderRulesParams{
		SearchRulesVersion: new(matcher.searchRules), ID: itemID,
	})
	if err != nil {
		return fmt.Errorf("record the search rules item %d found nothing under: %w", itemID, err)
	}
	return nil
}

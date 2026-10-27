// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// rematchOutcome is what a rescan's match of one film or series came to (D357 §6).
type rematchOutcome string

// The outcomes of a rescan's match: matched to another work than before; to the same; left alone,
// as a person set it or its NFO speaks for it (D357 §3); left with several, or none, to choose
// from, waiting for a person as a first match would (D172, D301); or failed, through its own fault,
// at its provider (D231 §4).
const (
	rematchChanged   rematchOutcome = "changed"
	rematchUnchanged rematchOutcome = "unchanged"
	rematchLeftAlone rematchOutcome = "left alone"
	rematchUnsettled rematchOutcome = "unsettled"
	rematchFailed    rematchOutcome = "failed"
)

// rematched is what a rescan's match of one item did: its outcome, the work it followed before and
// the one it follows now, at its provider, by name, and the rule that settled it, or why it was
// left alone.
type rematched struct {
	outcome          rematchOutcome
	before, after    string
	beforeName       string
	afterName        string
	settledBy        string
	isGuess          bool
	leftAloneBecause string
}

// The reasons a rescan leaves a film or series alone (D357 §3).
const (
	leftAloneChosen  = "a person chose its match"
	leftAloneByNFO   = "its NFO file speaks for it"
	leftAloneNoMatch = "its library is not matched at a provider"
	leftAloneGone    = "it is gone, or not a film or series"
)

// rematch matches a film or series again as it would one never matched (D357 §2): the name, year
// and ids its files and NFOs give, its provider searched again, the matcher as it now is settling
// it. What a person set is left alone (§3); a different answer replaces the old one, marked a guess
// where it is one (§4).
func (matcher *Matcher) rematch(ctx context.Context, itemID int64, said filesQuery) (rematched, error) {
	kind, err := store.OrNotFound(matcher.database.Read().GetItemKind(ctx, itemID))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return rematched{outcome: rematchLeftAlone, leftAloneBecause: leftAloneGone}, nil
	case err != nil:
		return rematched{}, fmt.Errorf("read item %d: %w", itemID, err)
	case kind == itemKindCollection:
		return matcher.rematchSeries(ctx, itemID, said)
	}
	return matcher.rematchFilm(ctx, itemID, said)
}

// settlement is the work a rescan's match settled on, how, and by which rule; the search's
// candidates, for a guess or a person to choose from; and the details fetched of the one chosen
// for its runtime, nil when none were (D365 §4).
type settlement struct {
	chosen     Candidate
	how        settledBy
	rule       string
	candidates []Candidate
	chosenBody []byte
}

// settledFromFiles is what an item's files say settles its match on: an id they give, or a fresh
// search for their name and year, a film's candidates fitted to its files (D365).
func (matcher *Matcher) settledFromFiles(
	ctx context.Context, provider Provider, itemID int64, kind string, said filesQuery,
) (settlement, error) {
	id, err := idOfIDs(ctx, provider, kind, said.ids)
	if err != nil {
		return settlement{}, err
	}
	if id != "" {
		return settlement{chosen: Candidate{ID: id}, how: clearly, rule: settledByIDInHand}, nil
	}
	query, candidates, err := searchWithSpellings(ctx, provider, Query{Kind: kind, Name: said.name, Year: said.year})
	if err != nil {
		return settlement{}, err
	}
	candidates, fit, err := matcher.fitFilm(ctx, provider, itemID, query, candidates)
	if err != nil {
		return settlement{}, err
	}
	chosen, how, rule := settleMatchSaying(query, candidates, kind == KindMovie, fit.lengthsMs)
	matcher.logFilmSettled(ctx, itemID, query, candidates, fit, settlement{chosen: chosen, how: how})
	return settlement{
		chosen: chosen, how: how, rule: rule, candidates: candidates, chosenBody: fit.fetched[chosen.ID],
	}, nil
}

// settleRematch ends a rescan's match that settled: another work replaces the old one (replace), and the
// answer is marked a guess where it is one, else cleared of an earlier guess. It says what it did.
func (matcher *Matcher) settleRematch(
	ctx context.Context, provider Provider, itemID int64, settled settlement, result rematched,
	replace func() error,
) (rematched, error) {
	result.after, result.outcome = settled.chosen.ID, rematchUnchanged
	if settled.chosen.ID != result.before {
		result.outcome = rematchChanged
		if err := replace(); err != nil {
			return rematched{}, err
		}
	}
	if err := matcher.keepGuessOrNot(ctx, provider, itemID, settled.how, settled.candidates); err != nil {
		return rematched{}, err
	}
	result.afterName = settled.chosen.Name
	if result.afterName != "" {
		return result, nil
	}
	name, err := matcher.itemName(ctx, itemID) // an id its files give, not a search's candidate
	result.afterName = name
	return result, err
}

// rematchFilm is rematch of a film.
func (matcher *Matcher) rematchFilm(ctx context.Context, titleID int64, said filesQuery) (rematched, error) {
	title, err := store.OrNotFound(matcher.database.Read().GetTitleForMatch(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) || err == nil && title.Kind != KindMovie {
		return rematched{outcome: rematchLeftAlone, leftAloneBecause: leftAloneGone}, nil
	}
	if err != nil {
		return rematched{}, fmt.Errorf("read title %d: %w", titleID, err)
	}
	if why, isLeftAlone := leftAloneFilm(title, said); isLeftAlone {
		return rematched{outcome: rematchLeftAlone, leftAloneBecause: why}, nil
	}
	provider, known := matcher.providers[providerOf(title.MetadataSource)]
	if !known {
		return rematched{outcome: rematchLeftAlone, leftAloneBecause: leftAloneNoMatch}, nil
	}
	result := rematched{beforeName: ptr.ValueOrZero(title.Name)}
	if ptr.ValueOrZero(title.FollowedSource) == provider.Name() {
		result.before = ptr.ValueOrZero(title.FollowedSourceID)
	}
	settled, err := matcher.settledFromFiles(ctx, provider, titleID, KindMovie, said)
	if err != nil {
		return rematched{}, err
	}
	result.settledBy, result.isGuess = settled.rule, settled.how == byGuess
	if settled.how == unsettled {
		result.outcome = rematchUnsettled
		return result, matcher.unsettleFilm(ctx, provider, title, said, settled.candidates)
	}
	return matcher.settleRematch(ctx, provider, titleID, settled, result, func() error {
		return matcher.acceptAnswer(ctx, provider, title, answer{
			id: settled.chosen.ID, identification: identificationIdentified, keptIDs: said.ids,
			body: settled.chosenBody,
		})
	})
}

// leftAloneFilm says whether a rescan leaves a film as it is, and why: a person chose its match,
// or an NFO speaks for it.
func leftAloneFilm(title sqlcgen.GetTitleForMatchRow, said filesQuery) (string, bool) {
	switch {
	case title.IdentificationState == identificationConfirmed:
		return leftAloneChosen, true
	case said.isNFOSource || title.MetadataSource == sourceNFO || ptr.ValueOrZero(title.FollowedSource) == sourceNFO:
		return leftAloneByNFO, true
	}
	return "", false
}

// keepGuessOrNot marks a rescan's answer a guess where it is one, its candidates kept for a person
// correcting it, and otherwise clears an earlier guess and its candidates: a clear match keeps none
// (D301).
func (matcher *Matcher) keepGuessOrNot(
	ctx context.Context, provider Provider, itemID int64, how settledBy, candidates []Candidate,
) error {
	if how == byGuess {
		return matcher.markGuess(ctx, provider, itemID, candidates)
	}
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.ClearGuessedMatch(ctx, itemID)
	})
	if err != nil {
		return fmt.Errorf("clear item %d's guess: %w", itemID, err)
	}
	return matcher.dropCandidates(ctx, itemID)
}

// unsettleFilm leaves a film its rescan's search did not settle as a first match leaves one: with
// several candidates, ambiguous, kept for a person; with none, not found. The ids an earlier match
// recorded go, and its library owes a pass, which names it from its files again.
func (matcher *Matcher) unsettleFilm(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow, said filesQuery,
	candidates []Candidate,
) error {
	if err := matcher.forgetEarlierMatch(ctx, title.ID, said); err != nil {
		return err
	}
	var err error
	if len(candidates) == 0 {
		err = matcher.markNoMatch(ctx, title)
	} else {
		err = matcher.markAmbiguous(ctx, provider, title, candidates)
	}
	if err != nil {
		return err
	}
	if title.LibraryID != nil {
		matcher.pass.OwePass(*title.LibraryID)
	}
	return nil
}

// forgetEarlierMatch forgets what an item's earlier match recorded, its guess and its ids, but the
// ids its files give.
func (matcher *Matcher) forgetEarlierMatch(ctx context.Context, itemID int64, said filesQuery) error {
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if err := queries.ClearGuessedMatch(ctx, itemID); err != nil {
			return fmt.Errorf("clear its guess: %w", err)
		}
		return forgetIDsBut(ctx, queries, itemID, said.ids)
	})
	if err != nil {
		return fmt.Errorf("forget item %d's earlier match: %w", itemID, err)
	}
	return nil
}

// forgetIDsBut forgets every provider id an item holds but kept, by provider: what an earlier match
// recorded goes when a rescan's answer replaces it (D357 §4).
func forgetIDsBut(ctx context.Context, queries *sqlcgen.Queries, itemID int64, kept map[string]string) error {
	pairs := make([]string, 0, len(kept))
	for _, provider := range slices.Sorted(maps.Keys(kept)) {
		pairs = append(pairs, provider+":"+kept[provider])
	}
	keptJSON, err := json.Marshal(pairs)
	if err != nil {
		return fmt.Errorf("encode item %d's kept ids: %w", itemID, err)
	}
	err = queries.ForgetItemProviderIDsBut(ctx, sqlcgen.ForgetItemProviderIDsButParams{
		ItemID: itemID, KeptJSON: string(keptJSON),
	})
	if err != nil {
		return fmt.Errorf("forget item %d's earlier ids: %w", itemID, err)
	}
	return nil
}

// itemName is a film's or series' name as it is stored now.
func (matcher *Matcher) itemName(ctx context.Context, itemID int64) (string, error) {
	name, err := store.OrNotFound(matcher.database.Read().GetItemName(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read item %d's name: %w", itemID, err)
	}
	return name, nil
}

// rematchSeries is rematch of a series: the one thing identified, its episodes following it (D209).
func (matcher *Matcher) rematchSeries(ctx context.Context, seriesID int64, said filesQuery) (rematched, error) {
	series, err := store.OrNotFound(matcher.database.Read().GetSeriesForMatch(ctx, seriesID))
	if errors.Is(err, store.ErrNotFound) {
		return rematched{outcome: rematchLeftAlone, leftAloneBecause: leftAloneGone}, nil
	}
	if err != nil {
		return rematched{}, fmt.Errorf("read series %d: %w", seriesID, err)
	}
	if ptr.ValueOrZero(series.IdentificationState) == identificationConfirmed {
		return rematched{outcome: rematchLeftAlone, leftAloneBecause: leftAloneChosen}, nil
	}
	provider, known := matcher.providers[providerOf(series.MetadataSource)]
	if !known || series.MetadataSource == sourceNFO {
		return rematched{outcome: rematchLeftAlone, leftAloneBecause: leftAloneNoMatch}, nil
	}
	result := rematched{beforeName: ptr.ValueOrZero(series.Name)}
	if isSeriesMatched(series) {
		result.before = ptr.ValueOrZero(series.FollowedSourceID)
	}
	settled, err := matcher.settledFromFiles(ctx, provider, series.ID, KindSeries, said)
	if err != nil {
		return rematched{}, err
	}
	result.settledBy, result.isGuess = settled.rule, settled.how == byGuess
	if settled.how == unsettled {
		result.outcome = rematchUnsettled
		return result, matcher.unsettleSeries(ctx, provider, series, said, settled.candidates)
	}
	return matcher.settleRematch(ctx, provider, seriesID, settled, result, func() error {
		return matcher.replaceSeriesMatch(ctx, provider, series, settled.chosen.ID, said)
	})
}

// replaceSeriesMatch matches a series to another work: the old work's season listings go, as a
// person's choice drops them, so its episodes are the new work's (D301 §2), and the ids the old
// match recorded go with them.
func (matcher *Matcher) replaceSeriesMatch(
	ctx context.Context, provider Provider, series sqlcgen.GetSeriesForMatchRow, id string, said filesQuery,
) error {
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.DeleteSeasonListings(ctx, sqlcgen.DeleteSeasonListingsParams{
			ItemID: series.ID, Provider: provider.Name(),
		})
	})
	if err != nil {
		return fmt.Errorf("drop series %d's season listings: %w", series.ID, err)
	}
	return matcher.acceptSeriesAnswer(ctx, provider, series, answer{
		id: id, identification: identificationIdentified, keptIDs: said.ids,
	})
}

// unsettleSeries leaves a series its rescan's search did not settle as a first match leaves one:
// ambiguous with its candidates kept, or not found; its library then owes a pass, which names it
// and its episodes from its folders again.
func (matcher *Matcher) unsettleSeries(
	ctx context.Context, provider Provider, series sqlcgen.GetSeriesForMatchRow, said filesQuery,
	candidates []Candidate,
) error {
	if err := matcher.forgetEarlierMatch(ctx, series.ID, said); err != nil {
		return err
	}
	state, source := identificationAmbiguous, (*string)(nil)
	if len(candidates) == 0 {
		state, source = identificationUnidentified, new(sourceNone)
	} else if err := matcher.keepCandidates(ctx, provider, series.ID, candidates); err != nil {
		return err
	}
	return matcher.recordSeriesAndRederive(ctx, series, func(queries *sqlcgen.Queries) error {
		return queries.SetSeriesMatch(ctx, sqlcgen.SetSeriesMatchParams{
			FollowedSource: source, IdentificationState: &state, ID: series.ID,
		})
	})
}

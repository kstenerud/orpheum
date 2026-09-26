// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// matchSeries matches a series, or, once it is matched, fetches the season listings its episodes
// need (D209). Either way the pass then re-derives its library, which is how its episodes take
// their values.
func (matcher *Matcher) matchSeries(ctx context.Context, seriesID int64) (Outcome, error) {
	series, err := store.One(matcher.database.Read().GetSeriesForMatch(ctx, seriesID))
	if errors.Is(err, store.ErrNotFound) {
		return OutcomeNothingToDo, nil // not a series, or gone
	}
	if err != nil {
		return "", fmt.Errorf("read series %d: %w", seriesID, err)
	}
	provider, known := matcher.providers[providerOf(series.MetadataSource)]
	if !known || series.MetadataSource == sourceNFO {
		return OutcomeNothingToDo, nil
	}
	ident := derefText(series.Identification)
	switch {
	case seriesMatched(series):
		return matcher.listSeasons(ctx, provider, series)
	case ident == identAmbiguous, ident == identConfirmed:
		return OutcomeNothingToDo, nil // a person's to choose, or chosen
	}
	id, err := matcher.idInHand(ctx, provider, KindSeries, seriesID)
	if err != nil {
		return "", err
	}
	if id != "" {
		return OutcomeMatched, matcher.acceptSeries(ctx, provider, series, id, identIdentified)
	}
	query := Query{Kind: KindSeries, Name: derefText(series.Name), Year: int(deref(series.Year))}
	candidates, err := provider.Search(ctx, query)
	if err != nil {
		return "", fmt.Errorf("search %s for the series %q: %w", provider.Name(), query.Name, err)
	}
	if chosen, single := singleMatch(query, candidates, false); single {
		return OutcomeMatched, matcher.acceptSeries(ctx, provider, series, chosen.ID, identIdentified)
	}
	if len(candidates) == 0 {
		return OutcomeNoMatch, matcher.seriesOutcome(ctx, series, func(queries *sqlcgen.Queries) error {
			return queries.MarkSeriesUnmatched(ctx, seriesID)
		})
	}
	if err := matcher.keep(ctx, provider, seriesID, candidates); err != nil {
		return "", err
	}
	return OutcomeAmbiguous, matcher.seriesOutcome(ctx, series, func(queries *sqlcgen.Queries) error {
		return queries.SetSeriesMatch(ctx, sqlcgen.SetSeriesMatchParams{
			Identification: new(identAmbiguous),
			ID:             seriesID,
		})
	})
}

// acceptSeries fetches and stores the series' own response, records the match, identified by
// the matcher or confirmed by a person, and fetches its season listings.
func (matcher *Matcher) acceptSeries(
	ctx context.Context, provider Provider, series sqlcgen.GetSeriesForMatchRow, id, ident string,
) error {
	body, err := provider.Fetch(ctx, KindSeries, id)
	if err != nil {
		return fmt.Errorf("fetch %s series %s: %w", provider.Name(), id, err)
	}
	metadata, err := provider.Parse(KindSeries, body)
	if err != nil {
		return fmt.Errorf("read %s series %s: %w", provider.Name(), id, err)
	}
	stored, err := compress(body)
	if err != nil {
		return err
	}
	now := matcher.clock.Now().UnixMilli()
	found := seriesFound{provider: provider.Name(), seriesID: series.ID, metadata: metadata, ident: ident}
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return recordSeries(ctx, queries, found, stored, now)
	})
	if err != nil {
		return fmt.Errorf("record series %d's match: %w", series.ID, err)
	}
	series.SourceID, series.Identification = &metadata.ID, &ident
	_, err = matcher.listSeasons(ctx, provider, series)
	return err
}

// seriesFound is what a series was matched to, and by whom.
type seriesFound struct {
	provider string
	seriesID int64
	metadata Metadata
	ident    string
}

// recordSeries stores a series' response and ids and its match. A person's choice replaces the
// ids an earlier match recorded at that provider.
func recordSeries(ctx context.Context, queries *sqlcgen.Queries, found seriesFound, stored []byte, now int64) error {
	provider, seriesID, metadata, ident := found.provider, found.seriesID, found.metadata, found.ident
	if ident == identConfirmed {
		forget := sqlcgen.DeleteItemProviderIDsOfParams{ItemID: seriesID, Provider: provider}
		if err := queries.DeleteItemProviderIDsOf(ctx, forget); err != nil {
			return fmt.Errorf("forget the earlier %s match: %w", provider, err)
		}
	}
	err := queries.UpsertProviderResponse(ctx, sqlcgen.UpsertProviderResponseParams{
		ItemID: seriesID, Provider: provider, Part: "", FetchedAt: now, Body: stored,
	})
	if err != nil {
		return fmt.Errorf("store the response: %w", err)
	}
	ids := map[string]string{provider: metadata.ID}
	for other, id := range metadata.IDs {
		if _, own := ids[other]; !own {
			ids[other] = id
		}
	}
	for other, id := range ids {
		record := sqlcgen.InsertItemProviderIDParams{ItemID: seriesID, Provider: other, ProviderID: id}
		if insertErr := queries.InsertItemProviderID(ctx, record); insertErr != nil {
			return fmt.Errorf("record %s %s: %w", other, id, insertErr)
		}
	}
	err = queries.SetSeriesMatch(ctx, sqlcgen.SetSeriesMatchParams{
		SourceID: &metadata.ID, Identification: &ident,
		ID: seriesID,
	})
	if err != nil {
		return fmt.Errorf("record the match: %w", err)
	}
	return nil
}

// listSeasons fetches every season listing the series' episodes need — one call a season, never
// one an episode (D209) — stores them, and re-derives the library.
func (matcher *Matcher) listSeasons(
	ctx context.Context, provider Provider, series sqlcgen.GetSeriesForMatchRow,
) (Outcome, error) {
	seasons, err := seasonsToFetch(ctx, matcher.database.Read(), provider, series.ID)
	if err != nil {
		return "", err
	}
	outcome := OutcomeListed
	if len(seasons) == 0 {
		outcome = OutcomeNothingToDo
	}
	for _, season := range seasons {
		body, err := provider.FetchSeason(ctx, derefText(series.SourceID), season)
		if errors.Is(err, ErrNoMatch) {
			body, err = []byte{}, nil // the provider has no such season: stored empty, so it is not asked again
		}
		if err != nil {
			return "", fmt.Errorf("fetch series %d's season %d: %w", series.ID, season, err)
		}
		if err := matcher.storeSeason(ctx, provider, series.ID, season, body); err != nil {
			return "", err
		}
	}
	return outcome, matcher.seriesOutcome(ctx, series, nil)
}

func (matcher *Matcher) storeSeason(
	ctx context.Context, provider Provider, seriesID int64, season int, body []byte,
) error {
	stored, err := compress(body)
	if err != nil {
		return err
	}
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertProviderResponse(ctx, sqlcgen.UpsertProviderResponseParams{
			ItemID: seriesID, Provider: provider.Name(), Part: seasonPart(season),
			FetchedAt: matcher.clock.Now().UnixMilli(), Body: stored,
		})
	})
	if err != nil {
		return fmt.Errorf("store series %d's season %d: %w", seriesID, season, err)
	}
	return nil
}

// seriesOutcome writes what a match found, if anything, and re-derives the series' library, so
// its episodes are filed by it.
func (matcher *Matcher) seriesOutcome(
	ctx context.Context, series sqlcgen.GetSeriesForMatchRow, change func(*sqlcgen.Queries) error,
) error {
	if change != nil {
		if err := matcher.database.Write(ctx, change); err != nil {
			return fmt.Errorf("record series %d's match: %w", series.ID, err)
		}
	}
	if series.LibraryID == nil {
		return nil
	}
	if _, err := matcher.pass.Library(ctx, *series.LibraryID); err != nil {
		return err
	}
	return nil
}

// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Matcher matches one title to its library's provider: the fetch job's work (D170). A provider
// id in hand skips the search; a single result matching the name and year is accepted; anything
// else is ambiguous, with the results kept for the chooser. It never guesses (D172).
type Matcher struct {
	database  *store.Database
	cache     *store.Cache
	providers Providers
	clock     clock.Clock
	logger    *slog.Logger
}

// NewMatcher returns the matcher.
func NewMatcher(
	database *store.Database, cache *store.Cache, providers Providers, now clock.Clock, logger *slog.Logger,
) *Matcher {
	return &Matcher{database: database, cache: cache, providers: providers, clock: now, logger: logger}
}

// Outcome is what a match did.
type Outcome string

// The outcomes of a match.
const (
	OutcomeMatched     Outcome = "matched"
	OutcomeAmbiguous   Outcome = "ambiguous"
	OutcomeNoMatch     Outcome = "no match"
	OutcomeNothingToDo Outcome = "nothing to do"
)

// Match matches a title. Provider errors return as they came, for the queue to wait or retry.
func (matcher *Matcher) Match(ctx context.Context, titleID int64) (Outcome, error) {
	title, err := store.One(matcher.database.Read().GetTitleForMatch(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return OutcomeNothingToDo, nil // gone since it was queued
	}
	if err != nil {
		return "", fmt.Errorf("read title %d: %w", titleID, err)
	}
	if title.Source != nil || title.Ident != identUnidentified || title.MetadataSource == sourceNFO {
		return OutcomeNothingToDo, nil // matched, answered, ambiguous or confirmed since it was queued
	}
	provider, known := matcher.providers[providerOf(title.MetadataSource)]
	if !known {
		return "", fmt.Errorf("library source %s: no provider client", title.MetadataSource)
	}
	id, err := matcher.idInHand(ctx, provider, title)
	if err != nil {
		return "", err
	}
	if id != "" {
		return OutcomeMatched, matcher.accept(ctx, provider, title, id)
	}
	query := Query{Kind: title.Kind, Name: derefText(title.Name), Year: int(deref(title.Year))}
	candidates, err := provider.Search(ctx, query)
	if err != nil {
		return "", fmt.Errorf("search %s for %q: %w", provider.Name(), query.Name, err)
	}
	if chosen, single := singleMatch(query, candidates); single {
		return OutcomeMatched, matcher.accept(ctx, provider, title, chosen.ID)
	}
	if len(candidates) == 0 {
		return OutcomeNoMatch, matcher.noMatch(ctx, title)
	}
	return OutcomeAmbiguous, matcher.ambiguous(ctx, provider, title, candidates)
}

func providerOf(metadataSource string) string {
	return strings.TrimPrefix(metadataSource, "nfo_then_")
}

// idInHand is the provider's id for a title from its NFO or its path, if it has one: the
// provider's own, or one found from an IMDb id.
func (matcher *Matcher) idInHand(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow,
) (string, error) {
	ids, err := matcher.database.Read().ListItemProviderIDs(ctx, title.ID)
	if err != nil {
		return "", fmt.Errorf("read title %d's ids: %w", title.ID, err)
	}
	byProvider := map[string]string{}
	for _, row := range ids {
		byProvider[row.Provider] = row.ProviderID
	}
	if id := byProvider[provider.Name()]; id != "" {
		return id, nil
	}
	imdb := byProvider["imdb"]
	if imdb == "" {
		return "", nil
	}
	id, err := provider.Find(ctx, title.Kind, "imdb", imdb)
	if errors.Is(err, ErrNoMatch) {
		return "", nil // search instead
	}
	if err != nil {
		return "", fmt.Errorf("find imdb %s at %s: %w", imdb, provider.Name(), err)
	}
	return id, nil
}

// singleMatch is the one candidate whose name and year are the query's (D172). With no year in
// hand nothing matches: a wrong match is worse than none.
func singleMatch(query Query, candidates []Candidate) (Candidate, bool) {
	if query.Year == 0 {
		return Candidate{}, false
	}
	var found []Candidate
	want := foldName(query.Name)
	for _, candidate := range candidates {
		named := foldName(candidate.Name) == want || foldName(candidate.OriginalName) == want
		if named && candidate.Year == query.Year {
			found = append(found, candidate)
		}
	}
	if len(found) != 1 {
		return Candidate{}, false
	}
	return found[0], true
}

// foldName folds a name for comparison: NFC, case-folded, punctuation dropped, spaces
// collapsed (D49: letters of every script are kept).
func foldName(name string) string {
	var builder strings.Builder
	space := false
	for _, character := range norm.NFC.String(name) {
		switch {
		case unicode.IsLetter(character) || unicode.IsNumber(character):
			if space && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(unicode.ToLower(character))
			space = false
		case unicode.IsSpace(character):
			space = true
		}
	}
	return builder.String()
}

// accept fetches the work, stores the response and re-derives the title from it. A title that
// turns out to be one already in the library joins it.
func (matcher *Matcher) accept(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow, id string,
) error {
	body, err := provider.Fetch(ctx, title.Kind, id)
	if err != nil {
		return fmt.Errorf("fetch %s %s: %w", provider.Name(), id, err)
	}
	metadata, err := provider.Parse(title.Kind, body)
	if err != nil {
		return fmt.Errorf("read %s %s: %w", provider.Name(), id, err)
	}
	stored, err := compress(body)
	if err != nil {
		return err
	}
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return matcher.record(ctx, queries, provider.Name(), title, metadata.ID, stored)
	})
	if err != nil {
		return fmt.Errorf("record title %d's match: %w", title.ID, err)
	}
	return nil
}

// record stores a match's response and re-derives the title from it, as a pass would.
func (matcher *Matcher) record(
	ctx context.Context, queries *sqlcgen.Queries, provider string, title sqlcgen.GetTitleForMatchRow, id string,
	stored []byte,
) error {
	now := matcher.clock.Now().UnixMilli()
	titleID, err := joinExisting(ctx, queries, provider, id, title, now)
	if err != nil {
		return err
	}
	err = queries.UpsertProviderResponse(ctx, sqlcgen.UpsertProviderResponseParams{
		TitleID: titleID, Provider: provider, FetchedAt: now, Body: stored,
	})
	if err != nil {
		return fmt.Errorf("store the response: %w", err)
	}
	err = queries.SetTitleMatch(ctx, sqlcgen.SetTitleMatchParams{
		Source: &provider, SourceID: &id, Ident: identIdentified, ID: titleID,
	})
	if err != nil {
		return fmt.Errorf("record the match: %w", err)
	}
	work := &run{queries: queries, now: now, providers: matcher.providers}
	result, err := work.resolveWithMatch(ctx, titleID, titleSpec{kind: title.Kind})
	if err != nil {
		return err
	}
	return refile(ctx, queries, titleID, result.ident, result.reason)
}

// joinExisting moves a title's versions into another live title of its library that already
// has the matched id, leaves a tombstone pointing there (D173), and returns the title that stays.
func joinExisting(
	ctx context.Context, queries *sqlcgen.Queries, provider, id string, title sqlcgen.GetTitleForMatchRow, now int64,
) (int64, error) {
	lookup := sqlcgen.ListTitlesByProviderIDParams{Provider: provider, ProviderID: id}
	rows, err := queries.ListTitlesByProviderID(ctx, lookup)
	if err != nil {
		return 0, fmt.Errorf("look up %s %s: %w", provider, id, err)
	}
	for _, row := range rows {
		if row.ID == title.ID || row.LibraryID == nil || title.LibraryID == nil || *row.LibraryID != *title.LibraryID {
			continue
		}
		move := sqlcgen.MoveVersionsBetweenTitlesParams{ToTitle: row.ID, FromTitle: title.ID}
		err = queries.MoveVersionsBetweenTitles(ctx, move)
		if err != nil {
			return 0, fmt.Errorf("join title %d to %d: %w", title.ID, row.ID, err)
		}
		err = queries.MergeItem(ctx, sqlcgen.MergeItemParams{MergedInto: &row.ID, RemovedAt: &now, ID: title.ID})
		if err != nil {
			return 0, fmt.Errorf("merge title %d into %d: %w", title.ID, row.ID, err)
		}
		return row.ID, nil
	}
	record := sqlcgen.InsertItemProviderIDParams{ItemID: title.ID, Provider: provider, ProviderID: id}
	err = queries.InsertItemProviderID(ctx, record)
	if err != nil {
		return 0, fmt.Errorf("record %s %s: %w", provider, id, err)
	}
	return title.ID, nil
}

// ambiguous keeps the search results and marks the title for the chooser (D172).
func (matcher *Matcher) ambiguous(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow, candidates []Candidate,
) error {
	err := matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if err := queries.DeleteMatchCandidates(ctx, title.ID); err != nil {
			return fmt.Errorf("clear title %d's candidates: %w", title.ID, err)
		}
		for rank, candidate := range candidates {
			err := queries.InsertMatchCandidate(ctx, cachegen.InsertMatchCandidateParams{
				TitleID: title.ID, Rank: int64(rank), Provider: provider.Name(), ProviderID: candidate.ID,
				Name: optional(candidate.Name), Year: optionalInt(int64(candidate.Year)),
				PosterRef: optional(candidate.PosterRef),
			})
			if err != nil {
				return fmt.Errorf("keep a candidate for title %d: %w", title.ID, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("keep title %d's candidates: %w", title.ID, err)
	}
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		markErr := queries.SetTitleMatch(ctx, sqlcgen.SetTitleMatchParams{Ident: identAmbiguous, ID: title.ID})
		if markErr != nil {
			return fmt.Errorf("mark title %d ambiguous: %w", title.ID, markErr)
		}
		return refile(ctx, queries, title.ID, identAmbiguous, "")
	})
	if err != nil {
		return fmt.Errorf("record title %d's ambiguity: %w", title.ID, err)
	}
	return nil
}

// noMatch records that the provider has nothing for the title, so it is not searched again
// until a person asks.
func (matcher *Matcher) noMatch(ctx context.Context, title sqlcgen.GetTitleForMatchRow) error {
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		err := queries.SetTitleMatch(ctx, sqlcgen.SetTitleMatchParams{
			Source: new(sourceNone), Ident: identUnidentified, ID: title.ID,
		})
		if err != nil {
			return fmt.Errorf("record title %d's empty search: %w", title.ID, err)
		}
		return refile(ctx, queries, title.ID, identUnidentified, noMatchReason(title.MetadataSource))
	})
	if err != nil {
		return fmt.Errorf("record that nothing matched title %d: %w", title.ID, err)
	}
	return nil
}

// refile files a title's live files by its identification, as the pass would.
func refile(ctx context.Context, queries *sqlcgen.Queries, titleID int64, ident, reason string) error {
	files, err := queries.ListLiveFilesOfTitle(ctx, titleID)
	if err != nil {
		return fmt.Errorf("list title %d's files: %w", titleID, err)
	}
	filing := filingListed
	if ident == identUnidentified {
		filing = filingUnidentified
	} else {
		reason = ""
	}
	for _, file := range files {
		params := sqlcgen.SetFileFilingParams{Filing: filing, FilingReason: optional(reason), ID: file.ID}
		err := queries.SetFileFiling(ctx, params)
		if err != nil {
			return fmt.Errorf("file %s: %w", file.Path, err)
		}
	}
	return nil
}

func derefText(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

// Handle is the fetch job's handler. A provider's rate limit defers the queue until its time; a
// key unset or refused holds it until a person fixes the key; an id the provider does not have
// fails the job; anything else retries.
func (matcher *Matcher) Handle(ctx context.Context, job jobs.Job) error {
	outcome, err := matcher.Match(ctx, job.Subject)
	if limited, isLimited := errors.AsType[*RateLimitedError](err); isLimited {
		return &jobs.RetryAtError{At: matcher.clock.Now().Add(limited.After), Err: err}
	}
	switch {
	case errors.Is(err, ErrNoKey), errors.Is(err, ErrKeyRefused):
		return fmt.Errorf("match title %d: %w", job.Subject, jobs.Blocked(err))
	case errors.Is(err, ErrNoMatch):
		return fmt.Errorf("match title %d: %w", job.Subject, jobs.Permanent(err))
	case err != nil:
		return err
	}
	matcher.logger.InfoContext(ctx, "matched a title", "titleId", job.Subject, "outcome", string(outcome))
	return nil
}

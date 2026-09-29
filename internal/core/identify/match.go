// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Matcher matches a film or a series to its library's provider: the fetch job's work (D170,
// D209). A provider id in hand skips the search; a single result matching the name and year is
// accepted; anything else is ambiguous, with the results kept for the chooser. It never guesses
// (D172). A series' episodes then take their values from its season listings, which it fetches.
type Matcher struct {
	database  *store.Database
	cache     *store.Cache
	pass      *Pass
	providers Providers
	clock     clock.Clock
	logger    *slog.Logger
}

// NewMatcher returns the matcher. It reaches the providers through the pass, which re-derives a
// series' episodes once it is matched.
func NewMatcher(
	database *store.Database, cache *store.Cache, pass *Pass, clockSource clock.Clock, logger *slog.Logger,
) *Matcher {
	return &Matcher{
		database: database, cache: cache, pass: pass, providers: pass.providers, clock: clockSource, logger: logger,
	}
}

// Outcome is what a match did.
type Outcome string

// The outcomes of a match.
const (
	OutcomeMatched     Outcome = "matched"
	OutcomeAmbiguous   Outcome = "ambiguous"
	OutcomeNoMatch     Outcome = "no match"
	OutcomeListed      Outcome = "season listings fetched"
	OutcomeNothingToDo Outcome = "nothing to do"
)

// Match matches a film or a series. Provider errors return as they came, for the queue to wait
// or retry.
func (matcher *Matcher) Match(ctx context.Context, itemID int64) (Outcome, error) {
	kind, err := store.One(matcher.database.Read().GetItemKind(ctx, itemID))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return OutcomeNothingToDo, nil // gone since it was queued
	case err != nil:
		return "", fmt.Errorf("read item %d: %w", itemID, err)
	case kind == itemCollection:
		return matcher.matchSeries(ctx, itemID)
	}
	return matcher.matchTitle(ctx, itemID)
}

func (matcher *Matcher) matchTitle(ctx context.Context, titleID int64) (Outcome, error) {
	title, err := store.One(matcher.database.Read().GetTitleForMatch(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return OutcomeNothingToDo, nil // gone since it was queued
	}
	if err != nil {
		return "", fmt.Errorf("read title %d: %w", titleID, err)
	}
	if title.FollowedSource != nil || title.Identification != identUnidentified || title.MetadataSource == sourceNFO {
		return OutcomeNothingToDo, nil // matched, answered, ambiguous or confirmed since it was queued
	}
	provider, known := matcher.providers[providerOf(title.MetadataSource)]
	if !known {
		return "", fmt.Errorf("library source %s: no provider client", title.MetadataSource)
	}
	id, err := matcher.idInHand(ctx, provider, title.Kind, title.ID)
	if err != nil {
		return "", err
	}
	if id != "" {
		return OutcomeMatched, matcher.accept(ctx, provider, title, id, identIdentified)
	}
	query := Query{Kind: title.Kind, Name: ptr.ValueOrZero(title.Name), Year: int(deref(title.Year))}
	candidates, err := provider.Search(ctx, query)
	if err != nil {
		return "", fmt.Errorf("search %s for %q: %w", provider.Name(), query.Name, err)
	}
	if chosen, single := singleMatch(query, candidates, true); single {
		return OutcomeMatched, matcher.accept(ctx, provider, title, chosen.ID, identIdentified)
	}
	if len(candidates) == 0 {
		return OutcomeNoMatch, matcher.noMatch(ctx, title)
	}
	return OutcomeAmbiguous, matcher.ambiguous(ctx, provider, title, candidates)
}

func providerOf(metadataSource string) string {
	return strings.TrimPrefix(metadataSource, "nfo_then_")
}

// idInHand is the provider's id for a film or series from its NFO or its path, if it has one:
// the provider's own, or one found from an IMDb or TVDB id.
func (matcher *Matcher) idInHand(ctx context.Context, provider Provider, kind string, itemID int64) (string, error) {
	ids, err := matcher.database.Read().ListItemProviderIDs(ctx, itemID)
	if err != nil {
		return "", fmt.Errorf("read item %d's ids: %w", itemID, err)
	}
	byProvider := map[string]string{}
	for _, row := range ids {
		byProvider[row.Provider] = row.ProviderID
	}
	if id := byProvider[provider.Name()]; id != "" {
		return id, nil
	}
	for _, source := range []string{sourceIMDb, sourceTVDB} {
		other := byProvider[source]
		if other == "" || (source == sourceTVDB && kind != KindSeries) {
			continue
		}
		id, err := provider.Find(ctx, kind, source, other)
		if errors.Is(err, ErrNoMatch) {
			continue // search instead
		}
		if err != nil {
			return "", fmt.Errorf("find %s %s at %s: %w", source, other, provider.Name(), err)
		}
		return id, nil
	}
	return "", nil
}

// singleMatch is the one candidate whose name and year are the query's (D172). A film with no
// year in hand matches nothing: a wrong match is worse than none. A series' year is checked
// where its folder or tvshow.nfo gives one (D209).
func singleMatch(query Query, candidates []Candidate, yearRequired bool) (Candidate, bool) {
	if query.Year == 0 && yearRequired {
		return Candidate{}, false
	}
	var found []Candidate
	want := foldName(query.Name)
	for _, candidate := range candidates {
		named := foldName(candidate.Name) == want || foldName(candidate.OriginalName) == want
		if named && (query.Year == 0 || candidate.Year == query.Year) {
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

// accept fetches the work, stores the response and re-derives the title from it, identified by
// the matcher or confirmed by a person. A title that turns out to be one already in the library
// joins it.
func (matcher *Matcher) accept(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow, id, ident string,
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
	// Read before the write: a setting read may write its starting value, and there is one writer.
	sorter, err := matcher.pass.sorter(ctx)
	if err != nil {
		return err
	}
	var owed Report
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		found := match{provider: provider.Name(), id: metadata.ID, ident: ident, artwork: metadata.Artwork}
		owed = Report{}
		return matcher.record(ctx, queries, found, title, stored, derivation{sorter: sorter, report: &owed})
	})
	if err != nil {
		return fmt.Errorf("record title %d's match: %w", title.ID, err)
	}
	matcher.pass.owe(ctx, owed)
	return nil
}

// derivation is how a match re-derives its title: the locale it sorts in, and the report it notes
// what the title owes in.
type derivation struct {
	sorter *textindex.Sorter
	report *Report
}

// match is what a title was matched to, and by whom: identified by the matcher, confirmed by a
// person.
type match struct {
	provider string
	id       string
	ident    string
	// artwork is the work's images, whose refs a rematch keeps of the item's earlier ones.
	artwork []ArtRef
}

// record stores a match's response and re-derives the title from it, as a pass would.
func (matcher *Matcher) record(
	ctx context.Context, queries *sqlcgen.Queries, found match, title sqlcgen.GetTitleForMatchRow, stored []byte,
	derive derivation,
) error {
	provider, id := found.provider, found.id
	now := matcher.clock.Now().UnixMilli()
	if found.ident == identConfirmed {
		forget := sqlcgen.DeleteItemProviderIDsOfParams{ItemID: title.ID, Provider: provider}
		err := queries.DeleteItemProviderIDsOf(ctx, forget)
		if err != nil {
			return fmt.Errorf("forget title %d's earlier %s match: %w", title.ID, provider, err)
		}
	}
	if isRematch := ptr.ValueOrZero(title.FollowedSource) == provider &&
		ptr.ValueOrZero(title.FollowedSourceID) != id &&
		title.FollowedSourceID != nil; isRematch {
		if err := retireArtworkOf(ctx, queries, title.ID, provider, found.artwork, now); err != nil {
			return err
		}
	}
	titleID, err := joinExisting(ctx, queries, provider, id, title, now)
	if err != nil {
		return err
	}
	err = queries.UpsertProviderResponse(ctx, sqlcgen.UpsertProviderResponseParams{
		ItemID: titleID, Provider: provider, Part: "", FetchedAt: now, BodyGzip: stored,
	})
	if err != nil {
		return fmt.Errorf("store the response: %w", err)
	}
	err = queries.SetTitleMatch(ctx, sqlcgen.SetTitleMatchParams{
		FollowedSource: &provider, FollowedSourceID: &id, Identification: found.ident, ID: titleID,
	})
	if err != nil {
		return fmt.Errorf("record the match: %w", err)
	}
	work := &run{queries: queries, now: now, report: derive.report, providers: matcher.providers, sorter: derive.sorter}
	result, err := work.resolveWithMatch(ctx, titleID, titleSpec{kind: title.Kind})
	if err != nil {
		return err
	}
	return refile(ctx, queries, titleID, result.ident, result.reason)
}

// retireArtworkOf retires an item's images from a provider that its new work does not have: a
// rematch to another work leaves none of the earlier work's kinds behind (D223 §9).
func retireArtworkOf(
	ctx context.Context, queries *sqlcgen.Queries, itemID int64, provider string, kept []ArtRef, now int64,
) error {
	refs := make([]string, 0, len(kept))
	for _, ref := range kept {
		refs = append(refs, ref.Ref)
	}
	encoded, err := json.Marshal(refs)
	if err != nil {
		return fmt.Errorf("encode item %d's artwork: %w", itemID, err)
	}
	err = queries.RetireArtworkNotFound(ctx, sqlcgen.RetireArtworkNotFoundParams{
		RemovedAt: &now, ItemID: itemID, Source: provider, FoundRefs: string(encoded),
	})
	if err != nil {
		return fmt.Errorf("retire item %d's earlier work's artwork: %w", itemID, err)
	}
	return nil
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
	if err := matcher.keep(ctx, provider, title.ID, candidates); err != nil {
		return err
	}
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		markErr := queries.SetTitleMatch(ctx, sqlcgen.SetTitleMatchParams{Identification: identAmbiguous, ID: title.ID})
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
			FollowedSource: new(sourceNone), Identification: identUnidentified, ID: title.ID,
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

// keep replaces a film's or series' kept search results in cache.db.
func (matcher *Matcher) keep(ctx context.Context, provider Provider, itemID int64, candidates []Candidate) error {
	err := matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if err := queries.DeleteMatchCandidates(ctx, itemID); err != nil {
			return fmt.Errorf("clear item %d's candidates: %w", itemID, err)
		}
		for rank, candidate := range candidates {
			err := queries.InsertMatchCandidate(ctx, cachegen.InsertMatchCandidateParams{
				ItemID: itemID, Rank: int64(rank), Provider: provider.Name(), ProviderID: candidate.ID,
				Name: optional(candidate.Name), Year: optionalInt(int64(candidate.Year)),
				PosterPath: optional(candidate.PosterRef),
			})
			if err != nil {
				return fmt.Errorf("keep a candidate for item %d: %w", itemID, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("keep item %d's candidates: %w", itemID, err)
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

// Handle is the fetch job's handler. A provider's rate limit defers the queue until its time; a
// key unset or refused holds it until a person fixes the key; an id the provider does not have
// fails the job; anything else retries.
func (matcher *Matcher) Handle(ctx context.Context, job jobs.Job) error {
	var args refreshArgs
	if err := json.Unmarshal([]byte(cmp.Or(job.Args, "{}")), &args); err != nil {
		return fmt.Errorf("fetch job %d: %w", job.ID, jobs.Permanent(err))
	}
	var outcome Outcome
	var err error
	if args.IsRefresh {
		outcome, err = matcher.refresh(ctx, job.ItemID, args.RetryImages)
	} else {
		outcome, err = matcher.Match(ctx, job.ItemID)
	}
	if limited, isLimited := errors.AsType[*RateLimitedError](err); isLimited {
		return &jobs.RetryAtError{At: matcher.clock.Now().Add(limited.After), Err: err}
	}
	switch {
	case errors.Is(err, ErrNoKey), errors.Is(err, ErrKeyRefused):
		return fmt.Errorf("match title %d: %w", job.ItemID, jobs.Blocked(err))
	case errors.Is(err, ErrNoMatch):
		return fmt.Errorf("match title %d: %w", job.ItemID, jobs.Permanent(err))
	case err != nil:
		return err
	}
	matcher.logger.InfoContext(ctx, "matched a title", "titleId", job.ItemID, "outcome", string(outcome))
	return nil
}

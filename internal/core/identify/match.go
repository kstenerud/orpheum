// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Matcher matches a film or a series to its library's provider: the fetch_metadata job's work (D170,
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
	// imageRetrier asks for an item's missing images again: the artwork service, set by the
	// server's wiring.
	imageRetrier ImageRetrier
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
	OutcomeMatched         Outcome = "matched"
	OutcomeAmbiguous       Outcome = "ambiguous"
	OutcomeNoMatch         Outcome = "no match"
	OutcomeListingsFetched Outcome = "season listings fetched"
	OutcomeNothingToDo     Outcome = "nothing to do"
)

// Match matches a film or a series. Provider errors return as they came, for the queue to wait
// or retry.
func (matcher *Matcher) Match(ctx context.Context, itemID int64) (Outcome, error) {
	kind, err := store.OrNotFound(matcher.database.Read().GetItemKind(ctx, itemID))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return OutcomeNothingToDo, nil // gone since it was queued
	case err != nil:
		return "", fmt.Errorf("read item %d: %w", itemID, err)
	case kind == itemKindCollection:
		return matcher.matchSeries(ctx, itemID)
	}
	return matcher.matchTitle(ctx, itemID)
}

func (matcher *Matcher) matchTitle(ctx context.Context, titleID int64) (Outcome, error) {
	title, err := store.OrNotFound(matcher.database.Read().GetTitleForMatch(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return OutcomeNothingToDo, nil // gone since it was queued
	}
	if err != nil {
		return "", fmt.Errorf("read title %d: %w", titleID, err)
	}
	if title.FollowedSource != nil || title.IdentificationState != identificationUnidentified ||
		title.MetadataSource == sourceNFO {
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
		return OutcomeMatched, matcher.accept(ctx, provider, title, id, identificationIdentified)
	}
	query, candidates, err := searchWithSpellings(ctx, provider,
		Query{Kind: title.Kind, Name: ptr.ValueOrZero(title.Name), Year: int(ptr.ValueOrZero(title.Year))})
	if err != nil {
		return "", err
	}
	candidates, fit, err := matcher.fitFilm(ctx, provider, title.ID, query, candidates)
	if err != nil {
		return "", err
	}
	chosen, how := settleMatch(query, candidates, true, fit.lengths)
	matcher.logFilmSettled(ctx, title.ID, query, candidates, fit, settlement{chosen: chosen, how: how})
	settled := answer{id: chosen.ID, identification: identificationIdentified, body: fit.fetched[chosen.ID]}
	switch how {
	case clearly:
		return OutcomeMatched, matcher.acceptAnswer(ctx, provider, title, settled)
	case byGuess:
		return OutcomeMatched, matcher.acceptGuess(ctx, provider, title, settled, candidates)
	case unsettled:
	}
	if len(candidates) == 0 {
		return OutcomeNoMatch, matcher.markNoMatch(ctx, title)
	}
	return OutcomeAmbiguous, matcher.markAmbiguous(ctx, provider, title, candidates)
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
	return idOfIDs(ctx, provider, kind, byProvider)
}

// idOfIDs is the provider's id among ids, by provider, if they hold one: the provider's own, or one
// it finds from an IMDb or TVDB id.
func idOfIDs(ctx context.Context, provider Provider, kind string, byProvider map[string]string) (string, error) {
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

// ReconsiderAmbiguous returns to their first match the films and series left ambiguous whose kept
// search results the rules now settle: a rule that changed since they were searched would settle
// them, and nothing searched them again (the owner's NAS, 2026-10-13, where a film's single TMDB
// match, its ampersand folded, waited for a person's choice). It reads only persistent.db and cache.db,
// asking no provider; the first-match sweep then searches each once more, and accepts what the
// rules settle. A person's choice is never undone. It says how many it returned.
func (matcher *Matcher) ReconsiderAmbiguous(ctx context.Context) (int, error) {
	waiting, err := matcher.ambiguousItems(ctx)
	if err != nil {
		return 0, err
	}
	var settled []reconsidered
	for _, item := range waiting {
		isSettled, settleErr := matcher.isSettledByKept(ctx, item.id, item.query)
		if settleErr != nil {
			return 0, settleErr
		}
		if isSettled {
			settled = append(settled, item)
		}
	}
	if len(settled) == 0 {
		return 0, nil
	}
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, item := range settled {
			returnErr := queries.ReturnAmbiguousTitleToItsFirstMatch(ctx, item.id)
			if item.query.Kind == KindSeries {
				returnErr = queries.ReturnAmbiguousSeriesToItsFirstMatch(ctx, item.id)
			}
			if returnErr != nil {
				return fmt.Errorf("return item %d to its first match: %w", item.id, returnErr)
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("return the settled to their first match: %w", err)
	}
	return len(settled), nil
}

// reconsidered is an ambiguous film or series, and the query its kept results answered.
type reconsidered struct {
	id    int64
	query Query
}

// ambiguousItems are the films and series left ambiguous, with a live file, and their queries.
func (matcher *Matcher) ambiguousItems(ctx context.Context) ([]reconsidered, error) {
	queries := matcher.database.Read()
	titles, err := queries.ListTitlesByIdentification(ctx, sqlcgen.ListTitlesByIdentificationParams{
		IdentificationState: identificationAmbiguous, IsEveryLibrary: 1, LibraryIdsJSON: "[]",
	})
	if err != nil {
		return nil, fmt.Errorf("list the ambiguous titles: %w", err)
	}
	series, err := queries.ListSeriesByIdentification(ctx, sqlcgen.ListSeriesByIdentificationParams{
		IdentificationState: ptr.NilIfZero(identificationAmbiguous), IsEveryLibrary: 1, LibraryIdsJSON: "[]",
	})
	if err != nil {
		return nil, fmt.Errorf("list the ambiguous series: %w", err)
	}
	items := make([]reconsidered, 0, len(titles)+len(series))
	for _, title := range titles {
		if title.Kind == KindMovie {
			items = append(items, reconsidered{id: title.ID, query: Query{
				Kind: KindMovie, Name: ptr.ValueOrZero(title.Name), Year: int(ptr.ValueOrZero(title.Year)),
			}})
		}
	}
	for _, one := range series {
		items = append(items, reconsidered{id: one.ID, query: Query{
			Kind: KindSeries, Name: ptr.ValueOrZero(one.Name), Year: int(ptr.ValueOrZero(one.Year)),
		}})
	}
	return items, nil
}

// isSettledByKept is whether an item's kept search results settle its match (settleMatch), or
// whether none are kept, as after cache.db was set aside: then it is searched once more, as the
// chooser would search it (Chooser.Candidates).
func (matcher *Matcher) isSettledByKept(ctx context.Context, itemID int64, query Query) (bool, error) {
	candidates, _, err := matcher.keptCandidates(ctx, itemID)
	if err != nil {
		return false, err
	}
	if len(candidates) == 0 {
		return true, nil
	}
	var lengths []fileLength
	if query.Kind == KindMovie {
		if lengths, err = matcher.fileLengths(ctx, itemID); err != nil {
			return false, err
		}
	}
	_, how := settleMatch(query, candidates, query.Kind == KindMovie, lengths)
	return how != unsettled, nil
}

// keptCandidates are an item's kept search results, best first, and the provider that gave them.
func (matcher *Matcher) keptCandidates(ctx context.Context, itemID int64) ([]Candidate, string, error) {
	kept, err := matcher.cache.Read().ListMatchCandidates(ctx, itemID)
	if err != nil {
		return nil, "", fmt.Errorf("read item %d's candidates: %w", itemID, err)
	}
	provider := ""
	candidates := make([]Candidate, 0, len(kept))
	for _, row := range kept {
		provider = row.Provider
		candidates = append(candidates, Candidate{
			ID: row.ProviderID, Name: ptr.ValueOrZero(row.Name), Year: int(ptr.ValueOrZero(row.Year)),
			ProviderPosterPath: ptr.ValueOrZero(row.ProviderPosterPath), Votes: int(ptr.ValueOrZero(row.Votes)),
			RuntimeMs: ptr.ValueOrZero(row.RuntimeMs),
		})
	}
	return candidates, provider, nil
}

// accept fetches the work, stores the response and re-derives the title from it, identified by
// the matcher or confirmed by a person. A title that turns out to be one already in the library
// joins it.
func (matcher *Matcher) accept(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow, id, identification string,
) error {
	return matcher.acceptAnswer(ctx, provider, title, answer{id: id, identification: identification})
}

// answer is the work a match settled on, and by whom: identified by the matcher or confirmed by a
// person. A rescan's answer replaces every id an earlier match recorded, keeping only those its
// files give, keptIDs (D357 §4); nil keeps them all. Its body is the work's details where settling
// it fetched them already, for its runtime (D365 §4); nil fetches them.
type answer struct {
	id             string
	identification string
	keptIDs        map[string]string
	body           []byte
}

// acceptAnswer is accept, of an answer.
func (matcher *Matcher) acceptAnswer(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow, settled answer,
) error {
	id, identification := settled.id, settled.identification
	body := settled.body
	if body == nil {
		fetched, err := provider.Fetch(ctx, title.Kind, id)
		if err != nil {
			return fmt.Errorf("fetch %s %s: %w", provider.Name(), id, err)
		}
		body = fetched
	}
	metadata, err := provider.Parse(title.Kind, body)
	if err != nil {
		return fmt.Errorf("read %s %s: %w", provider.Name(), id, err)
	}
	bodyGzip, err := bounded.Gzip(body)
	if err != nil {
		return fmt.Errorf("keep the provider's answer: %w", err)
	}
	// Read before the write: a setting read may write its starting value, and there is one writer.
	sorter, err := matcher.pass.sorter(ctx)
	if err != nil {
		return err
	}
	var owed Report
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		found := titleMatch{
			provider: provider.Name(), id: metadata.ID, identification: identification, artwork: metadata.Artwork,
			keptIDs: settled.keptIDs,
		}
		owed = Report{}
		return matcher.recordMatch(ctx, queries, found, title, bodyGzip, derivation{sorter: sorter, report: &owed})
	})
	if err != nil {
		return fmt.Errorf("record title %d's match: %w", title.ID, err)
	}
	matcher.pass.runAfterPassHooks(ctx, owed)
	return nil
}

// derivation is how a match re-derives its title: the locale it sorts in, and the report it notes
// what the title owes in.
type derivation struct {
	sorter *textindex.Sorter
	report *Report
}

// titleMatch is what a title was matched to, and by whom: identified by the matcher, confirmed by a
// person.
type titleMatch struct {
	provider       string
	id             string
	identification string
	// artwork is the work's images, whose refs a rematch keeps of the item's earlier ones.
	artwork []ImageRef
	// keptIDs, when not nil, are the only ids of the title's that stay: a rescan's new answer
	// replaces every id the old one recorded (D357 §4).
	keptIDs map[string]string
}

// recordMatch stores a match's response and re-derives the title from it, as a pass would.
func (matcher *Matcher) recordMatch(
	ctx context.Context, queries *sqlcgen.Queries, found titleMatch, title sqlcgen.GetTitleForMatchRow, bodyGzip []byte,
	derive derivation,
) error {
	provider, id := found.provider, found.id
	now := matcher.clock.Now().UnixMilli()
	if found.keptIDs != nil {
		if err := forgetIDsBut(ctx, queries, title.ID, found.keptIDs); err != nil {
			return err
		}
	}
	if found.identification == identificationConfirmed {
		forget := sqlcgen.DeleteItemProviderIDsAtProviderParams{ItemID: title.ID, Provider: provider}
		err := queries.DeleteItemProviderIDsAtProvider(ctx, forget)
		if err != nil {
			return fmt.Errorf("forget title %d's earlier %s match: %w", title.ID, provider, err)
		}
	}
	if isRematch := ptr.ValueOrZero(title.FollowedSource) == provider &&
		ptr.ValueOrZero(title.FollowedSourceID) != id &&
		title.FollowedSourceID != nil; isRematch {
		derive.report.Rematched = append(derive.report.Rematched,
			NamedImages{ItemID: title.ID, Source: provider, Refs: found.artwork})
	}
	titleID, err := joinExisting(ctx, queries, provider, id, title, now)
	if err != nil {
		return err
	}
	err = queries.UpsertProviderResponse(ctx, sqlcgen.UpsertProviderResponseParams{
		ItemID: titleID, Provider: provider, Part: "", FetchedAt: now, BodyGzip: bodyGzip,
	})
	if err != nil {
		return fmt.Errorf("store the response: %w", err)
	}
	err = queries.SetTitleMatch(ctx, sqlcgen.SetTitleMatchParams{
		FollowedSource: &provider, FollowedSourceID: &id, IdentificationState: found.identification, ID: titleID,
	})
	if err != nil {
		return fmt.Errorf("record the match: %w", err)
	}
	filer := &libraryFiler{
		queries: queries, nowMs: now, report: derive.report, providers: matcher.providers, sorter: derive.sorter,
	}
	resolution, err := filer.resolveWithMatch(ctx, titleID, titleSpec{kind: title.Kind})
	if err != nil {
		return err
	}
	return refile(ctx, queries, titleID, resolution.identification, resolution.reason)
}

// joinableTitle says whether a title the matched id names is one a title can join: another live
// title of its library, or a title of its kind whose library lists none of its files any more, a
// deleted library's, which comes back (D231 §7, D363 §10).
func joinableTitle(
	row sqlcgen.ListTitlesByProviderIDRow, title sqlcgen.GetTitleForMatchRow,
) (isJoinable, isDeletedLibrarys bool) {
	if row.ID == title.ID || title.FilingLibraryID == nil {
		return false, false
	}
	if row.IsFiledByNone == 1 {
		return row.Kind == title.Kind, true
	}
	return row.FilingLibraryID != nil && *row.FilingLibraryID == *title.FilingLibraryID, false
}

// joinExisting moves a title's versions into another live title of its library that already
// has the matched id, or into a deleted library's title that has it, which comes back into this
// library with its history (D208 §2, D231 §7); it leaves a tombstone pointing there (D173), and
// returns the title that stays.
func joinExisting(
	ctx context.Context, queries *sqlcgen.Queries, provider, id string, title sqlcgen.GetTitleForMatchRow, now int64,
) (int64, error) {
	lookup := sqlcgen.ListTitlesByProviderIDParams{Provider: provider, ProviderID: id}
	rows, err := queries.ListTitlesByProviderID(ctx, lookup)
	if err != nil {
		return 0, fmt.Errorf("look up %s %s: %w", provider, id, err)
	}
	for _, row := range rows {
		isJoinable, isDeletedLibrarys := joinableTitle(row, title)
		if !isJoinable {
			continue
		}
		if isDeletedLibrarys {
			back := sqlcgen.MoveItemToLibraryParams{LibraryID: title.FilingLibraryID, ID: row.ID}
			if moveErr := queries.MoveItemToLibrary(ctx, back); moveErr != nil {
				return 0, fmt.Errorf("bring title %d back: %w", row.ID, moveErr)
			}
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

// markAmbiguous keeps the search results and marks the title for the chooser (D172).
func (matcher *Matcher) markAmbiguous(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow, candidates []Candidate,
) error {
	if err := matcher.keepCandidates(ctx, provider, title.ID, candidates); err != nil {
		return err
	}
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		markErr := queries.SetTitleMatch(ctx, sqlcgen.SetTitleMatchParams{
			IdentificationState: identificationAmbiguous,
			ID:                  title.ID,
		})
		if markErr != nil {
			return fmt.Errorf("mark title %d ambiguous: %w", title.ID, markErr)
		}
		return refile(ctx, queries, title.ID, identificationAmbiguous, "")
	})
	if err != nil {
		return fmt.Errorf("record title %d's ambiguity: %w", title.ID, err)
	}
	return nil
}

// acceptGuess accepts a film's match the matcher guessed among candidates (D301): matched as any
// other, its candidates kept for a person correcting it, and marked a guess until one does.
func (matcher *Matcher) acceptGuess(
	ctx context.Context, provider Provider, title sqlcgen.GetTitleForMatchRow, guessed answer, candidates []Candidate,
) error {
	if err := matcher.acceptAnswer(ctx, provider, title, guessed); err != nil {
		return err
	}
	return matcher.markGuess(ctx, provider, title.ID, candidates)
}

// markGuess keeps an item's candidates and marks its match a guess (D301 §2, §4).
func (matcher *Matcher) markGuess(ctx context.Context, provider Provider, itemID int64, candidates []Candidate) error {
	if err := matcher.keepCandidates(ctx, provider, itemID, candidates); err != nil {
		return err
	}
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.MarkGuessedMatch(ctx, sqlcgen.MarkGuessedMatchParams{
			ItemID: itemID, GuessedAt: matcher.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("mark item %d's match a guess: %w", itemID, err)
	}
	return nil
}

// markNoMatch records that the provider has nothing for the title, so it is not searched again
// until a person asks.
func (matcher *Matcher) markNoMatch(ctx context.Context, title sqlcgen.GetTitleForMatchRow) error {
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		err := queries.SetTitleMatch(ctx, sqlcgen.SetTitleMatchParams{
			FollowedSource: new(sourceNone), IdentificationState: identificationUnidentified, ID: title.ID,
		})
		if err != nil {
			return fmt.Errorf("record title %d's empty search: %w", title.ID, err)
		}
		return refile(ctx, queries, title.ID, identificationUnidentified, noMatchReason(title.MetadataSource))
	})
	if err != nil {
		return fmt.Errorf("record that nothing matched title %d: %w", title.ID, err)
	}
	return nil
}

// keepCandidates replaces a film's or series' kept search results in cache.db.
func (matcher *Matcher) keepCandidates(
	ctx context.Context, provider Provider, itemID int64, candidates []Candidate,
) error {
	err := matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if err := queries.DeleteMatchCandidates(ctx, itemID); err != nil {
			return fmt.Errorf("clear item %d's candidates: %w", itemID, err)
		}
		for rank, candidate := range candidates {
			err := queries.InsertMatchCandidate(ctx, cachegen.InsertMatchCandidateParams{
				ItemID: itemID, Rank: int64(rank), Provider: provider.Name(), ProviderID: candidate.ID,
				Name: ptr.NilIfZero(candidate.Name), Year: ptr.NilIfZero(int64(candidate.Year)),
				ProviderPosterPath: ptr.NilIfZero(candidate.ProviderPosterPath),
				Votes:              ptr.NilIfZero(int64(candidate.Votes)),
				RuntimeMs:          ptr.NilIfZero(candidate.RuntimeMs),
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

// dropCandidates forgets an item's kept candidates: chosen from, searched for under a name or year
// that is no longer the item's, so the next look searches again, or of a match settled clearly
// (D301).
func (matcher *Matcher) dropCandidates(ctx context.Context, itemID int64) error {
	err := matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.DeleteMatchCandidates(ctx, itemID)
	})
	if err != nil {
		return fmt.Errorf("drop item %d's candidates: %w", itemID, err)
	}
	return nil
}

// refile files a title's live files by its identification, as the pass would.
func refile(ctx context.Context, queries *sqlcgen.Queries, titleID int64, identification, reason string) error {
	files, err := queries.ListLiveFilesOfTitle(ctx, titleID)
	if err != nil {
		return fmt.Errorf("list title %d's files: %w", titleID, err)
	}
	filing := library.FilingListed
	if identification == identificationUnidentified {
		filing = library.FilingUnidentified
	} else {
		reason = ""
	}
	for _, file := range files {
		err := library.FileAs(ctx, queries, file.ID, file.LibraryKind, filing, reason)
		if err != nil {
			return fmt.Errorf("file %s: %w", file.Path, err)
		}
	}
	return nil
}

// Handle is the fetch_metadata job's handler. It asks the provider with the job's importance: a
// person's refresh above the background (providers.JobImportance). A provider that cannot be
// reached, asks us to wait or has its key unset or refused answers that the job parks on it (its
// provider service's doing, D231 §5); an id the provider does not have fails the job, as the
// item's own failure (RecordFailure); anything else retries. A person's refresh waiting for the
// item (refresh_request) makes whichever fetch runs a refresh that asks for what they asked, and
// a fetch that succeeds answers it and clears a failure of the item's own (D231 §4).
func (matcher *Matcher) Handle(ctx context.Context, job jobs.Job) error {
	ctx = providers.WithImportance(ctx, providers.JobImportance(job.Priority))
	args, err := decodeFetchArgs(cmp.Or(job.ArgsJSON, "{}"))
	if err != nil {
		return fmt.Errorf("fetch_metadata job %d: %w", job.ID, jobs.MarkPermanent(err))
	}
	request, err := store.OrNotFound(matcher.database.Read().GetRefreshRequest(ctx, job.ItemID))
	hasRequest := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("read item %d's refresh request: %w", job.ItemID, err)
	}
	if rematch, isWaiting, rematchErr := matcher.pendingRematch(ctx, job.ItemID); rematchErr != nil {
		return rematchErr
	} else if isWaiting {
		return matcher.handleRematch(ctx, job.ItemID, rematch)
	}
	var outcome Outcome
	if args.IsRefresh || hasRequest {
		outcome, err = matcher.refresh(ctx, job.ItemID, args.ShouldRetryImages || request.ShouldFetchImages != 0)
	} else {
		outcome, err = matcher.Match(ctx, job.ItemID)
	}
	switch {
	case errors.Is(err, ErrNoMatch):
		return fmt.Errorf("fetch item %d: %w", job.ItemID, jobs.MarkPermanent(err))
	case err != nil:
		return err
	}
	if err := matcher.settle(ctx, job.ItemID, request.RequestedAt); err != nil {
		return err
	}
	matcher.logger.InfoContext(ctx, "matched an item", "itemId", job.ItemID, "outcome", string(outcome))
	return nil
}

// settle records that a fetch of an item succeeded: a failure of its own is cleared, and a
// person's refresh the fetch read (requested at or before readAt) is answered; one made since
// stays for the next.
func (matcher *Matcher) settle(ctx context.Context, itemID, readAt int64) error {
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if err := queries.ClearFetchFailure(ctx, itemID); err != nil {
			return fmt.Errorf("clear its failure: %w", err)
		}
		err := queries.SettleRefreshRequest(ctx, sqlcgen.SettleRefreshRequestParams{ItemID: itemID, ReadAt: readAt})
		if err != nil {
			return fmt.Errorf("answer its refresh: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("record item %d's fetch: %w", itemID, err)
	}
	return nil
}

// RecordFailure records a fetch_metadata job that failed as its item's own failure (D231 §4,
// §10): an id its provider does not have, or its attempts spent on a failure that was not its
// host's being down, which parks instead. The sweeps pass the item over, its page shows why, and
// a person's refresh, "try again", clears it; the refresh it answers is settled.
func (matcher *Matcher) RecordFailure(ctx context.Context, job jobs.Job, failure string) error {
	now := matcher.clock.Now().UnixMilli()
	err := matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := store.OrNotFound(queries.GetItemKind(ctx, job.ItemID)); errors.Is(err, store.ErrNotFound) {
			return nil // gone since it was queued
		} else if err != nil {
			return fmt.Errorf("read it: %w", err)
		}
		err := queries.RecordFetchFailure(ctx, sqlcgen.RecordFetchFailureParams{
			ItemID: job.ItemID, Failure: failure, FailedAt: now,
		})
		if err != nil {
			return fmt.Errorf("keep why: %w", err)
		}
		err = queries.SettleRefreshRequest(ctx, sqlcgen.SettleRefreshRequestParams{ItemID: job.ItemID, ReadAt: now})
		if err != nil {
			return fmt.Errorf("answer its refresh: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("record item %d's failure: %w", job.ItemID, err)
	}
	matcher.logger.WarnContext(ctx, "an item could not be fetched; it waits for a person", "itemId", job.ItemID,
		"failure", failure)
	if rematch, isWaiting, rematchErr := matcher.pendingRematch(ctx, job.ItemID); rematchErr != nil {
		return rematchErr
	} else if isWaiting {
		return matcher.endRematch(ctx, job.ItemID, rematch.RescanID, rematchFailed)
	}
	return nil
}

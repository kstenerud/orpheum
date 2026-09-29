// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Refusals a person's request can meet.
var (
	// ErrNoItem is a film, episode or series that does not exist or was removed.
	ErrNoItem = errors.New("no such title or series")
	// ErrNotChoosable is an episode: its series is what is matched (D209).
	ErrNotChoosable = errors.New("an episode takes its match from its series; choose the series'")
	// ErrUnknownProvider is a provider this server has no client for.
	ErrUnknownProvider = errors.New("no such provider")
	// ErrUnknownField is a field that cannot be overridden on this item.
	ErrUnknownField = errors.New("no such field")
	// ErrBadValue is an override value the field cannot hold.
	ErrBadValue = errors.New("the value does not fit the field")
	// ErrNoOverride is a field with no override to remove.
	ErrNoOverride = errors.New("the field has no override")
)

// IdentificationStates are the states of identification a list can ask for.
func IdentificationStates() []string {
	return []string{
		identificationIdentified, identificationAmbiguous, identificationUnidentified, identificationConfirmed,
	}
}

// OverridableFields are the fields a person can override on a title (D183); a series has fewer.
func OverridableFields() []string {
	return []string{fieldName, fieldOverview, fieldTagline, fieldYear, fieldRuntimeMs}
}

func seriesOverridableFields() []string { return []string{fieldName, fieldOverview, fieldYear} }

// Chooser is what a person does about identification: see the titles and series in a state,
// see an ambiguous one's candidates, choose its match, and override a field (D172, D183, D209).
type Chooser struct {
	matcher *Matcher
	pass    *Pass
}

// NewChooser returns the chooser.
func NewChooser(matcher *Matcher, pass *Pass) *Chooser {
	return &Chooser{matcher: matcher, pass: pass}
}

// ItemSummary is one title or series in a list.
type ItemSummary struct {
	ID                  int64
	LibraryID           int64
	Kind                string // a title's kind, or "series"
	Name                string
	Year                int64
	IdentificationState string
	Source              string
	SourceID            string
}

// Items lists the titles and series in one state of identification, by name.
func (chooser *Chooser) Items(ctx context.Context, identification string) ([]ItemSummary, error) {
	queries := chooser.matcher.database.Read()
	titles, err := queries.ListTitlesByIdentification(ctx, identification)
	if err != nil {
		return nil, fmt.Errorf("list %s titles: %w", identification, err)
	}
	series, err := queries.ListSeriesByIdentification(ctx, &identification)
	if err != nil {
		return nil, fmt.Errorf("list %s series: %w", identification, err)
	}
	items := make([]ItemSummary, 0, len(titles)+len(series))
	for _, row := range titles {
		items = append(items, ItemSummary{
			ID: row.ID, LibraryID: ptr.ValueOrZero(row.LibraryID), Kind: row.Kind, Name: ptr.ValueOrZero(row.Name),
			Year: ptr.ValueOrZero(row.Year), IdentificationState: row.IdentificationState,
			Source:   ptr.ValueOrZero(row.FollowedSource),
			SourceID: ptr.ValueOrZero(row.FollowedSourceID),
		})
	}
	for _, row := range series {
		items = append(items, ItemSummary{
			ID: row.ID, LibraryID: ptr.ValueOrZero(row.LibraryID), Kind: KindSeries, Name: ptr.ValueOrZero(row.Name),
			Year: ptr.ValueOrZero(row.Year), IdentificationState: ptr.ValueOrZero(row.IdentificationState),
			Source:   ptr.ValueOrZero(row.FollowedSource),
			SourceID: ptr.ValueOrZero(row.FollowedSourceID),
		})
	}
	slices.SortStableFunc(items, func(a, b ItemSummary) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return items, nil
}

// target is the title or series a request names.
type target struct {
	title    sqlcgen.GetTitleForMatchRow
	series   sqlcgen.GetSeriesForMatchRow
	isSeries bool
}

func (item target) id() int64 {
	if item.isSeries {
		return item.series.ID
	}
	return item.title.ID
}

func (item target) searchQuery() Query {
	if item.isSeries {
		return Query{
			Kind: KindSeries, Name: ptr.ValueOrZero(item.series.Name), Year: int(ptr.ValueOrZero(item.series.Year)),
		}
	}
	return Query{
		Kind: item.title.Kind, Name: ptr.ValueOrZero(item.title.Name), Year: int(ptr.ValueOrZero(item.title.Year)),
	}
}

func (item target) identification() string {
	if item.isSeries {
		return ptr.ValueOrZero(item.series.IdentificationState)
	}
	return item.title.IdentificationState
}

func (item target) metadataSource() string {
	if item.isSeries {
		return item.series.MetadataSource
	}
	return item.title.MetadataSource
}

func (item target) libraryID() *int64 {
	if item.isSeries {
		return item.series.LibraryID
	}
	return item.title.LibraryID
}

// Candidates are a film's or series' kept search results, best first. When cache.db lost them,
// an ambiguous one is searched once more (schema.md, cache.db).
func (chooser *Chooser) Candidates(ctx context.Context, itemID int64) ([]Candidate, string, error) {
	item, err := chooser.readTarget(ctx, itemID)
	if err != nil {
		return nil, "", err
	}
	kept, err := chooser.matcher.cache.Read().ListMatchCandidates(ctx, itemID)
	if err != nil {
		return nil, "", fmt.Errorf("read item %d's candidates: %w", itemID, err)
	}
	provider := ""
	candidates := make([]Candidate, 0, len(kept))
	for _, row := range kept {
		provider = row.Provider
		candidates = append(candidates, Candidate{
			ID: row.ProviderID, Name: ptr.ValueOrZero(row.Name), Year: int(ptr.ValueOrZero(row.Year)),
			PosterRef: ptr.ValueOrZero(row.ProviderPosterPath),
		})
	}
	if len(candidates) > 0 || item.identification() != identificationAmbiguous {
		return candidates, provider, nil
	}
	return chooser.searchAgain(ctx, item)
}

func (chooser *Chooser) searchAgain(ctx context.Context, item target) ([]Candidate, string, error) {
	provider, known := chooser.matcher.providers[providerOf(item.metadataSource())]
	if !known {
		return []Candidate{}, "", nil // an NFO library's item has no provider to ask
	}
	query := item.searchQuery()
	candidates, err := provider.Search(ctx, query)
	if err != nil {
		return nil, "", fmt.Errorf("search %s for %q: %w", provider.Name(), query.Name, err)
	}
	if err := chooser.matcher.keepCandidates(ctx, provider, item.id(), candidates); err != nil {
		return nil, "", err
	}
	return candidates, provider.Name(), nil
}

// Choose matches a film or series to a work a person chose: fetched, stored and permanent, over
// any NFO and every later pass (D172, D209). The candidates are no longer needed.
func (chooser *Chooser) Choose(ctx context.Context, itemID int64, providerName, providerID string) error {
	item, err := chooser.readTarget(ctx, itemID)
	if err != nil {
		return err
	}
	if !item.isSeries && item.title.Kind == kindEpisode {
		return fmt.Errorf("title %d: %w", itemID, ErrNotChoosable)
	}
	provider, known := chooser.matcher.providers[providerName]
	if !known {
		return fmt.Errorf("%q: %w", providerName, ErrUnknownProvider)
	}
	if item.isSeries {
		err = chooser.matcher.acceptSeries(ctx, provider, item.series, providerID, identificationConfirmed)
	} else {
		err = chooser.matcher.accept(ctx, provider, item.title, providerID, identificationConfirmed)
	}
	if err != nil {
		return err
	}
	return chooser.dropCandidates(ctx, itemID)
}

// dropCandidates forgets an item's kept candidates: chosen from, or searched for under a name or
// year that is no longer the item's, so the next look searches again.
func (chooser *Chooser) dropCandidates(ctx context.Context, itemID int64) error {
	err := chooser.matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.DeleteMatchCandidates(ctx, itemID)
	})
	if err != nil {
		return fmt.Errorf("drop item %d's candidates: %w", itemID, err)
	}
	return nil
}

// isSearchField is whether a field is one a search for candidates is made with: changing it makes
// the kept candidates another search's (D223 §7).
func isSearchField(field string) bool {
	return field == fieldName || field == fieldYear
}

// SetOverride sets a field on a title or series to a person's value, which every source and
// pass keeps (D183), and re-derives its library.
func (chooser *Chooser) SetOverride(ctx context.Context, itemID int64, field, value string, setByUserID int64) error {
	// A person's text is cleaned where it enters (principles §22): it is shown to everyone.
	if field == fieldOverview {
		value = textindex.Prose(value)
	} else {
		value = textindex.Line(value)
	}
	item, err := chooser.readTarget(ctx, itemID)
	if err != nil {
		return err
	}
	if checkErr := checkOverride(item, field, value); checkErr != nil {
		return checkErr
	}
	err = chooser.matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertOverride(ctx, sqlcgen.UpsertOverrideParams{
			ItemID: itemID, Field: field, OverrideValue: value, SetByUserID: &setByUserID,
			SetAt: chooser.matcher.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("override item %d's %s: %w", itemID, field, err)
	}
	if isSearchField(field) {
		if err := chooser.dropCandidates(ctx, itemID); err != nil {
			return err
		}
	}
	return chooser.rederive(ctx, item)
}

// ClearOverride removes a field's override, so the field follows its source again (D183).
func (chooser *Chooser) ClearOverride(ctx context.Context, itemID int64, field string) error {
	item, err := chooser.readTarget(ctx, itemID)
	if err != nil {
		return err
	}
	if !slices.Contains(fieldsOf(item), field) {
		return fmt.Errorf("%q: %w", field, ErrUnknownField)
	}
	var removed int64
	err = chooser.matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var deleteErr error
		removed, deleteErr = queries.DeleteOverride(ctx, sqlcgen.DeleteOverrideParams{ItemID: itemID, Field: field})
		return deleteErr //nolint:wrapcheck // wrapped below, outside the transaction
	})
	if err != nil {
		return fmt.Errorf("remove item %d's %s override: %w", itemID, field, err)
	}
	if removed == 0 {
		return fmt.Errorf("item %d's %s: %w", itemID, field, ErrNoOverride)
	}
	if isSearchField(field) {
		if err := chooser.dropCandidates(ctx, itemID); err != nil {
			return err
		}
	}
	return chooser.rederive(ctx, item)
}

func fieldsOf(item target) []string {
	if item.isSeries {
		return seriesOverridableFields()
	}
	return OverridableFields()
}

func checkOverride(item target, field, value string) error {
	if !slices.Contains(fieldsOf(item), field) {
		return fmt.Errorf("%q: %w", field, ErrUnknownField)
	}
	switch field {
	case fieldYear, fieldRuntimeMs:
		if number, err := strconv.ParseInt(value, 10, 64); err != nil || number <= 0 {
			return fmt.Errorf("%s %q: %w; it takes a whole number above zero", field, value, ErrBadValue)
		}
	case fieldName:
		if value == "" {
			return fmt.Errorf("%s: %w; a name cannot be empty", field, ErrBadValue)
		}
	case fieldOverview, fieldTagline:
	default:
		return fmt.Errorf("%q: %w", field, ErrUnknownField)
	}
	return nil
}

// rederive runs the pass over the item's library, which is how its values are resolved.
func (chooser *Chooser) rederive(ctx context.Context, item target) error {
	libraryID := item.libraryID()
	if libraryID == nil {
		return nil
	}
	if _, err := chooser.pass.IdentifyLibrary(ctx, *libraryID); err != nil {
		return err
	}
	return nil
}

// readTarget reads the title or series an id names.
func (chooser *Chooser) readTarget(ctx context.Context, itemID int64) (target, error) {
	queries := chooser.matcher.database.Read()
	kind, err := store.OrNotFound(queries.GetItemKind(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return target{}, fmt.Errorf("item %d: %w", itemID, ErrNoItem)
	}
	if err != nil {
		return target{}, fmt.Errorf("read item %d: %w", itemID, err)
	}
	var item target
	if kind == itemKindCollection {
		item.isSeries = true
		item.series, err = store.OrNotFound(queries.GetSeriesForMatch(ctx, itemID))
	} else {
		item.title, err = store.OrNotFound(queries.GetTitleForMatch(ctx, itemID))
	}
	if errors.Is(err, store.ErrNotFound) {
		return target{}, fmt.Errorf("item %d: %w", itemID, ErrNoItem)
	}
	if err != nil {
		return target{}, fmt.Errorf("read item %d: %w", itemID, err)
	}
	return item, nil
}

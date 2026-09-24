// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Refusals a person's request can meet.
var (
	// ErrNoTitle is a title that does not exist or was removed.
	ErrNoTitle = errors.New("no such title")
	// ErrUnknownProvider is a provider this server has no client for.
	ErrUnknownProvider = errors.New("no such provider")
	// ErrUnknownField is a field that cannot be overridden.
	ErrUnknownField = errors.New("no such field")
	// ErrBadValue is an override value the field cannot hold.
	ErrBadValue = errors.New("the value does not fit the field")
	// ErrNoOverride is a field with no override to remove.
	ErrNoOverride = errors.New("the field has no override")
)

// Idents are the states of identification a title list can ask for.
func Idents() []string {
	return []string{identIdentified, identAmbiguous, identUnidentified, identConfirmed}
}

// Fields are the fields a person can override on a title (D183).
func Fields() []string {
	return []string{fieldName, fieldOverview, fieldTagline, fieldYear, fieldRuntime}
}

// Chooser is what a person does about identification: see the titles in a state, see an
// ambiguous title's candidates, choose its match, and override a field (D172, D183).
type Chooser struct {
	matcher *Matcher
	pass    *Pass
}

// NewChooser returns the chooser.
func NewChooser(matcher *Matcher, pass *Pass) *Chooser {
	return &Chooser{matcher: matcher, pass: pass}
}

// TitleSummary is one title in a list.
type TitleSummary struct {
	ID        int64
	LibraryID int64
	Kind      string
	Name      string
	Year      int64
	Ident     string
	Source    string
	SourceID  string
}

// Titles lists the titles in one state of identification.
func (chooser *Chooser) Titles(ctx context.Context, ident string) ([]TitleSummary, error) {
	rows, err := chooser.matcher.database.Read().ListTitlesByIdent(ctx, ident)
	if err != nil {
		return nil, fmt.Errorf("list %s titles: %w", ident, err)
	}
	titles := make([]TitleSummary, 0, len(rows))
	for _, row := range rows {
		titles = append(titles, TitleSummary{
			ID: row.ID, LibraryID: deref(row.LibraryID), Kind: row.Kind, Name: derefText(row.Name),
			Year: deref(row.Year), Ident: row.Ident, Source: derefText(row.Source), SourceID: derefText(row.SourceID),
		})
	}
	return titles, nil
}

// Candidates are a title's kept search results, best first. When cache.db lost them, an
// ambiguous title is searched once more (schema.md, cache.db).
func (chooser *Chooser) Candidates(ctx context.Context, titleID int64) ([]Candidate, string, error) {
	title, err := chooser.title(ctx, titleID)
	if err != nil {
		return nil, "", err
	}
	kept, err := chooser.matcher.cache.Read().ListMatchCandidates(ctx, titleID)
	if err != nil {
		return nil, "", fmt.Errorf("read title %d's candidates: %w", titleID, err)
	}
	provider := ""
	candidates := make([]Candidate, 0, len(kept))
	for _, row := range kept {
		provider = row.Provider
		candidates = append(candidates, Candidate{
			ID: row.ProviderID, Name: derefText(row.Name), Year: int(deref(row.Year)),
			PosterRef: derefText(row.PosterRef),
		})
	}
	if len(candidates) > 0 || title.Ident != identAmbiguous {
		return candidates, provider, nil
	}
	return chooser.searchAgain(ctx, title)
}

func (chooser *Chooser) searchAgain(
	ctx context.Context, title sqlcgen.GetTitleForMatchRow,
) ([]Candidate, string, error) {
	provider, known := chooser.matcher.providers[providerOf(title.MetadataSource)]
	if !known {
		return []Candidate{}, "", nil // an NFO library's title has no provider to ask
	}
	query := Query{Kind: title.Kind, Name: derefText(title.Name), Year: int(deref(title.Year))}
	candidates, err := provider.Search(ctx, query)
	if err != nil {
		return nil, "", fmt.Errorf("search %s for %q: %w", provider.Name(), query.Name, err)
	}
	if err := chooser.matcher.keep(ctx, provider, title.ID, candidates); err != nil {
		return nil, "", err
	}
	return candidates, provider.Name(), nil
}

// Choose matches a title to a work a person chose: fetched, stored and permanent, over any NFO
// and every later pass (D172). The candidates are no longer needed.
func (chooser *Chooser) Choose(ctx context.Context, titleID int64, providerName, providerID string) error {
	title, err := chooser.title(ctx, titleID)
	if err != nil {
		return err
	}
	provider, known := chooser.matcher.providers[providerName]
	if !known {
		return fmt.Errorf("%q: %w", providerName, ErrUnknownProvider)
	}
	err = chooser.matcher.accept(ctx, provider, title, providerID, identConfirmed)
	if err != nil {
		return err
	}
	err = chooser.matcher.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.DeleteMatchCandidates(ctx, titleID)
	})
	if err != nil {
		return fmt.Errorf("drop title %d's candidates: %w", titleID, err)
	}
	return nil
}

// SetOverride sets a field on a title to a person's value, which every source and pass keeps
// (D183), and re-derives the title's library.
func (chooser *Chooser) SetOverride(ctx context.Context, titleID int64, field, value string, setBy int64) error {
	if err := checkOverride(field, value); err != nil {
		return err
	}
	title, err := chooser.title(ctx, titleID)
	if err != nil {
		return err
	}
	err = chooser.matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertOverride(ctx, sqlcgen.UpsertOverrideParams{
			ItemID: titleID, Field: field, Value: value, SetBy: &setBy, SetAt: chooser.matcher.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("override title %d's %s: %w", titleID, field, err)
	}
	return chooser.rederive(ctx, title)
}

// ClearOverride removes a field's override, so the field follows its source again (D183).
func (chooser *Chooser) ClearOverride(ctx context.Context, titleID int64, field string) error {
	if !slices.Contains(Fields(), field) {
		return fmt.Errorf("%q: %w", field, ErrUnknownField)
	}
	title, err := chooser.title(ctx, titleID)
	if err != nil {
		return err
	}
	var removed int64
	err = chooser.matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var deleteErr error
		removed, deleteErr = queries.DeleteOverride(ctx, sqlcgen.DeleteOverrideParams{ItemID: titleID, Field: field})
		return deleteErr //nolint:wrapcheck // wrapped below, outside the transaction
	})
	if err != nil {
		return fmt.Errorf("remove title %d's %s override: %w", titleID, field, err)
	}
	if removed == 0 {
		return fmt.Errorf("title %d's %s: %w", titleID, field, ErrNoOverride)
	}
	return chooser.rederive(ctx, title)
}

func checkOverride(field, value string) error {
	switch field {
	case fieldYear, fieldRuntime:
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

// rederive runs the pass over the title's library, which is how a title's values are resolved.
func (chooser *Chooser) rederive(ctx context.Context, title sqlcgen.GetTitleForMatchRow) error {
	if title.LibraryID == nil {
		return nil
	}
	if _, err := chooser.pass.Library(ctx, *title.LibraryID); err != nil {
		return err
	}
	return nil
}

func (chooser *Chooser) title(ctx context.Context, titleID int64) (sqlcgen.GetTitleForMatchRow, error) {
	title, err := store.One(chooser.matcher.database.Read().GetTitleForMatch(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return title, fmt.Errorf("title %d: %w", titleID, ErrNoTitle)
	}
	if err != nil {
		return title, fmt.Errorf("read title %d: %w", titleID, err)
	}
	return title, nil
}

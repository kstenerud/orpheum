// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// titleSpec is one file's evidence about its title: the local data, or its NFO as the source.
type titleSpec struct {
	kind      string // movie, episode, extra
	key       string // set for episodes and extras, whose name does not identify them
	name      string
	year      int
	runtimeMs int64
	overview  string
	tagline   string
	ids       map[string]string
	locked    []string
	source    string // "nfo", "none", a provider's name, or "" while a provider match is pending
	sourceID  string // the title's id at its source, when its ids do not say (an episode's)
	ident     string // identified or unidentified
	reason    string // why a file is unidentified
}

// fromNFO takes an NFO as the title's source: its values over the path's, and its locked fields
// as a person's corrections (D182, D183).
func (spec *titleSpec) fromNFO(nfo NFO) {
	if nfo.Title != "" {
		spec.name = nfo.Title
	}
	if nfo.Year > 0 {
		spec.year = nfo.Year
	}
	if nfo.RuntimeMs > 0 {
		spec.runtimeMs = nfo.RuntimeMs
	}
	spec.overview, spec.tagline = nfo.Plot, nfo.Tagline
	merged := maps.Clone(spec.ids)
	maps.Copy(merged, nfo.ProviderIDs)
	spec.ids, spec.locked = merged, nfo.Locked
	spec.source, spec.ident = sourceNFO, identIdentified
}

func movieKey(name string, year int) string {
	return kindMovie + "|" + strings.ToLower(name) + "|" + strconv.Itoa(year)
}

func episodeKey(seriesID int64, season, episode int) string {
	return fmt.Sprintf("episode|%d|%d|%d", seriesID, season, episode)
}

// titleFor finds the title a spec names, or makes it. A provider id finds a film first: in this
// library, or in another where it has no live files left, when the title follows its files here
// (D208 §2). Otherwise its key does: an episode's series and place, a film's name and year.
func (work *run) titleFor(ctx context.Context, spec titleSpec) (int64, error) {
	for _, provider := range []string{sourceTMDB, sourceIMDb, sourceTVDB} {
		id := spec.ids[provider]
		if id == "" || spec.kind == kindEpisode {
			continue // an episode is its series' (D209)
		}
		found, err := work.titleByProviderID(ctx, spec.kind, provider, id)
		if err != nil || found != 0 {
			return found, err
		}
	}
	key := spec.key
	if key == "" {
		key = movieKey(spec.name, spec.year)
	}
	if id, found := work.titles[key]; found {
		return id, nil
	}
	libraryID := work.library.ID
	id, err := work.queries.InsertItem(ctx, sqlcgen.InsertItemParams{
		Kind: "title", LibraryID: &libraryID, CreatedAt: work.now,
	})
	if err != nil {
		return 0, fmt.Errorf("add a title for %q: %w", spec.name, err)
	}
	err = work.queries.InsertTitle(ctx, sqlcgen.InsertTitleParams{ID: id, Kind: spec.kind, Ident: identUnidentified})
	if err != nil {
		return 0, fmt.Errorf("add title %q: %w", spec.name, err)
	}
	work.titles[key] = id
	return id, nil
}

// titleByProviderID finds a title of a kind by a provider's id: a provider numbers its films and
// its episodes separately, so an id alone may name both.
func (work *run) titleByProviderID(ctx context.Context, kind, provider, providerID string) (int64, error) {
	rows, err := work.queries.ListTitlesByProviderID(ctx, sqlcgen.ListTitlesByProviderIDParams{
		Provider: provider, ProviderID: providerID,
	})
	if err != nil {
		return 0, fmt.Errorf("look up %s %s: %w", provider, providerID, err)
	}
	rows = slices.DeleteFunc(rows, func(row sqlcgen.ListTitlesByProviderIDRow) bool { return row.Kind != kind })
	for _, row := range rows {
		if row.LibraryID != nil && *row.LibraryID == work.library.ID {
			return row.ID, nil
		}
	}
	for _, row := range rows {
		live, countErr := work.queries.CountLiveFilesOfTitle(ctx, row.ID)
		if countErr != nil {
			return 0, fmt.Errorf("count the files of title %d: %w", row.ID, countErr)
		}
		if live > 0 {
			continue // its files are still there: that library keeps its title
		}
		libraryID := work.library.ID
		err = work.queries.MoveItemToLibrary(ctx, sqlcgen.MoveItemToLibraryParams{LibraryID: &libraryID, ID: row.ID})
		if err != nil {
			return 0, fmt.Errorf("move title %d to library %d: %w", row.ID, libraryID, err)
		}
		return row.ID, nil
	}
	return 0, nil
}

// resolveAndFile writes each title's resolved values — override, else the best evidence of any
// of its files, NFO before local data (D183) — and files each video by its title.
func (work *run) resolveAndFile(ctx context.Context) error {
	idents, reasons := map[int64]string{}, map[int64]string{}
	for titleID, specs := range work.evidence {
		result, err := work.resolveWithMatch(ctx, titleID, best(specs))
		if err != nil {
			return err
		}
		idents[titleID], reasons[titleID] = result.ident, result.reason
	}
	for _, file := range work.files {
		titleID, placed := work.fileTitle[file.ID]
		if !placed {
			continue
		}
		filing, reason := filingListed, ""
		if ident := idents[titleID]; ident == identUnidentified {
			filing, reason = filingUnidentified, reasons[titleID]
			work.report.Unidentified++
		} else {
			work.report.Listed++
		}
		err := work.queries.SetFileFiling(ctx, sqlcgen.SetFileFilingParams{
			Filing: filing, FilingReason: optional(reason), ID: file.ID,
		})
		if err != nil {
			return fmt.Errorf("file %s: %w", file.Path, err)
		}
	}
	return nil
}

// best is the strongest evidence: an NFO's over the path's.
func best(specs []titleSpec) titleSpec {
	for _, spec := range specs {
		if spec.source == sourceNFO {
			return spec
		}
	}
	return specs[0]
}

// resolveWithMatch resolves a title from its best evidence, or from what its stored match says
// when no NFO speaks for it or a person chose the match.
func (work *run) resolveWithMatch(ctx context.Context, titleID int64, spec titleSpec) (resolved, error) {
	if err := work.matchEvidence(ctx, titleID, &spec); err != nil {
		return resolved{}, err
	}
	return work.resolve(ctx, titleID, spec)
}

// resolved is a title's identification after resolving, and why when it is unidentified.
type resolved struct {
	ident  string
	reason string
}

// resolve writes a title's provider ids, its NFO's locked fields as overrides, and its resolved
// values, and returns its ident and, when it is unidentified, why.
func (work *run) resolve(ctx context.Context, titleID int64, spec titleSpec) (resolved, error) {
	for provider, id := range spec.ids {
		err := work.queries.InsertItemProviderID(ctx, sqlcgen.InsertItemProviderIDParams{
			ItemID: titleID, Provider: provider, ProviderID: id,
		})
		if err != nil {
			return resolved{}, fmt.Errorf("record %s %s: %w", provider, id, err)
		}
	}
	values := map[string]string{
		fieldName: spec.name, fieldOverview: spec.overview, fieldTagline: spec.tagline,
		fieldYear: strconv.Itoa(spec.year), fieldRuntime: strconv.FormatInt(spec.runtimeMs, 10),
	}
	for _, field := range spec.locked {
		err := work.queries.InsertOverrideIfAbsent(ctx, sqlcgen.InsertOverrideIfAbsentParams{
			ItemID: titleID, Field: field, Value: values[field], Origin: "nfo", SetAt: work.now,
		})
		if err != nil {
			return resolved{}, fmt.Errorf("record the locked %s: %w", field, err)
		}
	}
	overrides, err := work.queries.ListOverrides(ctx, titleID)
	if err != nil {
		return resolved{}, fmt.Errorf("read the overrides of title %d: %w", titleID, err)
	}
	for _, override := range overrides {
		values[override.Field] = override.Value
	}
	year, _ := strconv.ParseInt(values[fieldYear], 10, 64)       //nolint:errcheck // zero when unset
	runtime, _ := strconv.ParseInt(values[fieldRuntime], 10, 64) //nolint:errcheck // zero when unset
	var source, sourceID *string
	if spec.source != "" {
		source = &spec.source
		id := spec.ids[spec.source]
		if spec.source == sourceNFO {
			id = spec.ids["tmdb"]
		}
		if spec.sourceID != "" {
			id = spec.sourceID
		}
		if id != "" {
			sourceID = &id
		}
	}
	err = work.queries.UpdateTitleResolved(ctx, sqlcgen.UpdateTitleResolvedParams{
		Year: optionalInt(year), RuntimeMs: optionalInt(runtime), Source: source, SourceID: sourceID,
		Ident: spec.ident, ID: titleID,
	})
	if err != nil {
		return resolved{}, fmt.Errorf("resolve title %d: %w", titleID, err)
	}
	err = work.queries.UpsertOriginalText(ctx, sqlcgen.UpsertOriginalTextParams{
		ItemID: titleID, Lang: langUnknown, Name: values[fieldName], Overview: optional(values[fieldOverview]),
		Tagline: optional(values[fieldTagline]),
	})
	if err != nil {
		return resolved{}, fmt.Errorf("name title %d: %w", titleID, err)
	}
	current, err := store.One(work.queries.GetTitleIdent(ctx, titleID))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return resolved{}, fmt.Errorf("read title %d's identification: %w", titleID, err)
	}
	return resolved{ident: current, reason: spec.reason}, nil
}

func optional(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func optionalInt(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

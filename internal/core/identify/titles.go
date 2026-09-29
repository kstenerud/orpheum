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

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// titleSpec is one file's evidence about its title: the local data, or its NFO as the source.
type titleSpec struct {
	kind           string // movie, episode, extra
	key            string // set for episodes and extras, whose name does not identify them
	name           string
	year           int
	runtimeMs      int64
	overview       string
	tagline        string
	ids            map[string]string
	lockedFields   []string
	artwork        []ArtRef // the images its source names
	source         string   // "nfo", "none", a provider's name, or "" while a provider match is pending
	sourceID       string   // the title's id at its source, when its ids do not say (an episode's)
	identification string   // identified or unidentified
	reason         string   // why a file is unidentified
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
	spec.ids, spec.lockedFields = merged, nfo.LockedFields
	spec.source, spec.identification = sourceNFO, identificationIdentified
}

func movieKey(name string, year int) string {
	return KindMovie + "|" + strings.ToLower(name) + "|" + strconv.Itoa(year)
}

func episodeKey(seriesID int64, season, episode int) string {
	return fmt.Sprintf("episode|%d|%d|%d", seriesID, season, episode)
}

// findOrAddTitle finds the title a spec names, or makes it. A provider id finds a film first: in this
// library, or in another where it has no live files left, when the title follows its files here
// (D208 §2). Otherwise its key does: an episode's series and place, a film's name and year.
func (filer *libraryFiler) findOrAddTitle(ctx context.Context, spec titleSpec) (int64, error) {
	for _, provider := range []string{sourceTMDB, sourceIMDb, sourceTVDB} {
		id := spec.ids[provider]
		if id == "" || spec.kind == library.TitleKindEpisode {
			continue // an episode is its series' (D209)
		}
		found, err := filer.claimTitleByProviderID(ctx, spec.kind, provider, id)
		if err != nil || found != 0 {
			return found, err
		}
	}
	key := spec.key
	if key == "" {
		key = movieKey(spec.name, spec.year)
	}
	if id, found := filer.titleIDByKey[key]; found {
		return id, nil
	}
	libraryID := filer.library.ID
	id, err := filer.queries.InsertItem(ctx, sqlcgen.InsertItemParams{
		Kind: "title", LibraryID: &libraryID, AddedAt: filer.nowMs,
	})
	if err != nil {
		return 0, fmt.Errorf("add a title for %q: %w", spec.name, err)
	}
	err = filer.queries.InsertTitle(ctx, sqlcgen.InsertTitleParams{
		ID: id, Kind: spec.kind,
		IdentificationState: identificationUnidentified,
	})
	if err != nil {
		return 0, fmt.Errorf("add title %q: %w", spec.name, err)
	}
	filer.titleIDByKey[key] = id
	return id, nil
}

// claimTitleByProviderID finds a title of a kind by a provider's id: a provider numbers its films and
// its episodes separately, so an id alone may name both.
func (filer *libraryFiler) claimTitleByProviderID(
	ctx context.Context, kind, provider, providerID string) (int64, error,
) {
	rows, err := filer.queries.ListTitlesByProviderID(ctx, sqlcgen.ListTitlesByProviderIDParams{
		Provider: provider, ProviderID: providerID,
	})
	if err != nil {
		return 0, fmt.Errorf("look up %s %s: %w", provider, providerID, err)
	}
	rows = slices.DeleteFunc(rows, func(row sqlcgen.ListTitlesByProviderIDRow) bool { return row.Kind != kind })
	for _, row := range rows {
		if row.LibraryID != nil && *row.LibraryID == filer.library.ID {
			return row.ID, nil
		}
	}
	for _, row := range rows {
		liveFileCount, countErr := filer.queries.CountLiveFilesOfTitle(ctx, row.ID)
		if countErr != nil {
			return 0, fmt.Errorf("count the files of title %d: %w", row.ID, countErr)
		}
		if liveFileCount > 0 {
			continue // its files are still there: that library keeps its title
		}
		libraryID := filer.library.ID
		err = filer.queries.MoveItemToLibrary(ctx, sqlcgen.MoveItemToLibraryParams{LibraryID: &libraryID, ID: row.ID})
		if err != nil {
			return 0, fmt.Errorf("move title %d to library %d: %w", row.ID, libraryID, err)
		}
		return row.ID, nil
	}
	return 0, nil
}

// resolveAndFile writes each title's resolved values — override, else the best evidence of any
// of its files, NFO before local data (D183) — and files each video by its title.
func (filer *libraryFiler) resolveAndFile(ctx context.Context) error {
	identificationByTitle, reasonByTitle := map[int64]string{}, map[int64]string{}
	for titleID, specs := range filer.evidence {
		resolution, err := filer.resolveWithMatch(ctx, titleID, bestEvidence(specs))
		if err != nil {
			return err
		}
		identificationByTitle[titleID], reasonByTitle[titleID] = resolution.identification, resolution.reason
	}
	for _, file := range filer.files {
		titleID, placed := filer.titleIDByFile[file.ID]
		if !placed {
			continue
		}
		filing, reason := library.FilingListed, ""
		if why, isExtra := filer.extraReasonByFile[file.ID]; isExtra {
			// An extra is filed as one, with its owner, its type and the evidence (D163).
			filing, reason = library.FilingExtra, why
			filer.report.ExtraCount++
		} else if identification := identificationByTitle[titleID]; identification == identificationUnidentified {
			filing, reason = library.FilingUnidentified, reasonByTitle[titleID]
			filer.report.UnidentifiedCount++
		} else {
			filer.report.ListedCount++
		}
		err := filer.queries.SetFileFiling(ctx, sqlcgen.SetFileFilingParams{
			Filing: filing, FilingReason: ptr.NilIfZero(reason), ID: file.ID,
		})
		if err != nil {
			return fmt.Errorf("file %s: %w", file.Path, err)
		}
	}
	return nil
}

// bestEvidence is the strongest evidence: an NFO's over the path's.
func bestEvidence(specs []titleSpec) titleSpec {
	for _, spec := range specs {
		if spec.source == sourceNFO {
			return spec
		}
	}
	return specs[0]
}

// resolveWithMatch resolves a title from its best evidence, or from what its stored match says
// when no NFO speaks for it or a person chose the match.
func (filer *libraryFiler) resolveWithMatch(
	ctx context.Context, titleID int64, spec titleSpec) (titleResolution, error,
) {
	if err := filer.matchEvidence(ctx, titleID, &spec); err != nil {
		return titleResolution{}, err
	}
	return filer.resolve(ctx, titleID, spec)
}

// titleResolution is a title's identification after resolving, and why when it is unidentified.
type titleResolution struct {
	identification string
	reason         string
}

// resolve writes a title's provider ids, its NFO's locked fields as overrides, and its resolved
// values, and returns its identification and, when it is unidentified, why.
func (filer *libraryFiler) resolve(ctx context.Context, titleID int64, spec titleSpec) (titleResolution, error) {
	if err := filer.recordIDs(ctx, titleID, spec.ids); err != nil {
		return titleResolution{}, err
	}
	values := map[string]string{
		fieldName: spec.name, fieldOverview: spec.overview, fieldTagline: spec.tagline,
		fieldYear: strconv.Itoa(spec.year), fieldRuntimeMs: strconv.FormatInt(spec.runtimeMs, 10),
	}
	for _, field := range spec.lockedFields {
		err := filer.queries.InsertOverrideIfAbsent(ctx, sqlcgen.InsertOverrideIfAbsentParams{
			ItemID: titleID, Field: field, OverrideValue: values[field], Origin: "nfo", SetAt: filer.nowMs,
		})
		if err != nil {
			return titleResolution{}, fmt.Errorf("record the locked %s: %w", field, err)
		}
	}
	overrides, err := filer.queries.ListOverrides(ctx, titleID)
	if err != nil {
		return titleResolution{}, fmt.Errorf("read the overrides of title %d: %w", titleID, err)
	}
	for _, override := range overrides {
		values[override.Field] = override.OverrideValue
	}
	year, _ := strconv.ParseInt(values[fieldYear], 10, 64)           //nolint:errcheck // zero when unset
	runtimeMs, _ := strconv.ParseInt(values[fieldRuntimeMs], 10, 64) //nolint:errcheck // zero when unset
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
	err = filer.queries.UpdateTitleResolved(ctx, sqlcgen.UpdateTitleResolvedParams{
		Year: ptr.NilIfZero(year), RuntimeMs: ptr.NilIfZero(runtimeMs),
		FollowedSource: source, FollowedSourceID: sourceID,
		IdentificationState: spec.identification, ID: titleID,
	})
	if err != nil {
		return titleResolution{}, fmt.Errorf("resolve title %d: %w", titleID, err)
	}
	err = filer.writeText(ctx, titleID, itemText{
		sortListing: spec.kind, name: values[fieldName], overview: values[fieldOverview], tagline: values[fieldTagline],
	})
	if err != nil {
		return titleResolution{}, fmt.Errorf("name title %d: %w", titleID, err)
	}
	if owedErr := filer.oweArtwork(ctx, titleID, spec.source, spec.artwork); owedErr != nil {
		return titleResolution{}, owedErr
	}
	identification, err := store.OrNotFound(filer.queries.GetTitleIdentification(ctx, titleID))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return titleResolution{}, fmt.Errorf("read title %d's identification: %w", titleID, err)
	}
	return titleResolution{identification: identification, reason: spec.reason}, nil
}

// recordIDs keeps the provider ids a title's evidence names.
func (filer *libraryFiler) recordIDs(ctx context.Context, titleID int64, ids map[string]string) error {
	for provider, id := range ids {
		err := filer.queries.InsertItemProviderID(ctx, sqlcgen.InsertItemProviderIDParams{
			ItemID: titleID, Provider: provider, ProviderID: id,
		})
		if err != nil {
			return fmt.Errorf("record %s %s: %w", provider, id, err)
		}
	}
	return nil
}

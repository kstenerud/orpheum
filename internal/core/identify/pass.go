// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Pass turns a library's stored observations into titles (identification.md). It reads only
// what the database holds — paths, probes, NFO bytes — so it opens no file, and it can run again
// at any time and reach the same answer (D182).
type Pass struct {
	database       *store.Database
	clock          clock.Clock
	logger         *slog.Logger
	providers      Providers
	settings       Settings
	afterPassHooks []func(context.Context, Report)
}

// AfterPass calls follow after each pass and each re-derivation a match makes, with what it left
// owing, once it is stored. Set them before the pass runs.
func (pass *Pass) AfterPass(follow func(context.Context, Report)) {
	pass.afterPassHooks = append(pass.afterPassHooks, follow)
}

// runAfterPassHooks hands what a pass or a match left owing to the queues that follow it.
func (pass *Pass) runAfterPassHooks(ctx context.Context, report Report) {
	for _, follow := range pass.afterPassHooks {
		follow(ctx, report)
	}
}

// Settings are what the pass reads from the settings: the locale it sorts in.
type Settings interface {
	String(ctx context.Context, key settings.Key) (string, error)
}

// Providers are the metadata providers, by name. The pass only parses their stored responses.
type Providers map[string]Provider

// Report is what one pass filed.
type Report struct {
	ListedCount       int
	UnidentifiedCount int
	ExtraCount        int
	// AwaitingMatch are the films and series no source has answered for yet: the fetch queue's
	// first fetches (D170, D209).
	AwaitingMatch []int64
	// AwaitingListings are matched series with a season listing to fetch: missing, or older
	// than a new episode it does not list (D209 §3).
	AwaitingListings []int64
	// NamedImages are the images each item's provider names, every pass and every match; the
	// artwork service fetches what it does not hold (D212).
	NamedImages []NamedImages
	// Rematched are items a match moved to another work at a source they followed, with the new
	// work's images: the earlier work's images go (D223 §9).
	Rematched []NamedImages
	// BesideTheMedia are the images a pass found beside its library's media; an image not found
	// again goes (D215 §1). Nil from a match.
	BesideTheMedia *LocalImages
}

// NewPass returns the identification pass.
func NewPass(
	database *store.Database, clockSource clock.Clock, logger *slog.Logger, providers Providers, sortSettings Settings,
) *Pass {
	return &Pass{
		database: database, clock: clockSource, logger: logger, providers: cleanProviders(providers),
		settings: sortSettings,
	}
}

// sorter is the sort-key maker for the locale the settings name now (D47).
func (pass *Pass) sorter(ctx context.Context) (*textindex.Sorter, error) {
	locale, err := pass.settings.String(ctx, settings.SortLocale)
	if err != nil {
		return nil, fmt.Errorf("read sort_locale: %w", err)
	}
	sorter, err := textindex.NewSorter(locale)
	if err != nil {
		return nil, fmt.Errorf("sort in %q: %w", locale, err)
	}
	return sorter, nil
}

// IdentifyAll identifies every live library again, as a scan's pass does: the sweep that derives,
// from persistent.db alone, the season listings and the images each item's sources name, which the
// pass's report hands on (D231 §3). A library that fails is logged, and the others still run; it
// says how many passes ran.
func (pass *Pass) IdentifyAll(ctx context.Context) (int, error) {
	libraryIDs, err := pass.database.Read().ListLiveLibraryIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the libraries: %w", err)
	}
	passed := 0
	for _, libraryID := range libraryIDs {
		if _, err := pass.IdentifyLibrary(ctx, libraryID); err != nil {
			if ctx.Err() != nil {
				return passed, fmt.Errorf("identify library %d: %w", libraryID, ctx.Err())
			}
			pass.logger.ErrorContext(ctx, "a library's pass failed", "libraryId", libraryID, "err", err)
			continue
		}
		passed++
	}
	return passed, nil
}

// IdentifyLibrary identifies every video in a library, in one transaction.
func (pass *Pass) IdentifyLibrary(ctx context.Context, libraryID int64) (Report, error) {
	var report Report
	sorter, err := pass.sorter(ctx)
	if err != nil {
		return Report{}, err
	}
	err = pass.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		filer := &libraryFiler{
			queries: queries, nowMs: pass.clock.Now().UnixMilli(), report: &report, providers: pass.providers,
			sorter:       sorter,
			titleIDByKey: map[string]int64{}, seriesIDByName: map[string]int64{}, nfoByVideo: map[int64]NFO{},
			showNFOs: map[string]NFO{}, nfosByFolder: map[string][]namedNFO{}, titleIDByFolder: map[string]int64{},
			evidence: map[int64][]titleSpec{}, titleIDByFile: map[int64]int64{}, extraReasonByFile: map[int64]string{},
			localArt: map[int64][]LocalImage{}, seriesIDByFolder: map[string]int64{},
			seriesSpecByID: map[int64]seriesSpec{}, seriesMatchByID: map[int64]sqlcgen.GetSeriesForMatchRow{},
			listings: map[string][]Episode{}, returningEpisodes: map[int64]bool{},
		}
		if loadErr := filer.load(ctx, libraryID); loadErr != nil {
			return loadErr
		}
		if identifyErr := filer.identify(ctx); identifyErr != nil {
			return identifyErr
		}
		return filer.listAwaitingMatch(ctx, libraryID)
	})
	if err != nil {
		return Report{}, fmt.Errorf("identify library %d: %w", libraryID, err)
	}
	// The locale may have changed after this pass read it: any name it keyed in the old one is
	// keyed again in the new.
	if _, err := pass.RefreshSortAndSearchKeys(ctx); err != nil {
		return Report{}, err
	}
	pass.runAfterPassHooks(ctx, report)
	return report, nil
}

// libraryFiler is one pass over one library.
type libraryFiler struct {
	queries   *sqlcgen.Queries
	nowMs     int64
	report    *Report
	providers Providers
	sorter    *textindex.Sorter
	library   sqlcgen.GetLibraryForIdentifyRow
	roots     []sqlcgen.ListLibraryRootsRow
	files     []sqlcgen.ListIdentifyFilesRow

	nfoByVideo        map[int64]NFO // an NFO attached to its video
	nfosByFolder      map[string][]namedNFO
	showNFOs          map[string]NFO   // a tvshow.nfo, by its folder
	titleIDByKey      map[string]int64 // by key
	seriesIDByName    map[string]int64 // by name, as textindex.SameWork folds it
	titleIDByFolder   map[string]int64 // a film's folder, for its extras
	evidence          map[int64][]titleSpec
	titleIDByFile     map[int64]int64
	extraReasonByFile map[int64]string       // an extra's file, and why it is one (D163)
	localArt          map[int64][]LocalImage // the images beside each item (D215)
	seriesIDByFolder  map[string]int64       // the folder a series' images sit in
	seriesSpecByID    map[int64]seriesSpec   // what this pass's files say about each series
	seriesMatchByID   map[int64]sqlcgen.GetSeriesForMatchRow
	listings          map[string][]Episode // parsed season listings, by "series/season"
	// returningEpisodes are the episodes of a series brought back into this library, each brought
	// back with it when a file of it is filed (D208 section 2).
	returningEpisodes map[int64]bool
}

// listAwaitingMatch lists what the fetch queue owes the library: films and series never answered for.
func (filer *libraryFiler) listAwaitingMatch(ctx context.Context, libraryID int64) error {
	if filer.library.MetadataSource == sourceNFO {
		return nil
	}
	films, err := filer.queries.ListTitlesAwaitingMatch(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list the films awaiting a match: %w", err)
	}
	series, err := filer.queries.ListSeriesAwaitingMatch(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list the series awaiting a match: %w", err)
	}
	filer.report.AwaitingMatch = slices.Concat(films, series)
	return nil
}

// identify files every video: films and episodes first, so an extra can find its film.
func (filer *libraryFiler) identify(ctx context.Context) error {
	if err := filer.knowSeriesByFolder(ctx); err != nil {
		return err
	}
	var extras []pendingExtra
	for _, file := range filer.files {
		if !isVideo(file) {
			continue
		}
		relativePath, ok := filer.relativePath(file)
		if !ok {
			continue
		}
		if extra, isExtra := detectExtra(relativePath); isExtra {
			extras = append(extras, pendingExtra{file: file, relativePath: relativePath, extra: extra})
			continue
		}
		if err := filer.fileFilmOrEpisode(ctx, file, relativePath); err != nil {
			return err
		}
	}
	for _, pending := range extras {
		if err := filer.fileExtra(ctx, pending); err != nil {
			return err
		}
	}
	filer.mapLocalArtwork()
	if err := filer.resolveAndFile(ctx); err != nil {
		return err
	}
	for _, seriesID := range slices.Sorted(maps.Keys(filer.seriesSpecByID)) {
		if err := filer.resolveSeries(ctx, seriesID, filer.seriesSpecByID[seriesID]); err != nil {
			return err
		}
	}
	filer.reportLocalImages()
	return nil
}

// knowSeriesByFolder lets the name an episode's folders give find the series its neighbours are
// already in, before any file is filed: a series renamed by its match (D209) keeps a new episode
// that sorts before its old ones.
func (filer *libraryFiler) knowSeriesByFolder(ctx context.Context) error {
	for _, file := range filer.files {
		relativePath, ok := filer.relativePath(file)
		if !isVideo(file) || !ok {
			continue
		}
		parsed := ParsePath(relativePath)
		if parsed.Kind != library.TitleKindEpisode {
			continue
		}
		key := textindex.SameWork(parsed.Name)
		if _, known := filer.seriesIDByName[key]; known {
			continue
		}
		seriesID, err := filer.currentSeries(ctx, file)
		if err != nil {
			return err
		}
		if seriesID != 0 {
			filer.seriesIDByName[key] = seriesID
		}
	}
	return nil
}

func isVideo(file sqlcgen.ListIdentifyFilesRow) bool {
	return file.PayloadHash != nil && file.Filing != library.FilingSkipped && file.Filing != library.FilingErrored &&
		file.Filing != library.FilingAttached
}

// relativePath is a file's path below the library root on its node that holds it, as text.
func (filer *libraryFiler) relativePath(file sqlcgen.ListIdentifyFilesRow) (string, bool) {
	var rootPaths []string
	for _, root := range filer.roots {
		if root.NodeID == file.NodeID {
			rootPaths = append(rootPaths, root.RootPath)
		}
	}
	_, relative, found := library.RootHolding(file.ReportedPathRaw, rootPaths)
	if !found {
		return "", false
	}
	return library.PathText(relative), true
}

// fileFilmOrEpisode files a film or an episode.
func (filer *libraryFiler) fileFilmOrEpisode(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, relativePath string,
) error {
	parsed := ParsePath(relativePath)
	if parsed.Kind == library.TitleKindEpisode {
		return filer.fileEpisode(ctx, file, parsed)
	}
	spec := filer.movieSpec(file, parsed)
	titleID, hasCurrentTitle, err := filer.currentTitle(ctx, file)
	if err == nil && !hasCurrentTitle {
		titleID, err = filer.findOrAddTitle(ctx, spec)
	}
	if err != nil {
		return err
	}
	filer.titleIDByFolder[path.Dir(file.Path)] = titleID
	return filer.place(ctx, file, titleID, spec)
}

// movieSpec is what a film's file and its NFO say about the work.
func (filer *libraryFiler) movieSpec(file sqlcgen.ListIdentifyFilesRow, parsed ParsedPath) titleSpec {
	spec := titleSpec{
		kind: KindMovie, name: parsed.Name, year: parsed.Year, ids: parsed.ProviderIDs,
		runtimeMs: probeRuntimeMs(file.ProbeJSON),
	}
	if nfo, found := filer.nfoFor(file); found && nfo.Kind == KindMovie {
		spec.fromNFO(nfo)
	}
	filer.unidentifiedUnlessNFO(&spec)
	return spec
}

// namedNFO is an NFO in a folder, by its file's stem.
type namedNFO struct {
	stem string
	nfo  NFO
}

// nfoFor is the NFO describing a video: the one attached to it; else one in its folder whose
// name begins the video's ("Film (2019).nfo" for "Film (2019) - 1080p.mkv", which describes
// every version in the folder); else the folder's movie.nfo.
func (filer *libraryFiler) nfoFor(file sqlcgen.ListIdentifyFilesRow) (NFO, bool) {
	if nfo, found := filer.nfoByVideo[file.ID]; found {
		return nfo, true
	}
	videoStem := strings.TrimSuffix(path.Base(file.Path), path.Ext(file.Path))
	chosen, score := NFO{}, -1
	for _, candidate := range filer.nfosByFolder[path.Dir(file.Path)] {
		switch {
		case strings.HasPrefix(videoStem, candidate.stem) && len(candidate.stem) > score:
			chosen, score = candidate.nfo, len(candidate.stem)
		case strings.EqualFold(candidate.stem, "movie") && score < 0:
			chosen, score = candidate.nfo, 0 // the fallback: any NFO naming the video wins over it
		}
	}
	return chosen, score >= 0
}

// unidentifiedUnlessNFO sets a spec with no NFO behind it: unidentified, and in a library that
// fetches from a provider, waiting for the match (D170).
func (filer *libraryFiler) unidentifiedUnlessNFO(spec *titleSpec) {
	if spec.source != "" {
		return // its NFO, or its series' listing, speaks for it
	}
	spec.identification = identificationUnidentified
	switch {
	case filer.library.MetadataSource == sourceNFO:
		spec.source, spec.reason = sourceNone, "no NFO file"
	case spec.reason == "":
		spec.reason = "not matched yet"
	}
}

// place gives a file its Version under the title, unless a person confirmed the file's current
// title: an automatic identification follows the path, a confirmed one never moves (D208 §1).
func (filer *libraryFiler) place(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, titleID int64, spec titleSpec,
) error {
	var err error
	if file.VersionID == nil {
		err = filer.newVersion(ctx, file, titleID)
	} else {
		titleID, err = filer.keepOrMove(ctx, file, titleID)
	}
	if err != nil {
		return err
	}
	filer.titleIDByFile[file.ID] = titleID
	filer.evidence[titleID] = append(filer.evidence[titleID], spec)
	return nil
}

// newVersion gives a file its own Version under the title.
func (filer *libraryFiler) newVersion(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, titleID int64) error {
	version, err := filer.queries.InsertVersion(ctx,
		sqlcgen.InsertVersionParams{TitleID: titleID, CreatedAt: filer.nowMs})
	if err != nil {
		return fmt.Errorf("add a version for %s: %w", file.Path, err)
	}
	err = library.PutInVersion(ctx, filer.queries, file.ID, version)
	if err != nil {
		return fmt.Errorf("put %s in its version: %w", file.Path, err)
	}
	return nil
}

// keepOrMove keeps a file's Version where it is when the file has not moved or a person
// confirmed its title, and otherwise moves it to titleID. It returns the title the file ends in.
func (filer *libraryFiler) keepOrMove(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, titleID int64) (int64, error,
) {
	current, err := filer.queries.GetVersionTitle(ctx, *file.VersionID)
	if err != nil {
		return 0, fmt.Errorf("read %s's version: %w", file.Path, err)
	}
	if current.IdentificationState == identificationConfirmed || !hasMoved(file) {
		titleID = current.TitleID
	} else if current.TitleID != titleID {
		move := sqlcgen.MoveVersionToTitleParams{TitleID: titleID, ID: *file.VersionID}
		err = filer.queries.MoveVersionToTitle(ctx, move)
		if err != nil {
			return 0, fmt.Errorf("move %s to its title: %w", file.Path, err)
		}
	}
	if hasMoved(file) {
		// Record the path this identification came from.
		err = library.PutInVersion(ctx, filer.queries, file.ID, *file.VersionID)
		if err != nil {
			return 0, fmt.Errorf("record %s's identification: %w", file.Path, err)
		}
	}
	return titleID, nil
}

// hasMoved reports whether a file's path differs from the one its identification came from: a
// rename or a move, which is identified again unless a person confirmed the match (D208 §1).
func hasMoved(file sqlcgen.ListIdentifyFilesRow) bool {
	return file.IdentifiedFromPath == nil || *file.IdentifiedFromPath != file.Path
}

// currentTitle is the title a file already belongs to, if it has one and has not moved.
func (filer *libraryFiler) currentTitle(ctx context.Context, file sqlcgen.ListIdentifyFilesRow) (int64, bool, error) {
	if file.VersionID == nil || hasMoved(file) {
		return 0, false, nil
	}
	current, err := filer.queries.GetVersionTitle(ctx, *file.VersionID)
	if err != nil {
		return 0, false, fmt.Errorf("read %s's version: %w", file.Path, err)
	}
	return current.TitleID, true, nil
}

// probeRuntimeMs is the probe's duration: a local fallback for the runtime (D183).
func probeRuntimeMs(probe *string) int64 {
	if probe == nil {
		return 0
	}
	parsed, err := library.DecodeProbe(*probe)
	if err != nil {
		return 0
	}
	return parsed.Format.DurationMs
}

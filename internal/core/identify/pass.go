// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
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
	"github.com/kstenerud/orpheum/internal/platform/ptr"
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
	owed           owedPasses
	// readsAsItGoes has the pass read each row when it needs it, as a match's filer does, rather
	// than from the snapshot: a test's, comparing the two.
	readsAsItGoes bool
	// batchRows is how many rows a batch changes before it commits: passBatchRows, or a test's.
	batchRows int64
	// betweenBatches runs after each batch commits: a test's, writing between them.
	betweenBatches func(ctx context.Context, rowsChanged int64)
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
	// UnusedTitleCount is how many titles the pass made and gave no file, which it merged into the
	// file's: a fault in filing, which the log names (retireEmptied).
	UnusedTitleCount int
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
		settings: sortSettings, batchRows: passBatchRows,
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

// IdentifyLibrary identifies every video in a library: in a read alone when nothing would change,
// and else in batches of whole filing units, each one short write (D303, DL9; D304 §2).
func (pass *Pass) IdentifyLibrary(ctx context.Context, libraryID int64) (Report, error) {
	sorter, err := pass.sorter(ctx)
	if err != nil {
		return Report{}, err
	}
	report, err := pass.fileLibrary(ctx, libraryID, sorter)
	if err != nil {
		return Report{}, fmt.Errorf("identify library %d: %w", libraryID, err)
	}
	if err := pass.deriveLibraryOrderings(ctx, libraryID); err != nil {
		return Report{}, fmt.Errorf("identify library %d: %w", libraryID, err)
	}
	if report.UnusedTitleCount > 0 {
		pass.logger.WarnContext(ctx, "the pass made titles it gave no file, and merged them: a fault in filing",
			"libraryId", libraryID, "titles", report.UnusedTitleCount)
	}
	// The locale may have changed after this pass read it: any name it keyed in the old one is
	// keyed again in the new.
	if _, err := pass.RefreshSortAndSearchKeys(ctx); err != nil {
		return Report{}, err
	}
	pass.runAfterPassHooks(ctx, report)
	return report, nil
}

// passBatchRows is how many rows a batch of the pass changes before it commits and hands the
// writer on (D304 §2): a few milliseconds' writing, about 25 at the reference library's 0.44 ms a
// film of some ten rows. A unit is never split, so the batch that begins one writes it whole.
const passBatchRows = 400

// passReadsOnAReader is how often a pass reads its library again on a reader because someone else
// wrote while it filed in batches. After that it reads it again inside the batch that finds the
// change, holding the writer for the read: a library being matched is written to every second, by
// the fetches its new titles queue, and a pass that only ever read on a reader never finished
// (staging, 2026-10-16: "read the library 20 times while others wrote", every pass of the shows).
const passReadsOnAReader = 3

// errLibraryChanged is a batch finding that someone else committed since the pass read the library:
// what the pass decided from that read may be stale, so it reads again (D303, M1).
var errLibraryChanged = errors.New("the database changed since the pass read the library")

// fileLibrary files a library's units: in a read alone when nothing would change, which never
// takes the writer, and else in batches. It first adopts the titles and series of the videos it
// governs that their filing library files no more (adopt).
func (pass *Pass) fileLibrary(ctx context.Context, libraryID int64, sorter *textindex.Sorter) (Report, error) {
	if err := pass.adopt(ctx, libraryID); err != nil {
		return Report{}, err
	}
	var report Report
	wouldWrite, err := pass.database.WouldWrite(ctx, func(queries *sqlcgen.Queries) error {
		report = Report{}
		filer := pass.newFiler(queries, sorter, &report)
		if loadErr := filer.load(ctx, libraryID); loadErr != nil {
			return loadErr
		}
		units, unitsErr := filer.filingUnits(ctx)
		if unitsErr != nil {
			return unitsErr
		}
		if _, identifyErr := filer.identifyUnits(ctx, units, -1, 0); identifyErr != nil {
			return identifyErr
		}
		return filer.listAwaitingMatch(ctx, libraryID)
	})
	if err != nil {
		return Report{}, fmt.Errorf("read it to file: %w", err)
	}
	if !wouldWrite {
		report.allImagesSaid(libraryID)
		return report, nil
	}
	report = Report{}
	filer, err := pass.fileInBatches(ctx, libraryID, sorter, &report)
	if err != nil {
		return Report{}, err
	}
	filer.queries = pass.database.Read()
	report.allImagesSaid(libraryID)
	return report, filer.listAwaitingMatch(ctx, libraryID)
}

// allImagesSaid makes a pass's report of the images beside the media a set, empty when no unit
// found one: none said is every image gone, which the artwork service retires (D215 §1).
func (report *Report) allImagesSaid(libraryID int64) {
	if report.BesideTheMedia == nil {
		report.BesideTheMedia = &LocalImages{LibraryID: libraryID}
	}
}

// fileInBatches reads the library on a reader, then files its units in batches, each one write that
// changes no more than batchRows rows but for its last unit, in the order of their paths. Each
// batch first checks that no one else committed since the read; if someone did, it reads again and
// goes on with every unit not yet written whole (D303, M1): on a reader the first few times, then
// inside the batch, so each batch makes progress however often others write.
func (pass *Pass) fileInBatches(
	ctx context.Context, libraryID int64, sorter *textindex.Sorter, report *Report,
) (*libraryFiler, error) {
	written := map[int64]bool{} // the files of the units written
	read := func(queries *sqlcgen.Queries) (*libraryFiler, []filingUnit, error) {
		filer := pass.newFiler(queries, sorter, report)
		if err := filer.load(ctx, libraryID); err != nil {
			return nil, nil, err
		}
		units, err := filer.filingUnits(ctx)
		if err != nil {
			return nil, nil, err
		}
		return filer, slices.DeleteFunc(units, func(unit filingUnit) bool { return unit.isWritten(written) }), nil
	}
	for reads := 1; ; reads++ {
		commitsAtRead := pass.database.Commits()
		var filer *libraryFiler
		var units []filingUnit
		err := pass.database.ReadTx(ctx, func(queries *sqlcgen.Queries) error {
			var readErr error
			filer, units, readErr = read(queries)
			return readErr
		})
		if err != nil {
			return nil, fmt.Errorf("read it to file in batches: %w", err)
		}
		var readInBatch func(*sqlcgen.Queries) (*libraryFiler, []filingUnit, error)
		if reads >= passReadsOnAReader {
			readInBatch = read
		}
		filer, err = pass.writeBatches(ctx, filer, units, commitsAtRead, written, readInBatch)
		if !errors.Is(err, errLibraryChanged) {
			return filer, err
		}
	}
}

// newFiler is a pass's filer over one library, reporting into report.
func (pass *Pass) newFiler(queries *sqlcgen.Queries, sorter *textindex.Sorter, report *Report) *libraryFiler {
	return &libraryFiler{
		queries: queries, nowMs: pass.clock.Now().UnixMilli(), report: report, providers: pass.providers,
		sorter:       sorter,
		titleIDByKey: map[string]int64{}, seriesIDByName: map[string]int64{}, nfoByVideo: map[int64]NFO{},
		showNFOs: map[string]NFO{}, nfosByFolder: map[string][]namedNFO{}, titleIDByFolder: map[string]int64{},
		seriesIDByFolder: map[string]int64{}, seriesMatchByID: map[int64]sqlcgen.GetSeriesForMatchRow{},
		listings: map[string][]Episode{}, returningEpisodes: map[int64]bool{},
		readsAsItGoes: pass.readsAsItGoes,
	}
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

	nfoByVideo map[int64]NFO // an NFO attached to its video
	// seriesWithUnplaced are the library's series with an episode its name may place, read once a
	// pass the first time a series' seasons to fetch are asked (seasonsForUnplaced).
	seriesWithUnplaced map[int64]bool
	nfosByFolder       map[string][]namedNFO
	showNFOs           map[string]NFO   // a tvshow.nfo, by its folder
	titleIDByKey       map[string]int64 // by key
	seriesIDByName     map[string]int64 // by name, as textindex.SameWork folds it
	titleIDByFolder    map[string]int64 // a film's folder, for its extras
	evidence           map[int64][]titleSpec
	titleIDByFile      map[int64]int64
	extraReasonByFile  map[int64]string       // an extra's file, and why it is one (D163)
	localArt           map[int64][]LocalImage // the images beside each item (D215)
	seriesIDByFolder   map[string]int64       // the folder a series' images sit in
	seriesSpecByID     map[int64]seriesSpec   // what this pass's files say about each series
	seriesMatchByID    map[int64]sqlcgen.GetSeriesForMatchRow
	listings           map[string][]Episode // parsed season listings, by "series/season"
	// returningEpisodes are the episodes of a series brought back into this library, each brought
	// back with it when a file of it is filed (D208 section 2).
	returningEpisodes map[int64]bool
	// contradicted are the files whose place their path now contradicts, filed again as a moved
	// file is (contradictedPlacements).
	contradicted map[int64]bool
	// leftFor are titles a file left this pass, each with the title the file is in: one it moved a
	// version out of, or one made for a file that stayed where it was. retireEmptied merges each
	// left with no version. added are the titles the pass made.
	leftFor map[int64]int64
	added   map[int64]bool
	// snap is what the pass read of the library at its start, which it compares against before it
	// writes; none in a match's filer, or with readsAsItGoes.
	snap          snapshot
	readsAsItGoes bool
	// recordedByFile is when each of an other library's files says it was recorded, as its probe
	// read it, for those that say (D349 §4).
	recordedByFile map[int64]int64
	// filesSay, when a rescan reads the library, collects what each film's and series' files say
	// it is, its match set aside (filesSayOf); nil in every other pass.
	filesSay map[int64]filesQuery
}

// identifyUnits files units in turn. With changedAtStart a count of rows the writer had changed
// when the batch began, it stops after the unit that brings the batch's changes to batchRows, and
// returns the units left; with -1, a read's, it files them all.
func (filer *libraryFiler) identifyUnits(
	ctx context.Context, units []filingUnit, changedAtStart, batchRows int64,
) ([]filingUnit, error) {
	for index, unit := range units {
		if err := filer.identifyUnit(ctx, unit); err != nil {
			return nil, err
		}
		if changedAtStart < 0 {
			continue
		}
		changed, err := filer.queries.CountRowsChangedByTheWriter(ctx)
		if err != nil {
			return nil, fmt.Errorf("count the rows changed: %w", err)
		}
		if changed-changedAtStart >= batchRows {
			return units[index+1:], nil
		}
	}
	return nil, nil
}

// identifyUnit files one unit: what the pass gathers of its files' evidence is the unit's alone.
func (filer *libraryFiler) identifyUnit(ctx context.Context, unit filingUnit) error {
	filer.files = unit.files
	filer.evidence, filer.titleIDByFile = map[int64][]titleSpec{}, map[int64]int64{}
	filer.extraReasonByFile, filer.localArt = map[int64]string{}, map[int64][]LocalImage{}
	filer.seriesSpecByID, filer.contradicted = map[int64]seriesSpec{}, map[int64]bool{}
	filer.leftFor, filer.added = map[int64]int64{}, map[int64]bool{}
	return filer.identify(ctx)
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

// identify files every video: films and episodes first, so an extra can find its film; an other
// library's each as a title of its own.
func (filer *libraryFiler) identify(ctx context.Context) error {
	if filer.isOtherLibrary() {
		return filer.identifyOther(ctx)
	}
	if err := filer.contradictedPlacements(ctx); err != nil {
		return err
	}
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
		if extra, isExtra := detectExtra(relativePath); isExtra && !filer.isSeriesLibrary() {
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
		filer.noteSeriesFilesSay(seriesID, filer.seriesSpecByID[seriesID])
		if err := filer.resolveSeries(ctx, seriesID, filer.seriesSpecByID[seriesID]); err != nil {
			return err
		}
	}
	filer.reportLocalImages()
	return filer.retireEmptied(ctx)
}

// retireEmptied merges each title a file left this pass, once it has no version left, into the
// title the file is in, as a match's join does (D173): a renamed file identified again, an episode
// an older reading misplaced, and a title made for a file that stayed where it was. Left live, each
// stayed in its series and its library with no file, one more each pass that made it (the owner's
// NAS, 2026-10-16: 80,246 under one series). A title the pass made and merged is a fault in filing,
// which the log names.
func (filer *libraryFiler) retireEmptied(ctx context.Context) error {
	for _, titleID := range slices.Sorted(maps.Keys(filer.leftFor)) {
		into := filer.leftFor[titleID]
		merged, err := filer.queries.MergeTitleLeftWithoutVersions(ctx, sqlcgen.MergeTitleLeftWithoutVersionsParams{
			MergedInto: &into, RemovedAt: &filer.nowMs, TitleID: titleID,
		})
		if err != nil {
			return fmt.Errorf("merge title %d, left with no version, into %d: %w", titleID, into, err)
		}
		if merged == 0 {
			continue
		}
		if filer.added[titleID] {
			filer.report.UnusedTitleCount++
		}
		filer.forgetTitle(titleID)
	}
	return nil
}

// forgetTitle drops a title the pass merged from what it finds titles by, so a later unit files
// nothing into a tombstone.
func (filer *libraryFiler) forgetTitle(titleID int64) {
	maps.DeleteFunc(filer.titleIDByKey, func(_ string, held int64) bool { return held == titleID })
	delete(filer.snap.titles, titleID)
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
		parsed := filer.parse(relativePath)
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

// isVideo is whether a file is a video the pass files: read, or stored unread by its listing, which
// a pass files from its path at once (D304 §1).
func isVideo(file sqlcgen.ListIdentifyFilesRow) bool {
	return file.SidecarKind == nil && file.Filing != library.FilingSkipped && file.Filing != library.FilingErrored &&
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
	parsed := filer.parse(relativePath)
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

// isSeriesLibrary is whether every video of the library is an episode of a series.
func (filer *libraryFiler) isSeriesLibrary() bool {
	return filer.library.Kind == library.LibraryKindShows
}

// parse reads a path as the library's kind says: in a series library, as an episode or a special
// whatever its name (ParseEpisodePath), never a film.
func (filer *libraryFiler) parse(relativePath string) ParsedPath {
	if filer.isSeriesLibrary() {
		return ParseEpisodePath(relativePath)
	}
	return ParsePath(relativePath)
}

// contradictedPlacements finds, in a series library, the files whose place the path now
// contradicts, which the pass files again as it would a moved file, unless a person confirmed them
// (D208 §1): one filed as a film, or an episode in a series no one matched whose name its folders
// no longer give. Kept where they were, the episodes an older reading of the folders made series of
// their own, and the featurettes it made films, stayed there however often the library was scanned
// (the owner's NAS, 2026-10-13).
func (filer *libraryFiler) contradictedPlacements(ctx context.Context) error {
	if !filer.isSeriesLibrary() {
		return nil
	}
	for _, file := range filer.files {
		relativePath, ok := filer.relativePath(file)
		if !isVideo(file) || !ok || file.VersionID == nil || hasMoved(file) {
			continue
		}
		current, err := filer.versionTitle(ctx, *file.VersionID)
		if err != nil {
			return fmt.Errorf("read %s's version: %w", file.Path, err)
		}
		if current.IdentificationState == identificationConfirmed {
			continue
		}
		place, isEpisode, err := filer.episodePlace(ctx, current.TitleID)
		if err != nil {
			return err
		}
		if !isEpisode {
			filer.contradicted[file.ID] = true // not an episode
			continue
		}
		series, err := filer.cachedSeriesMatch(ctx, place.CollectionID)
		if err != nil {
			return err
		}
		pathName := ParseEpisodePath(relativePath).Name
		named := textindex.SameWork(ptr.ValueOrZero(series.Name)) == textindex.SameWork(pathName)
		if !isSeriesMatched(series) && !named {
			filer.contradicted[file.ID] = true
		}
	}
	return nil
}

// isFiledAgain is whether a file's place is derived again rather than kept: it moved (D208 §1), or
// its path now contradicts its place.
func (filer *libraryFiler) isFiledAgain(file sqlcgen.ListIdentifyFilesRow) bool {
	return hasMoved(file) || filer.contradicted[file.ID]
}

// movieSpec is what a film's file and its NFO say about the work.
func (filer *libraryFiler) movieSpec(file sqlcgen.ListIdentifyFilesRow, parsed ParsedPath) titleSpec {
	spec := titleSpec{
		kind: KindMovie, name: parsed.Name, year: parsed.Year, ids: parsed.ProviderIDs,
		runtimeMs: file.DurationMs,
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
		var keptIn int64
		keptIn, err = filer.keepOrMove(ctx, file, titleID)
		if filer.added[titleID] {
			filer.noteLeft(titleID, keptIn)
		}
		titleID = keptIn
	}
	if err != nil {
		return err
	}
	filer.titleIDByFile[file.ID] = titleID
	filer.evidence[titleID] = append(filer.evidence[titleID], spec)
	return nil
}

// noteLeft records that a file left a title, or a title made for it, for the title it is in.
func (filer *libraryFiler) noteLeft(leftTitleID, inTitleID int64) {
	if filer.leftFor != nil && leftTitleID != inTitleID {
		filer.leftFor[leftTitleID] = inTitleID
	}
}

// newVersion gives a file its own Version under the title.
func (filer *libraryFiler) newVersion(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, titleID int64) error {
	version, err := filer.queries.InsertVersion(ctx,
		sqlcgen.InsertVersionParams{TitleID: titleID, CreatedAt: filer.nowMs})
	if err != nil {
		return fmt.Errorf("add a version for %s: %w", file.Path, err)
	}
	err = library.PutInVersion(ctx, filer.queries, file.ID, filer.library.Kind, version)
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
	current, err := filer.versionTitle(ctx, *file.VersionID)
	if err != nil {
		return 0, fmt.Errorf("read %s's version: %w", file.Path, err)
	}
	if current.IdentificationState == identificationConfirmed || !filer.isFiledAgain(file) {
		titleID = current.TitleID
	} else if current.TitleID != titleID {
		err = filer.moveVersion(ctx, *file.VersionID, titleID)
		filer.noteLeft(current.TitleID, titleID)
		if err != nil {
			return 0, fmt.Errorf("move %s to its title: %w", file.Path, err)
		}
	}
	if hasMoved(file) {
		// Record the path this identification came from.
		err = library.PutInVersion(ctx, filer.queries, file.ID, filer.library.Kind, *file.VersionID)
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

// currentTitle is the title a file already belongs to, if it has one and stays in it as keepOrMove
// keeps it: it has not moved, or a person confirmed the title. Asked before a title is found or
// made for the file, it keeps the pass from making one the file never goes to.
func (filer *libraryFiler) currentTitle(ctx context.Context, file sqlcgen.ListIdentifyFilesRow) (int64, bool, error) {
	if file.VersionID == nil {
		return 0, false, nil
	}
	current, err := filer.versionTitle(ctx, *file.VersionID)
	if err != nil {
		return 0, false, fmt.Errorf("read %s's version: %w", file.Path, err)
	}
	if current.IdentificationState != identificationConfirmed && filer.isFiledAgain(file) {
		return 0, false, nil
	}
	return current.TitleID, true, nil
}

// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Pass turns a library's stored observations into titles (identification.md). It reads only
// what the database holds — paths, probes, NFO bytes — so it opens no file, and it can run again
// at any time and reach the same answer (D182).
type Pass struct {
	database  *store.Database
	clock     clock.Clock
	logger    *slog.Logger
	providers Providers
	settings  Settings
	after     []func(context.Context, Report)
}

// AfterPass calls follow after each pass and each re-derivation a match makes, with what it left
// owing, once it is stored. Set them before the pass runs.
func (pass *Pass) AfterPass(follow func(context.Context, Report)) {
	pass.after = append(pass.after, follow)
}

// owe hands what a pass or a match left owing to the queues that follow it.
func (pass *Pass) owe(ctx context.Context, report Report) {
	for _, follow := range pass.after {
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
	Listed       int
	Unidentified int
	Extras       int
	// AwaitingMatch are the films and series no source has answered for yet: the fetch queue's
	// first fetches (D170, D209).
	AwaitingMatch []int64
	// AwaitingListings are matched series with a season listing to fetch: missing, or older
	// than a new episode it does not list (D209 §3).
	AwaitingListings []int64
	// Artwork are the images items' sources name that are not stored yet (D212).
	Artwork []OwedArtwork
}

// NewPass returns the identification pass.
func NewPass(
	database *store.Database, clockSource clock.Clock, logger *slog.Logger, providers Providers, config Settings,
) *Pass {
	return &Pass{
		database: database, clock: clockSource, logger: logger, providers: cleanProviders(providers), settings: config,
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

// Library identifies every video in a library, in one transaction.
func (pass *Pass) Library(ctx context.Context, libraryID int64) (Report, error) {
	var report Report
	sorter, err := pass.sorter(ctx)
	if err != nil {
		return Report{}, err
	}
	err = pass.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		work := &run{
			queries: queries, now: pass.clock.Now().UnixMilli(), report: &report, providers: pass.providers,
			sorter:       sorter,
			titleIDByKey: map[string]int64{}, seriesIDByName: map[string]int64{}, nfoByVideo: map[int64]NFO{},
			showNFOs: map[string]NFO{}, folderNFOs: map[string][]namedNFO{}, titleIDByFolder: map[string]int64{},
			evidence: map[int64][]titleSpec{}, titleIDByFile: map[int64]int64{}, extraOf: map[int64]string{},
			localArt: map[int64][]ArtRef{}, seriesFolders: map[string]int64{},
			seriesSeen: map[int64]seriesSpec{}, seriesRows: map[int64]sqlcgen.GetSeriesForMatchRow{},
			listings: map[string][]Episode{},
		}
		if loadErr := work.load(ctx, libraryID); loadErr != nil {
			return loadErr
		}
		if identifyErr := work.identify(ctx); identifyErr != nil {
			return identifyErr
		}
		return work.awaiting(ctx, libraryID)
	})
	if err != nil {
		return Report{}, fmt.Errorf("identify library %d: %w", libraryID, err)
	}
	// The locale may have changed after this pass read it: any name it keyed in the old one is
	// keyed again in the new.
	if _, err := pass.RefreshSortKeys(ctx); err != nil {
		return Report{}, err
	}
	pass.owe(ctx, report)
	return report, nil
}

// run is one pass over one library.
type run struct {
	queries   *sqlcgen.Queries
	now       int64
	report    *Report
	providers Providers
	sorter    *textindex.Sorter
	library   sqlcgen.GetLibraryForIdentifyRow
	roots     []sqlcgen.ListLibraryRootsRow
	files     []sqlcgen.ListIdentifyFilesRow

	nfoByVideo      map[int64]NFO // an NFO attached to its video
	folderNFOs      map[string][]namedNFO
	showNFOs        map[string]NFO   // a tvshow.nfo, by its folder
	titleIDByKey    map[string]int64 // by key
	seriesIDByName  map[string]int64 // by name, lower case
	titleIDByFolder map[string]int64 // a film's folder, for its extras
	evidence        map[int64][]titleSpec
	titleIDByFile   map[int64]int64
	extraOf         map[int64]string     // an extra's file, and why it is one (D163)
	localArt        map[int64][]ArtRef   // the images beside each item (D215)
	seriesFolders   map[string]int64     // the folder a series' images sit in
	seriesSeen      map[int64]seriesSpec // what this pass's files say about each series
	seriesRows      map[int64]sqlcgen.GetSeriesForMatchRow
	listings        map[string][]Episode // parsed season listings, by "series/season"
}

// awaiting lists what the fetch queue owes the library: films and series never answered for.
func (work *run) awaiting(ctx context.Context, libraryID int64) error {
	if work.library.MetadataSource == sourceNFO {
		return nil
	}
	films, err := work.queries.ListTitlesAwaitingMatch(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list the films awaiting a match: %w", err)
	}
	series, err := work.queries.ListSeriesAwaitingMatch(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list the series awaiting a match: %w", err)
	}
	work.report.AwaitingMatch = slices.Concat(films, series)
	return nil
}

// identify files every video: films and episodes first, so an extra can find its film.
func (work *run) identify(ctx context.Context) error {
	if err := work.knowSeriesByFolder(ctx); err != nil {
		return err
	}
	var extras []pendingExtra
	for _, file := range work.files {
		if !isVideo(file) {
			continue
		}
		rel, ok := work.relative(file)
		if !ok {
			continue
		}
		if extra, isExtra := extraOf(rel); isExtra {
			extras = append(extras, pendingExtra{file: file, rel: rel, extra: extra})
			continue
		}
		if err := work.work(ctx, file, rel); err != nil {
			return err
		}
	}
	for _, pending := range extras {
		if err := work.extra(ctx, pending); err != nil {
			return err
		}
	}
	work.mapLocalArtwork()
	if err := work.resolveAndFile(ctx); err != nil {
		return err
	}
	for _, seriesID := range slices.Sorted(maps.Keys(work.seriesSeen)) {
		if err := work.resolveSeries(ctx, seriesID, work.seriesSeen[seriesID]); err != nil {
			return err
		}
	}
	for _, itemID := range slices.Sorted(maps.Keys(work.localArt)) {
		if err := work.oweArtwork(ctx, itemID, SourceLocal, work.localArt[itemID]); err != nil {
			return err
		}
	}
	return work.retireLocalArtwork(ctx)
}

// knowSeriesByFolder lets the name an episode's folders give find the series its neighbours are
// already in, before any file is filed: a series renamed by its match (D209) keeps a new episode
// that sorts before its old ones.
func (work *run) knowSeriesByFolder(ctx context.Context) error {
	for _, file := range work.files {
		rel, ok := work.relative(file)
		if !isVideo(file) || !ok {
			continue
		}
		parsed := ParsePath(rel)
		if parsed.Kind != kindEpisode {
			continue
		}
		key := strings.ToLower(parsed.Name)
		if _, known := work.seriesIDByName[key]; known {
			continue
		}
		seriesID, err := work.currentSeries(ctx, file)
		if err != nil {
			return err
		}
		if seriesID != 0 {
			work.seriesIDByName[key] = seriesID
		}
	}
	return nil
}

func isVideo(file sqlcgen.ListIdentifyFilesRow) bool {
	return file.PayloadHash != nil && file.Filing != "skipped" && file.Filing != "errored" &&
		file.Filing != filingAttached
}

// relative is a file's path below the library root that holds it.
func (work *run) relative(file sqlcgen.ListIdentifyFilesRow) (string, bool) {
	best := ""
	for _, root := range work.roots {
		if root.NodeID == file.NodeID && strings.HasPrefix(file.Path, root.Path+"/") && len(root.Path) > len(best) {
			best = root.Path
		}
	}
	if best == "" {
		return "", false
	}
	return strings.TrimPrefix(file.Path, best+"/"), true
}

// work files a film or an episode.
func (work *run) work(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, rel string) error {
	parsed := ParsePath(rel)
	if parsed.Kind == kindEpisode {
		return work.episode(ctx, file, parsed)
	}
	spec := work.movieSpec(file, parsed)
	titleID, kept, err := work.currentTitle(ctx, file)
	if err == nil && !kept {
		titleID, err = work.titleFor(ctx, spec)
	}
	if err != nil {
		return err
	}
	work.titleIDByFolder[path.Dir(file.Path)] = titleID
	return work.place(ctx, file, titleID, spec)
}

// movieSpec is what a film's file and its NFO say about the work.
func (work *run) movieSpec(file sqlcgen.ListIdentifyFilesRow, parsed Parsed) titleSpec {
	spec := titleSpec{
		kind: kindMovie, name: parsed.Name, year: parsed.Year, ids: parsed.ProviderIDs,
		runtimeMs: probeRuntime(file.Probe),
	}
	if nfo, found := work.nfoFor(file); found && nfo.Kind == kindMovie {
		spec.fromNFO(nfo)
	}
	work.unidentifiedUnlessNFO(&spec)
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
func (work *run) nfoFor(file sqlcgen.ListIdentifyFilesRow) (NFO, bool) {
	if nfo, found := work.nfoByVideo[file.ID]; found {
		return nfo, true
	}
	videoStem := strings.TrimSuffix(path.Base(file.Path), path.Ext(file.Path))
	chosen, score := NFO{}, -1
	for _, candidate := range work.folderNFOs[path.Dir(file.Path)] {
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
func (work *run) unidentifiedUnlessNFO(spec *titleSpec) {
	if spec.source != "" {
		return // its NFO, or its series' listing, speaks for it
	}
	spec.ident = identUnidentified
	switch {
	case work.library.MetadataSource == sourceNFO:
		spec.source, spec.reason = sourceNone, "no NFO file"
	case spec.reason == "":
		spec.reason = "not matched yet"
	}
}

// place gives a file its Version under the title, unless a person confirmed the file's current
// title: an automatic identification follows the path, a confirmed one never moves (D208 §1).
func (work *run) place(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, titleID int64, spec titleSpec) error {
	var err error
	if file.VersionID == nil {
		err = work.newVersion(ctx, file, titleID)
	} else {
		titleID, err = work.keepOrMove(ctx, file, titleID)
	}
	if err != nil {
		return err
	}
	work.titleIDByFile[file.ID] = titleID
	work.evidence[titleID] = append(work.evidence[titleID], spec)
	return nil
}

// newVersion gives a file its own Version under the title.
func (work *run) newVersion(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, titleID int64) error {
	version, err := work.queries.InsertVersion(ctx, sqlcgen.InsertVersionParams{TitleID: titleID, CreatedAt: work.now})
	if err != nil {
		return fmt.Errorf("add a version for %s: %w", file.Path, err)
	}
	err = work.queries.SetFileVersion(ctx, sqlcgen.SetFileVersionParams{VersionID: &version, ID: file.ID})
	if err != nil {
		return fmt.Errorf("put %s in its version: %w", file.Path, err)
	}
	return nil
}

// keepOrMove keeps a file's Version where it is when the file has not moved or a person
// confirmed its title, and otherwise moves it to titleID. It returns the title the file ends in.
func (work *run) keepOrMove(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, titleID int64) (int64, error) {
	current, err := work.queries.GetVersionTitle(ctx, *file.VersionID)
	if err != nil {
		return 0, fmt.Errorf("read %s's version: %w", file.Path, err)
	}
	if current.Identification == identConfirmed || !moved(file) {
		titleID = current.TitleID
	} else if current.TitleID != titleID {
		move := sqlcgen.MoveVersionToTitleParams{TitleID: titleID, ID: *file.VersionID}
		err = work.queries.MoveVersionToTitle(ctx, move)
		if err != nil {
			return 0, fmt.Errorf("move %s to its title: %w", file.Path, err)
		}
	}
	if moved(file) {
		// Record the path this identification came from.
		err = work.queries.SetFileVersion(ctx, sqlcgen.SetFileVersionParams{VersionID: file.VersionID, ID: file.ID})
		if err != nil {
			return 0, fmt.Errorf("record %s's identification: %w", file.Path, err)
		}
	}
	return titleID, nil
}

// moved reports whether a file's path differs from the one its identification came from: a
// rename or a move, which is identified again unless a person confirmed the match (D208 §1).
func moved(file sqlcgen.ListIdentifyFilesRow) bool {
	return file.IdentifiedPath == nil || *file.IdentifiedPath != file.Path
}

// currentTitle is the title a file already belongs to, if it has one and has not moved.
func (work *run) currentTitle(ctx context.Context, file sqlcgen.ListIdentifyFilesRow) (int64, bool, error) {
	if file.VersionID == nil || moved(file) {
		return 0, false, nil
	}
	current, err := work.queries.GetVersionTitle(ctx, *file.VersionID)
	if err != nil {
		return 0, false, fmt.Errorf("read %s's version: %w", file.Path, err)
	}
	return current.TitleID, true, nil
}

// probeRuntime is the probe's duration: a local fallback for the runtime (D183).
func probeRuntime(probe *string) int64 {
	if probe == nil {
		return 0
	}
	var parsed nodeproto.Probe
	if err := json.Unmarshal([]byte(*probe), &parsed); err != nil {
		return 0
	}
	return parsed.Format.DurationMs
}

func deref(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

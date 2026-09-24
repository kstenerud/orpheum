// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"strings"

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
}

// Providers are the metadata providers, by name. The pass only parses their stored responses.
type Providers map[string]Provider

// Report is what one pass filed.
type Report struct {
	Listed       int
	Unidentified int
	// AwaitingMatch are the films no source has answered for yet: the fetch queue's work (D170).
	AwaitingMatch []int64
}

// NewPass returns the identification pass.
func NewPass(database *store.Database, now clock.Clock, logger *slog.Logger, providers Providers) *Pass {
	return &Pass{database: database, clock: now, logger: logger, providers: providers}
}

// Library identifies every video in a library, in one transaction.
func (pass *Pass) Library(ctx context.Context, libraryID int64) (Report, error) {
	var report Report
	err := pass.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		work := &run{
			queries: queries, now: pass.clock.Now().UnixMilli(), report: &report, providers: pass.providers,
			titles: map[string]int64{}, series: map[string]int64{}, nfoByVideo: map[int64]NFO{},
			showNFOs: map[string]NFO{}, folderNFOs: map[string][]namedNFO{}, folderTitle: map[string]int64{},
			evidence: map[int64][]titleSpec{}, fileTitle: map[int64]int64{},
		}
		if err := work.load(ctx, libraryID); err != nil {
			return err
		}
		if err := work.identify(ctx); err != nil {
			return err
		}
		awaiting, err := queries.ListTitlesAwaitingMatch(ctx, &libraryID)
		if err != nil {
			return fmt.Errorf("list the films awaiting a match: %w", err)
		}
		if work.library.MetadataSource != sourceNFO {
			report.AwaitingMatch = awaiting
		}
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("identify library %d: %w", libraryID, err)
	}
	return report, nil
}

// run is one pass over one library.
type run struct {
	queries   *sqlcgen.Queries
	now       int64
	report    *Report
	providers Providers
	library   sqlcgen.GetLibraryForIdentifyRow
	roots     []sqlcgen.ListLibraryRootsRow
	files     []sqlcgen.ListIdentifyFilesRow

	nfoByVideo  map[int64]NFO // an NFO attached to its video
	folderNFOs  map[string][]namedNFO
	showNFOs    map[string]NFO   // a tvshow.nfo, by its folder
	titles      map[string]int64 // by key
	series      map[string]int64 // by name, lower case
	folderTitle map[string]int64 // a film's folder, for its extras
	evidence    map[int64][]titleSpec
	fileTitle   map[int64]int64
}

// load reads the library, its roots, its files, and the titles and series it already has.
func (work *run) load(ctx context.Context, libraryID int64) error {
	var err error
	if work.library, err = work.queries.GetLibraryForIdentify(ctx, libraryID); err != nil {
		return fmt.Errorf("read the library: %w", err)
	}
	if work.roots, err = work.queries.ListLibraryRoots(ctx, libraryID); err != nil {
		return fmt.Errorf("list its roots: %w", err)
	}
	if work.files, err = work.queries.ListIdentifyFiles(ctx, libraryID); err != nil {
		return fmt.Errorf("list its files: %w", err)
	}
	for _, file := range work.files {
		isNFO := file.FilingReason != nil && *file.FilingReason == string(nodeproto.SidecarNFO)
		if file.Filing != "attached" || !isNFO {
			continue
		}
		body, bodyErr := work.queries.GetSidecarBody(ctx, file.ID)
		if bodyErr != nil {
			return fmt.Errorf("read the NFO %s: %w", file.Path, bodyErr)
		}
		nfo, parseErr := ParseNFO(body)
		if parseErr != nil {
			continue // not an NFO we can read: the file identifies without it
		}
		switch {
		case nfo.Kind == "tvshow":
			work.showNFOs[path.Dir(file.Path)] = nfo
		case file.AttachedTo != nil:
			work.nfoByVideo[*file.AttachedTo] = nfo
		}
		stem := strings.TrimSuffix(path.Base(file.Path), path.Ext(file.Path))
		folder := path.Dir(file.Path)
		work.folderNFOs[folder] = append(work.folderNFOs[folder], namedNFO{stem: stem, nfo: nfo})
	}
	return work.loadTitles(ctx, libraryID)
}

func (work *run) loadTitles(ctx context.Context, libraryID int64) error {
	titles, err := work.queries.ListLibraryTitles(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its titles: %w", err)
	}
	for _, title := range titles {
		if title.Name != nil && title.Kind == kindMovie {
			work.titles[movieKey(*title.Name, int(deref(title.Year)))] = title.ID
		}
	}
	series, err := work.queries.ListLibrarySeries(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its series: %w", err)
	}
	for _, show := range series {
		if show.Name == nil {
			continue
		}
		work.series[strings.ToLower(*show.Name)] = show.ID
		episodes, listErr := work.queries.ListEpisodesOfSeries(ctx, show.ID)
		if listErr != nil {
			return fmt.Errorf("list the episodes of %s: %w", *show.Name, listErr)
		}
		for _, episode := range episodes {
			work.titles[episodeKey(show.ID, int(deref(episode.Grp)), int(deref(episode.Number)))] = episode.TitleID
		}
	}
	return nil
}

// identify files every video: films and episodes first, so an extra can find its film.
func (work *run) identify(ctx context.Context) error {
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
	return work.resolveAndFile(ctx)
}

func isVideo(file sqlcgen.ListIdentifyFilesRow) bool {
	return file.PayloadHash != nil && file.Filing != "skipped" && file.Filing != "errored" && file.Filing != "attached"
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
	work.folderTitle[path.Dir(file.Path)] = titleID
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
	if spec.source == sourceNFO {
		return
	}
	spec.ident = identUnidentified
	if work.library.MetadataSource == sourceNFO {
		spec.source, spec.reason = sourceNone, "no NFO file"
	} else {
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
	work.fileTitle[file.ID] = titleID
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
	if current.Ident == identConfirmed || !moved(file) {
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

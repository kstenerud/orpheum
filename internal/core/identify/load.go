// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

// load reads the library, its roots, its files, and the titles and series it already has.
func (filer *libraryFiler) load(ctx context.Context, libraryID int64) error {
	var err error
	if filer.library, err = filer.queries.GetLibraryForIdentify(ctx, libraryID); err != nil {
		return fmt.Errorf("read the library: %w", err)
	}
	if filer.roots, err = filer.queries.ListLibraryRoots(ctx, libraryID); err != nil {
		return fmt.Errorf("list its roots: %w", err)
	}
	if filer.files, err = filer.queries.ListIdentifyFiles(ctx, libraryID); err != nil {
		return fmt.Errorf("list its files: %w", err)
	}
	if !filer.readsAsItGoes {
		if err := filer.loadSnapshot(ctx, libraryID); err != nil {
			return err
		}
	}
	if filer.library.MetadataSource != sourceTMDB {
		if err := filer.loadNFOs(ctx); err != nil {
			return err
		}
	}
	return filer.loadTitles(ctx, libraryID)
}

// loadNFOs reads the NFO files the library's source takes. A library asked for "TMDB only,
// ignore NFO files" reads none, not even their ids or locked fields (D183 §2); they stay filed
// as sidecars, so changing the library's source finds them.
func (filer *libraryFiler) loadNFOs(ctx context.Context) error {
	for _, file := range filer.files {
		isNFO := ptr.ValueOrZero(file.SidecarKind) == string(nodeproto.SidecarNFO)
		if file.Filing != library.FilingAttached || !isNFO {
			continue
		}
		body, bodyErr := filer.nfoBody(ctx, file.ID)
		if bodyErr != nil {
			return fmt.Errorf("read the NFO %s: %w", file.Path, bodyErr)
		}
		nfo, parseErr := ParseNFO(body)
		if parseErr != nil {
			continue // not an NFO we can read: the file identifies without it
		}
		switch {
		case nfo.Kind == "tvshow":
			filer.showNFOs[path.Dir(file.Path)] = nfo
		case file.AttachedToFile != nil:
			filer.nfoByVideo[*file.AttachedToFile] = nfo
		}
		stem := strings.TrimSuffix(path.Base(file.Path), path.Ext(file.Path))
		folder := path.Dir(file.Path)
		filer.nfosByFolder[folder] = append(filer.nfosByFolder[folder], namedNFO{stem: stem, nfo: nfo})
	}
	return nil
}

func (filer *libraryFiler) loadTitles(ctx context.Context, libraryID int64) error {
	titles, err := filer.queries.ListLibraryTitles(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its titles: %w", err)
	}
	for _, title := range titles {
		if title.Name != nil && title.Kind == KindMovie {
			filer.titleIDByKey[movieKey(*title.Name, int(ptr.ValueOrZero(title.Year)))] = title.ID
		}
	}
	series, err := filer.queries.ListLibrarySeries(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its series: %w", err)
	}
	for _, show := range series {
		if show.Name == nil {
			continue
		}
		filer.snap.holdSeries(show.ID)
		filer.seriesIDByName[textindex.SameWork(*show.Name)] = show.ID
		if err := filer.knowEpisodesOf(ctx, show.ID); err != nil {
			return err
		}
	}
	filer.snap.seriesPlaces = nil // the pass's writes would leave it behind: a later read asks
	return nil
}

// knowEpisodesOf lets a series' episodes be found at their places. One hidden with a deleted
// library comes back when a file of it is filed (D231 section 7).
func (filer *libraryFiler) knowEpisodesOf(ctx context.Context, seriesID int64) error {
	episodes, err := filer.seriesEpisodePlaces(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("list the episodes of series %d: %w", seriesID, err)
	}
	for _, episode := range episodes {
		filer.titleIDByKey[episodeKey(seriesID, int(ptr.ValueOrZero(episode.GroupNumber)),
			int(ptr.ValueOrZero(episode.NumberInGroup)))] = episode.TitleID
		if episode.RemovedAt != nil {
			filer.returningEpisodes[episode.TitleID] = true
		}
	}
	return nil
}

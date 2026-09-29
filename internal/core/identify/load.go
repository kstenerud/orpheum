// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"path"
	"strings"

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
		if file.Filing != filingAttached || !isNFO {
			continue
		}
		body, bodyErr := filer.queries.GetNFOBody(ctx, file.ID)
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
		filer.seriesIDByName[strings.ToLower(*show.Name)] = show.ID
		episodes, listErr := filer.queries.ListSeriesEpisodePlaces(ctx, show.ID)
		if listErr != nil {
			return fmt.Errorf("list the episodes of %s: %w", *show.Name, listErr)
		}
		for _, episode := range episodes {
			filer.titleIDByKey[episodeKey(show.ID, int(ptr.ValueOrZero(episode.GroupNumber)),
				int(ptr.ValueOrZero(episode.NumberInGroup)))] = episode.TitleID
		}
	}
	return nil
}

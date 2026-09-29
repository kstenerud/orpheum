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
	if work.library.MetadataSource != sourceTMDB {
		if err := work.loadNFOs(ctx); err != nil {
			return err
		}
	}
	return work.loadTitles(ctx, libraryID)
}

// loadNFOs reads the NFO files the library's source takes. A library asked for "TMDB only,
// ignore NFO files" reads none, not even their ids or locked fields (D183 §2); they stay filed
// as sidecars, so changing the library's source finds them.
func (work *run) loadNFOs(ctx context.Context) error {
	for _, file := range work.files {
		isNFO := ptr.ValueOrZero(file.SidecarKind) == string(nodeproto.SidecarNFO)
		if file.Filing != filingAttached || !isNFO {
			continue
		}
		body, bodyErr := work.queries.GetNFOBody(ctx, file.ID)
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
		case file.AttachedToFile != nil:
			work.nfoByVideo[*file.AttachedToFile] = nfo
		}
		stem := strings.TrimSuffix(path.Base(file.Path), path.Ext(file.Path))
		folder := path.Dir(file.Path)
		work.folderNFOs[folder] = append(work.folderNFOs[folder], namedNFO{stem: stem, nfo: nfo})
	}
	return nil
}

func (work *run) loadTitles(ctx context.Context, libraryID int64) error {
	titles, err := work.queries.ListLibraryTitles(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its titles: %w", err)
	}
	for _, title := range titles {
		if title.Name != nil && title.Kind == kindMovie {
			work.titleIDByKey[movieKey(*title.Name, int(deref(title.Year)))] = title.ID
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
		work.seriesIDByName[strings.ToLower(*show.Name)] = show.ID
		episodes, listErr := work.queries.ListSeriesEpisodePlaces(ctx, show.ID)
		if listErr != nil {
			return fmt.Errorf("list the episodes of %s: %w", *show.Name, listErr)
		}
		for _, episode := range episodes {
			work.titleIDByKey[episodeKey(show.ID, int(deref(episode.GroupNumber)),
				int(deref(episode.NumberInGroup)))] = episode.TitleID
		}
	}
	return nil
}

// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// TitleDetail is a film, episode or extra with what a detail page shows: its versions and their
// files and streams, its extras, and, for an episode, its series and place.
type TitleDetail struct {
	sqlcgen.GetTitleDetailRow
	Versions []Version
	Extras   []sqlcgen.ListTitleExtrasRow
	Episode  *sqlcgen.GetEpisodePlaceRow
	Art      Art
}

// Version is one of a title's versions: its files in part order (D155).
type Version struct {
	ID        int64
	EditionID *int64
	Files     []FileDetail
}

// FileDetail is one file of a version, with its streams.
type FileDetail struct {
	ID      int64
	Path    string
	Size    *int64
	Streams []sqlcgen.ListFileStreamsRow
}

// Title is a title's detail.
func (browser *Browser) Title(ctx context.Context, titleID int64) (TitleDetail, error) {
	queries := browser.database.Read()
	title, err := store.One(queries.GetTitleDetail(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return TitleDetail{}, fmt.Errorf("title %d: %w", titleID, ErrNoItem)
	}
	if err != nil {
		return TitleDetail{}, fmt.Errorf("read title %d: %w", titleID, err)
	}
	detail := TitleDetail{GetTitleDetailRow: title}
	if detail.Versions, err = browser.versions(ctx, titleID); err != nil {
		return TitleDetail{}, err
	}
	if detail.Extras, err = queries.ListTitleExtras(ctx, titleID); err != nil {
		return TitleDetail{}, fmt.Errorf("list title %d's extras: %w", titleID, err)
	}
	if detail.Art, err = browser.art(ctx, titleID); err != nil {
		return TitleDetail{}, err
	}
	place, err := store.One(queries.GetEpisodePlace(ctx, titleID))
	switch {
	case err == nil:
		detail.Episode = &place
	case !errors.Is(err, store.ErrNotFound):
		return TitleDetail{}, fmt.Errorf("read title %d's place: %w", titleID, err)
	}
	return detail, nil
}

func (browser *Browser) versions(ctx context.Context, titleID int64) ([]Version, error) {
	queries := browser.database.Read()
	files, err := queries.ListTitleFiles(ctx, titleID)
	if err != nil {
		return nil, fmt.Errorf("list title %d's files: %w", titleID, err)
	}
	versions := []Version{}
	for _, file := range files {
		if len(versions) == 0 || versions[len(versions)-1].ID != file.VersionID {
			versions = append(versions, Version{ID: file.VersionID, EditionID: file.EditionID})
		}
		streams, err := queries.ListFileStreams(ctx, file.FileID)
		if err != nil {
			return nil, fmt.Errorf("list file %d's streams: %w", file.FileID, err)
		}
		last := &versions[len(versions)-1]
		last.Files = append(last.Files, FileDetail{ID: file.FileID, Path: file.Path, Size: file.Size, Streams: streams})
	}
	return versions, nil
}

// SeriesDetail is a series with its episodes in aired order.
type SeriesDetail struct {
	sqlcgen.GetSeriesDetailRow
	Episodes []sqlcgen.ListSeriesEpisodeDetailsRow
	Art      Art
	Stills   map[int64]string // each episode's still, by its id
}

// Series is a series' detail.
func (browser *Browser) Series(ctx context.Context, seriesID int64) (SeriesDetail, error) {
	queries := browser.database.Read()
	series, err := store.One(queries.GetSeriesDetail(ctx, seriesID))
	if errors.Is(err, store.ErrNotFound) {
		return SeriesDetail{}, fmt.Errorf("series %d: %w", seriesID, ErrNoItem)
	}
	if err != nil {
		return SeriesDetail{}, fmt.Errorf("read series %d: %w", seriesID, err)
	}
	episodes, err := queries.ListSeriesEpisodeDetails(ctx, seriesID)
	if err != nil {
		return SeriesDetail{}, fmt.Errorf("list series %d's episodes: %w", seriesID, err)
	}
	detail := SeriesDetail{GetSeriesDetailRow: series, Episodes: episodes}
	if detail.Art, err = browser.art(ctx, seriesID); err != nil {
		return SeriesDetail{}, err
	}
	ids := make([]int64, 0, len(episodes))
	for _, episode := range episodes {
		ids = append(ids, episode.TitleID)
	}
	if detail.Stills, err = browser.shown(ctx, artStill, ids); err != nil {
		return SeriesDetail{}, err
	}
	return detail, nil
}

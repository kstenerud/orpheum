// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
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
	// Unreadable is why none of its files can be read now, or nil when one can (D241 §3).
	Unreadable *Unreadable
}

// Version is one of a title's versions: its files in part order (D155).
type Version struct {
	ID        int64
	EditionID *int64
	Files     []FileDetail
}

// FileDetail is one file of a version, with its streams.
type FileDetail struct {
	ID        int64
	Path      string
	SizeBytes *int64
	Streams   []sqlcgen.ListStreamsOfVideoFileRow
}

// Title is a title's detail; one in a library reach does not reach is ErrNoItem, as one that does
// not exist (D243 §4).
func (browser *Browser) Title(ctx context.Context, reach auth.Reach, titleID int64) (TitleDetail, error) {
	queries := browser.database.Read()
	title, err := store.OrNotFound(queries.GetTitleDetail(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && !reach.Reaches(title.LibraryID)) {
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
	if detail.Art, err = browser.itemArt(ctx, titleID); err != nil {
		return TitleDetail{}, err
	}
	if detail.Episode, err = browser.episodePlace(ctx, titleID); err != nil {
		return TitleDetail{}, err
	}
	unreadable, err := browser.unreadableItems(ctx, []int64{titleID}, false)
	if err != nil {
		return TitleDetail{}, err
	}
	if why, isUnreadable := unreadable[titleID]; isUnreadable {
		detail.Unreadable = &why
	}
	return detail, nil
}

// episodePlace is an episode's series and place in the aired order, or nil for a title in none.
func (browser *Browser) episodePlace(ctx context.Context, titleID int64) (*sqlcgen.GetEpisodePlaceRow, error) {
	place, err := store.OrNotFound(browser.database.Read().GetEpisodePlace(ctx, titleID))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, nil //nolint:nilnil // no place is an answer, not an error
	case err != nil:
		return nil, fmt.Errorf("read title %d's place: %w", titleID, err)
	}
	return &place, nil
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
		streams, err := queries.ListStreamsOfVideoFile(ctx, file.FileID)
		if err != nil {
			return nil, fmt.Errorf("list file %d's streams: %w", file.FileID, err)
		}
		last := &versions[len(versions)-1]
		last.Files = append(last.Files, FileDetail{
			ID: file.FileID, Path: file.Path, SizeBytes: file.SizeBytes,
			Streams: streams,
		})
	}
	return versions, nil
}

// SeriesDetail is a series with its episodes in aired order.
type SeriesDetail struct {
	sqlcgen.GetSeriesDetailRow
	Episodes               []sqlcgen.ListSeriesEpisodeDetailsRow
	Art                    Art
	StillSha256ByEpisodeID map[int64]string // each episode's still, by its id
	// UnreadableByEpisodeID is why none of an episode's files can be read now, for each such
	// episode; Unreadable is why none of the series' can, or nil when one can (D241 §3).
	UnreadableByEpisodeID map[int64]Unreadable
	Unreadable            *Unreadable
}

// Series is a series' detail; one in a library reach does not reach is ErrNoItem, as one that does
// not exist (D243 §4).
func (browser *Browser) Series(ctx context.Context, reach auth.Reach, seriesID int64) (SeriesDetail, error) {
	queries := browser.database.Read()
	series, err := store.OrNotFound(queries.GetSeriesDetail(ctx, seriesID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && !reach.Reaches(series.LibraryID)) {
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
	if detail.Art, err = browser.itemArt(ctx, seriesID); err != nil {
		return SeriesDetail{}, err
	}
	ids := make([]int64, 0, len(episodes))
	for _, episode := range episodes {
		ids = append(ids, episode.TitleID)
	}
	if detail.StillSha256ByEpisodeID, err = browser.shownArtSourceSha256s(ctx, ArtStill, ids); err != nil {
		return SeriesDetail{}, err
	}
	if detail.UnreadableByEpisodeID, err = browser.unreadableItems(ctx, ids, false); err != nil {
		return SeriesDetail{}, err
	}
	unreadable, err := browser.unreadableItems(ctx, []int64{seriesID}, true)
	if err != nil {
		return SeriesDetail{}, err
	}
	if why, isUnreadable := unreadable[seriesID]; isUnreadable {
		detail.Unreadable = &why
	}
	return detail, nil
}

// FileWork is what a file is, as now playing names it: its title, when one is filed, by id and name,
// and its path.
type FileWork struct {
	TitleID int64
	Name    string
	Path    string
}

// WorkOfFile is what a file is; library.ErrNoFile for one there is not.
func (browser *Browser) WorkOfFile(ctx context.Context, fileID int64) (FileWork, error) {
	row, err := store.OrNotFound(browser.database.Read().GetTitleOfFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return FileWork{}, fmt.Errorf("file %d: %w", fileID, ErrNoFile)
	}
	if err != nil {
		return FileWork{}, fmt.Errorf("read what file %d is: %w", fileID, err)
	}
	return FileWork{TitleID: ptr.ValueOrZero(row.TitleID), Name: ptr.ValueOrZero(row.Name), Path: row.Path}, nil
}

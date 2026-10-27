// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"

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
	Episode  *EpisodePlace
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

// Title is a title's detail as userID sees it, an episode placed in the ordering they watch its
// series in; one in a library reach does not reach is ErrNoItem, as one that does not exist (D243 §4).
func (browser *Browser) Title(ctx context.Context, reach auth.Reach, userID, titleID int64) (TitleDetail, error) {
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
	if detail.Episode, err = browser.EpisodePlaceOf(ctx, userID, titleID); err != nil {
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

// SeriesDetail is a series with its episodes in the ordering its person watches it in.
type SeriesDetail struct {
	sqlcgen.GetSeriesDetailRow
	// Ordering is the ordering its person watches it in (ViewingOrdering), and Orderings every one it
	// has, as its page offers them.
	Ordering  string
	Orderings []Ordering
	// NotInOrderingCount is how many of its episodes Ordering leaves out, listed after its groups.
	NotInOrderingCount int64
	// Seasons are the ordering's groups in its order, aired's seasons with the specials last: the
	// series' page lists them, and each one's episodes are read by Season (D306 §9).
	Seasons []SeasonSummary
	Art     Art
	// Unreadable is why none of the series' files can be read now, or nil when one can (D241 §3).
	Unreadable *Unreadable
}

// SeasonSummary is one season of a series, a group of the ordering it is shown in: its number,
// aired's 0 for the specials and the episodes with no season, how many episodes it has, and the
// still of its first episode that has one.
type SeasonSummary struct {
	Number int64
	// Name is the ordering's name for the group ("Disc 2"), "" where it names none, as aired names
	// none of its seasons.
	Name         string
	EpisodeCount int64
	StillSha256  string
}

// Series is a series' detail, its seasons summed up in the ordering userID watches it in; one in a
// library reach does not reach is ErrNoItem, as one that does not exist (D243 §4).
func (browser *Browser) Series(ctx context.Context, reach auth.Reach, userID, seriesID int64) (SeriesDetail, error) {
	queries := browser.database.Read()
	series, err := browser.reachedSeries(ctx, reach, seriesID)
	if err != nil {
		return SeriesDetail{}, err
	}
	ordering, err := ViewingOrdering(ctx, queries, userID, seriesID)
	if err != nil {
		return SeriesDetail{}, err
	}
	episodes, err := queries.ListSeriesEpisodeDetails(ctx, sqlcgen.ListSeriesEpisodeDetailsParams{
		CollectionID: seriesID, OrderingName: ordering,
	})
	if err != nil {
		return SeriesDetail{}, fmt.Errorf("list series %d's episodes in its %s ordering: %w", seriesID, ordering, err)
	}
	detail := SeriesDetail{GetSeriesDetailRow: series, Ordering: ordering}
	if detail.Art, err = browser.itemArt(ctx, seriesID); err != nil {
		return SeriesDetail{}, err
	}
	ids := make([]int64, 0, len(episodes))
	for _, episode := range episodes {
		ids = append(ids, episode.TitleID)
	}
	stills, err := browser.shownArtSourceSha256s(ctx, ArtStill, ids)
	if err != nil {
		return SeriesDetail{}, err
	}
	detail.Seasons = seasonsOf(episodes, stills, ordering == OrderingAired)
	err = browser.withOrderings(ctx, &detail)
	if err != nil {
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

// reachedSeries is a series reach reaches; ErrNoItem for one it does not, or that does not exist.
func (browser *Browser) reachedSeries(
	ctx context.Context, reach auth.Reach, seriesID int64,
) (sqlcgen.GetSeriesDetailRow, error) {
	series, err := store.OrNotFound(browser.database.Read().GetSeriesDetail(ctx, seriesID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && !reach.Reaches(series.LibraryID)) {
		return sqlcgen.GetSeriesDetailRow{}, fmt.Errorf("series %d: %w", seriesID, ErrNoItem)
	}
	if err != nil {
		return sqlcgen.GetSeriesDetailRow{}, fmt.Errorf("read series %d: %w", seriesID, err)
	}
	return series, nil
}

// seasonsOf sums a series' episodes, in an ordering's order, up by its groups, in order: in aired,
// numbered seasons in order, then the specials; in another, whose group 0 is a part like the rest
// (a TMDB episode group's first), by number alone.
func seasonsOf(
	episodes []sqlcgen.ListSeriesEpisodeDetailsRow, stills map[int64]string, isAired bool,
) []SeasonSummary {
	byNumber := map[int64]*SeasonSummary{}
	var numbers []int64
	for _, episode := range episodes {
		number := ptr.ValueOrZero(episode.GroupNumber)
		season, isSeen := byNumber[number]
		if !isSeen {
			season = &SeasonSummary{Number: number}
			byNumber[number] = season
			numbers = append(numbers, number)
		}
		season.EpisodeCount++
		if season.StillSha256 == "" {
			season.StillSha256 = stills[episode.TitleID]
		}
	}
	// The specials, aired's season 0, come after every numbered season.
	sortKey := func(number int64) int64 {
		if number == 0 && isAired {
			return math.MaxInt64
		}
		return number
	}
	slices.SortFunc(numbers, func(first, second int64) int { return cmp.Compare(sortKey(first), sortKey(second)) })
	seasons := make([]SeasonSummary, 0, len(numbers))
	for _, number := range numbers {
		seasons = append(seasons, *byNumber[number])
	}
	return seasons
}

// SeasonQuery asks for a page of one season's episodes as a person sees them: a series, the person,
// whose ordering's group the season is, its number, aired's 0 for the specials, a cursor from the
// page before ("" for the first) and how many at most.
type SeasonQuery struct {
	SeriesID int64
	UserID   int64
	Season   int64
	Cursor   string
	Limit    int64
}

// SeasonPage is a page of a season's episodes, in the order of the ordering it is a group of, with
// each one's still and why none of its files can be read now where none can, and the cursor of the
// next page, "" on the last.
type SeasonPage struct {
	Episodes               []sqlcgen.ListSeasonEpisodeDetailsRow
	StillSha256ByEpisodeID map[int64]string
	UnreadableByEpisodeID  map[int64]Unreadable
	NextCursor             string
}

// Season is a page of one season's episodes; a series reach does not reach is ErrNoItem, and a
// cursor this server did not give ErrBadCursor (D306 §9).
func (browser *Browser) Season(ctx context.Context, reach auth.Reach, query SeasonQuery) (SeasonPage, error) {
	return browser.episodePage(ctx, reach, query, func(
		queries *sqlcgen.Queries, ordering string, after float64,
	) ([]sqlcgen.ListSeasonEpisodeDetailsRow, error) {
		episodes, err := queries.ListSeasonEpisodeDetails(ctx, sqlcgen.ListSeasonEpisodeDetailsParams{
			CollectionID: query.SeriesID, OrderingName: ordering, SeasonNumber: query.Season, AfterSortOrder: after,
			MaxRows: query.Limit + 1,
		})
		if err != nil {
			return nil, fmt.Errorf("list season %d of series %d: %w", query.Season, query.SeriesID, err)
		}
		return episodes, nil
	})
}

// NotInOrdering is a page of the series' episodes the ordering its person watches it in leaves out,
// in aired order and at their aired places, which its page lists after the ordering's groups, so
// nothing in the library disappears by a viewing choice (episode-orderings.md §7 Q4). In aired it
// is empty. The query's season is not read.
func (browser *Browser) NotInOrdering(ctx context.Context, reach auth.Reach, query SeasonQuery) (SeasonPage, error) {
	return browser.episodePage(ctx, reach, query, func(
		queries *sqlcgen.Queries, ordering string, after float64,
	) ([]sqlcgen.ListSeasonEpisodeDetailsRow, error) {
		if ordering == OrderingAired {
			return nil, nil
		}
		rows, err := queries.ListEpisodesNotInOrdering(ctx, sqlcgen.ListEpisodesNotInOrderingParams{
			CollectionID: query.SeriesID, OrderingName: ordering, AfterSortOrder: after, MaxRows: query.Limit + 1,
		})
		if err != nil {
			return nil, fmt.Errorf("list series %d's episodes its %s ordering leaves out: %w", query.SeriesID,
				ordering, err)
		}
		episodes := make([]sqlcgen.ListSeasonEpisodeDetailsRow, 0, len(rows))
		for _, row := range rows {
			episodes = append(episodes, sqlcgen.ListSeasonEpisodeDetailsRow(row))
		}
		return episodes, nil
	})
}

// episodePage is a page of a series' episodes as list reads them, after the cursor, in the ordering
// the query's person watches it in, with their stills and why none of their files can be read now.
func (browser *Browser) episodePage(
	ctx context.Context, reach auth.Reach, query SeasonQuery,
	list func(queries *sqlcgen.Queries, ordering string, after float64) ([]sqlcgen.ListSeasonEpisodeDetailsRow, error),
) (SeasonPage, error) {
	if _, err := browser.reachedSeries(ctx, reach, query.SeriesID); err != nil {
		return SeasonPage{}, err
	}
	after := math.Inf(-1)
	if query.Cursor != "" {
		parsed, err := strconv.ParseFloat(query.Cursor, 64)
		if err != nil || math.IsNaN(parsed) {
			return SeasonPage{}, ErrBadCursor
		}
		after = parsed
	}
	queries := browser.database.Read()
	ordering, err := ViewingOrdering(ctx, queries, query.UserID, query.SeriesID)
	if err != nil {
		return SeasonPage{}, err
	}
	episodes, err := list(queries, ordering, after)
	if err != nil {
		return SeasonPage{}, err
	}
	page := SeasonPage{}
	if int64(len(episodes)) > query.Limit {
		episodes = episodes[:query.Limit]
		page.NextCursor = strconv.FormatFloat(episodes[len(episodes)-1].SortOrder, 'g', -1, 64)
	}
	page.Episodes = episodes
	ids := make([]int64, 0, len(episodes))
	for _, episode := range episodes {
		ids = append(ids, episode.TitleID)
	}
	if page.StillSha256ByEpisodeID, err = browser.shownArtSourceSha256s(ctx, ArtStill, ids); err != nil {
		return SeasonPage{}, err
	}
	if page.UnreadableByEpisodeID, err = browser.unreadableItems(ctx, ids, false); err != nil {
		return SeasonPage{}, err
	}
	return page, nil
}

// FileWork is what a file is, as now playing names it: its title, when one is filed, by id, kind
// and name, its path, the library it lives in, and how long it plays, 0 before it is probed.
type FileWork struct {
	TitleID int64
	// TitleKind is its title's kind (TitleKindMovie and the rest); "" for a file filed as no title.
	TitleKind  string
	Name       string
	Path       string
	LibraryID  int64
	DurationMs int64
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
	return FileWork{
		TitleID: ptr.ValueOrZero(row.TitleID), TitleKind: ptr.ValueOrZero(row.TitleKind),
		Name: ptr.ValueOrZero(row.Name), Path: row.Path, LibraryID: row.LibraryID, DurationMs: row.DurationMs,
	}, nil
}

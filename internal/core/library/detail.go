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
	// IsWatched is whether its person watched it, by playing or by hand (D370 §1).
	IsWatched bool
}

// Version is one of a title's versions: its files in part order (D155).
type Version struct {
	ID        int64
	EditionID *int64
	Files     []FileDetail
}

// FileDetail is one file of a version, with its streams, at the places its person reaches: Path is the
// first of them, and Paths each, in the order they were found (D363 §8). An admin reaches every place.
type FileDetail struct {
	ID        int64
	Path      string
	Paths     []string
	SizeBytes *int64
	Streams   []sqlcgen.ListStreamsOfVideoFileRow
}

// Title is a title's detail as userID sees it, an episode placed in the ordering they watch its
// series in; one in a library reach does not reach is ErrNoItem, as one that does not exist (D243 §4).
func (browser *Browser) Title(ctx context.Context, reach auth.Reach, userID, titleID int64) (TitleDetail, error) {
	queries := browser.database.Read()
	title, err := store.OrNotFound(queries.GetTitleDetail(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return TitleDetail{}, fmt.Errorf("title %d: %w", titleID, ErrNoItem)
	}
	if err != nil {
		return TitleDetail{}, fmt.Errorf("read title %d: %w", titleID, err)
	}
	if reachErr := ReachItem(ctx, queries, reach, titleID); reachErr != nil {
		return TitleDetail{}, reachErr
	}
	detail := TitleDetail{GetTitleDetailRow: title}
	if detail.Versions, err = browser.versions(ctx, reach, titleID); err != nil {
		return TitleDetail{}, err
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	detail.Extras, err = queries.ListTitleExtras(ctx, sqlcgen.ListTitleExtrasParams{
		OwnerTitle: titleID, IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	if err != nil {
		return TitleDetail{}, fmt.Errorf("list title %d's extras: %w", titleID, err)
	}
	if detail.Art, err = browser.itemArt(ctx, titleID); err != nil {
		return TitleDetail{}, err
	}
	if detail.Episode, err = browser.EpisodePlaceOf(ctx, userID, titleID); err != nil {
		return TitleDetail{}, err
	}
	unreadable, err := browser.unreadableItems(ctx, reach, []int64{titleID}, false)
	if err != nil {
		return TitleDetail{}, err
	}
	if why, isUnreadable := unreadable[titleID]; isUnreadable {
		detail.Unreadable = &why
	}
	watched, err := watchedTitles(ctx, queries, userID, []int64{titleID})
	if err != nil {
		return TitleDetail{}, err
	}
	detail.IsWatched = watched[titleID]
	return detail, nil
}

// versions are a title's versions and their files at the places reach reaches, each file once with
// every such place; a version none of whose files it reaches is not among them (D363 §8).
func (browser *Browser) versions(ctx context.Context, reach auth.Reach, titleID int64) ([]Version, error) {
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	places, err := queries.ListTitleFiles(ctx, sqlcgen.ListTitleFilesParams{
		TitleID: titleID, IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("list title %d's files: %w", titleID, err)
	}
	versions := []Version{}
	for _, place := range places {
		if len(versions) == 0 || versions[len(versions)-1].ID != place.VersionID {
			versions = append(versions, Version{ID: place.VersionID, EditionID: place.EditionID})
		}
		last := &versions[len(versions)-1]
		if count := len(last.Files); count > 0 && last.Files[count-1].ID == place.FileID {
			last.Files[count-1].Paths = append(last.Files[count-1].Paths, place.Path)
			continue
		}
		streams, err := queries.ListStreamsOfVideoFile(ctx, place.FileID)
		if err != nil {
			return nil, fmt.Errorf("list file %d's streams: %w", place.FileID, err)
		}
		last.Files = append(last.Files, FileDetail{
			ID: place.FileID, Path: place.Path, Paths: []string{place.Path}, SizeBytes: place.SizeBytes,
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
	// IsWatched is whether its person watched every one of its episodes with a file, one at least, in
	// the ordering they watch it in (D370 §3).
	IsWatched bool
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
	// IsWatched is whether its person watched every one of its episodes with a file, one at least
	// (D370 §3).
	IsWatched bool
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
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	episodes, err := queries.ListSeriesEpisodeDetails(ctx, sqlcgen.ListSeriesEpisodeDetailsParams{
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON, CollectionID: seriesID, OrderingName: ordering,
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
	watched, err := watchedTitles(ctx, queries, userID, ids)
	if err != nil {
		return SeriesDetail{}, err
	}
	detail.Seasons = seasonsOf(episodes, stills, watched, ordering == OrderingAired)
	detail.IsWatched = len(detail.Seasons) > 0
	for _, season := range detail.Seasons {
		detail.IsWatched = detail.IsWatched && season.IsWatched
	}
	err = browser.withOrderings(ctx, &detail)
	if err != nil {
		return SeriesDetail{}, err
	}
	unreadable, err := browser.unreadableItems(ctx, reach, []int64{seriesID}, true)
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
	queries := browser.database.Read()
	series, err := store.OrNotFound(queries.GetSeriesDetail(ctx, seriesID))
	if errors.Is(err, store.ErrNotFound) {
		return sqlcgen.GetSeriesDetailRow{}, fmt.Errorf("series %d: %w", seriesID, ErrNoItem)
	}
	if err != nil {
		return sqlcgen.GetSeriesDetailRow{}, fmt.Errorf("read series %d: %w", seriesID, err)
	}
	if err := ReachItem(ctx, queries, reach, seriesID); err != nil {
		return sqlcgen.GetSeriesDetailRow{}, err
	}
	return series, nil
}

// seasonsOf sums a series' episodes, in an ordering's order, up by its groups, in order: in aired,
// numbered seasons in order, then the specials; in another, whose group 0 is a part like the rest
// (a TMDB episode group's first), by number alone. A season is watched once every episode of it with
// a file is, one at least (D370 §3).
func seasonsOf(
	episodes []sqlcgen.ListSeriesEpisodeDetailsRow, stills map[int64]string, watched map[int64]bool, isAired bool,
) []SeasonSummary {
	byNumber := map[int64]*SeasonSummary{}
	var numbers []int64
	unwatched := map[int64]bool{} // a season with an episode with a file its person has not watched
	for _, episode := range episodes {
		number := ptr.ValueOrZero(episode.GroupNumber)
		season, isSeen := byNumber[number]
		if !isSeen {
			season = &SeasonSummary{Number: number}
			byNumber[number] = season
			numbers = append(numbers, number)
		}
		if episode.Available {
			season.IsWatched = !unwatched[number]
			if !watched[episode.TitleID] {
				season.IsWatched, unwatched[number] = false, true
			}
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
	// IsWatchedByEpisodeID are those its person watched (D370 §2).
	IsWatchedByEpisodeID map[int64]bool
	NextCursor           string
}

// Season is a page of one season's episodes; a series reach does not reach is ErrNoItem, and a
// cursor this server did not give ErrBadCursor (D306 §9).
func (browser *Browser) Season(ctx context.Context, reach auth.Reach, query SeasonQuery) (SeasonPage, error) {
	return browser.episodePage(ctx, reach, query, func(
		queries *sqlcgen.Queries, ordering string, after float64,
	) ([]sqlcgen.ListSeasonEpisodeDetailsRow, error) {
		isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
		episodes, err := queries.ListSeasonEpisodeDetails(ctx, sqlcgen.ListSeasonEpisodeDetailsParams{
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
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
		isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
		rows, err := queries.ListEpisodesNotInOrdering(ctx, sqlcgen.ListEpisodesNotInOrderingParams{
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
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
	if page.UnreadableByEpisodeID, err = browser.unreadableItems(ctx, reach, ids, false); err != nil {
		return SeasonPage{}, err
	}
	if page.IsWatchedByEpisodeID, err = watchedTitles(ctx, queries, query.UserID, ids); err != nil {
		return SeasonPage{}, err
	}
	return page, nil
}

// FileWork is what a file is, as now playing names it: its title, when one is filed, by id, kind
// and name, its first place's path, a library reach reaches that covers it, which a person goes back
// up to, 0 for none, and how long it plays, 0 before it is probed.
type FileWork struct {
	TitleID int64
	// TitleKind is its title's kind (TitleKindMovie and the rest); "" for a file filed as no title.
	TitleKind  string
	Name       string
	Path       string
	LibraryID  int64
	DurationMs int64
}

// WorkOfFile is what a file is, the library named one reach reaches; library.ErrNoFile for one there
// is not.
func (browser *Browser) WorkOfFile(ctx context.Context, reach auth.Reach, fileID int64) (FileWork, error) {
	queries := browser.database.Read()
	row, err := store.OrNotFound(queries.GetTitleOfFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return FileWork{}, fmt.Errorf("file %d: %w", fileID, ErrNoFile)
	}
	if err != nil {
		return FileWork{}, fmt.Errorf("read what file %d is: %w", fileID, err)
	}
	libraryIDs, err := queries.ListLibrariesOfFile(ctx, fileID)
	if err != nil {
		return FileWork{}, fmt.Errorf("read the libraries of file %d: %w", fileID, err)
	}
	work := FileWork{
		TitleID: ptr.ValueOrZero(row.TitleID), TitleKind: ptr.ValueOrZero(row.TitleKind),
		Name: ptr.ValueOrZero(row.Name), Path: row.Path, DurationMs: row.DurationMs,
	}
	for _, libraryID := range libraryIDs {
		if reach.Reaches(&libraryID) {
			work.LibraryID = libraryID
			break
		}
	}
	return work, nil
}

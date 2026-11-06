// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/rowstest"
)

// What waits for a person's match is counted as "Needs a match" lists it: an ambiguous title or
// series, or one a search found nothing for, with a live file a library lists; never an episode, a
// title not searched yet, or one with no live file (D209; D303's count, the access audit's A7; D363).
func TestCountWaitingForAPerson_CountsWhatTheListShows(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database, "PRAGMA foreign_keys = OFF",
		rowstest.Node(rowstest.NodeSpec{ID: 1, Name: "local"}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1, NodeID: 1, RootPath: "/"}))
	title := func(id int64, kind, state, source string, hasFile bool) {
		t.Helper()
		execRaw(t, database,
			rowstest.Title(rowstest.ItemSpec{ID: id, Kind: kind, IdentificationState: state, FollowedSource: source}),
			rowstest.Version(rowstest.VersionSpec{ID: id, TitleID: id}))
		if hasFile {
			execRaw(t, database, rowstest.File(rowstest.FileSpec{
				ID: id, NodeID: 1, Path: fmt.Sprintf("/p%d", id), LibraryKind: "movie", VersionID: id,
			}))
		}
	}
	title(1, "movie", "ambiguous", "", true)          // counted
	title(2, "movie", "unidentified", "none", true)   // counted: a search found nothing
	title(3, "episode", "unidentified", "none", true) // its series' to count
	title(4, "movie", "unidentified", "", true)       // not searched yet
	title(5, "movie", "ambiguous", "", false)         // no live file
	title(6, "episode", "identified", "tmdb", true)   // the series' episode
	title(7, "episode", "identified", "tmdb", true)
	execRaw(t, database,
		rowstest.Series(rowstest.ItemSpec{ID: 20, IdentificationState: "ambiguous"}),
		rowstest.Series(rowstest.ItemSpec{ID: 21, FollowedSource: "none"}),
		rowstest.Ordering(rowstest.OrderingSpec{ID: 1, SeriesID: 20, Kind: "aired"}),
		rowstest.Ordering(rowstest.OrderingSpec{ID: 2, SeriesID: 21, Kind: "aired"}),
		rowstest.Position(rowstest.PositionSpec{OrderingID: 1, TitleID: 6, SortOrder: 1}),
		rowstest.Position(rowstest.PositionSpec{OrderingID: 2, TitleID: 7, SortOrder: 1}))
	count, err := database.Read().CountWaitingForAPerson(t.Context())
	if err != nil || count != 4 {
		t.Errorf("counted %d, %v; want 4: the ambiguous film, the film nothing was found for, and both series",
			count, err)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/rowstest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The system library is in no list of libraries or anything drawn from them (D435 §2): seeded beside
// a visible library, each with a video, the admin's list, a library's read, the libraries' name space
// (§3), a removal and version_library, which every list of titles, the search and Next Up read, see
// the visible one alone. The server finds it by its kind (§1).
func TestArch_TheSystemLibraryIsInNoListing(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database,
		rowstest.Node(rowstest.NodeSpec{ID: 1}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1, Name: "Clips", Kind: "other", NodeID: 1, RootPath: "/media"}),
		rowstest.Library(rowstest.LibrarySpec{ID: 2, Name: "System", Kind: "other", NodeID: 1, RootPath: "/own"}),
		"INSERT INTO system_library (kind, library_id) VALUES ('system', 2)",
		rowstest.Title(rowstest.ItemSpec{ID: 20, Kind: "other"}),
		rowstest.Title(rowstest.ItemSpec{ID: 21, Kind: "other"}),
		rowstest.Version(rowstest.VersionSpec{ID: 30, TitleID: 20}),
		rowstest.Version(rowstest.VersionSpec{ID: 31, TitleID: 21}),
		rowstest.File(rowstest.FileSpec{
			ID: 10, SizeBytes: 100, NodeID: 1, Path: "/media/a.mkv", LibraryKind: "other", VersionID: 30,
		}),
		rowstest.File(rowstest.FileSpec{
			ID: 11, SizeBytes: 200, NodeID: 1, Path: "/own/sound-timing.h264-aac.mp4", LibraryKind: "other",
			VersionID: 31,
		}))
	queries := database.Read()

	libraries, err := queries.ListLibraries(t.Context())
	if err != nil || len(libraries) != 1 || libraries[0].ID != 1 {
		t.Errorf("the admin's libraries are %+v, %v; want Clips alone", libraries, err)
	}
	if _, err = queries.GetLibrary(t.Context(), 2); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("reading the system library by its id = %v; want no such library", err)
	}
	if _, err = queries.GetLiveLibraryByName(t.Context(), "System"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("the name System = %v; want it free for an admin's library (D435 §3)", err)
	}
	var removed int64
	err = database.Write(t.Context(), func(writing *sqlcgen.Queries) error {
		removed, err = writing.RemoveLibrary(t.Context(), sqlcgen.RemoveLibraryParams{ID: 2})
		if err != nil {
			return fmt.Errorf("remove the system library: %w", err)
		}
		return nil
	})
	if err != nil || removed != 0 {
		t.Errorf("removing the system library removed %d, %v; want none", removed, err)
	}
	checkFoundByKind(t, database)
	listed := fmt.Sprint(queryRows(t, database, "SELECT version_id, library_id FROM version_library ORDER BY 1", 2))
	if listed != "[[30 1]]" {
		t.Errorf("version_library = %s; want Clips' video alone", listed)
	}
}

// checkFoundByKind fails unless the server finds the seeded system library, and its one video, by its
// kind (D435 §1).
func checkFoundByKind(t *testing.T, database *Database) {
	t.Helper()
	queries := database.Read()
	system, err := queries.GetSystemLibrary(t.Context())
	if err != nil || system.ID != 2 || system.NodeID == nil || *system.NodeID != 1 || *system.RootPath != "/own" {
		t.Fatalf("the system library found by its kind is %+v, %v; want library 2 on node 1 at /own", system, err)
	}
	videos, err := queries.ListSystemVideos(t.Context())
	if err != nil || len(videos) != 1 || *videos[0].FileID != 11 || videos[0].Path != "/own/sound-timing.h264-aac.mp4" {
		t.Errorf("the system videos are %+v, %v; want its one clip", videos, err)
	}
}

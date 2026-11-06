// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/rowstest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A file is found, listed and played at its live place by place.file_id, not at the place whose id
// is the file's (D432 section 6): film 1's file has its own place, 1, gone, and lives at place 7.
// Its title lists it there, counts it, plays it from there and is found when the file first was.
func TestFilePlace_AFileIsListedAndPlayedAtAPlaceWhoseIDIsNotItsOwn(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database,
		rowstest.Node(rowstest.NodeSpec{ID: 1}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1, NodeID: 1}),
		rowstest.Title(rowstest.ItemSpec{ID: 1, LibraryID: 1}),
		rowstest.Version(rowstest.VersionSpec{ID: 1, TitleID: 1}),
		rowstest.File(rowstest.FileSpec{
			ID: 1, SizeBytes: 5, NodeID: 1, Path: "/m/a.mkv", ModifiedAt: 30, FirstSeenAt: 100, RemovedAt: 300,
			LibraryKind: "movie", VersionID: 1,
		}),
		rowstest.Place(rowstest.PlaceSpec{
			ID: 7, FileID: 1, NodeID: 1, Path: "/m/b.mkv", ModifiedAt: 40, FirstSeenAt: 200,
		}))
	ctx := t.Context()
	queries := database.Read()
	params := sqlcgen.ListWorkFilesParams{WorkID: 1, IsEveryLibrary: 1, LibraryIdsJSON: "[]"}
	listed, err := queries.ListWorkFiles(ctx, params)
	if err != nil || len(listed) != 1 {
		t.Fatalf("the title's files = %+v, %v; want file 1 at place 7", listed, err)
	}
	live, err := queries.ListLiveFilesOfWork(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	count, err := queries.CountLiveFilesOfWork(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	playable, err := queries.GetPlayableFile(ctx, 1)
	if err != nil {
		t.Fatalf("the file to play: %v; want it at place 7", err)
	}
	checks := []struct{ what, got, want string }{
		{
			"the title's file", fmt.Sprintf("%d %d %s", listed[0].FileID, listed[0].PlaceID, listed[0].Path),
			"1 7 /m/b.mkv",
		},
		{"its live files", fmt.Sprintf("%+v", live), "[{ID:1 Path:/m/b.mkv LibraryKind:movie DurationMs:0}]"},
		{"its count of live files", fmt.Sprint(count), "1"},
		{"where it plays from", string(playable.ReportedPathRaw), "/m/b.mkv"},
		{
			"when it was found", fmt.Sprint(queryRows(t, database,
				"SELECT found_at, found_file_modified_at FROM item WHERE id = 1", 2)), "[[100 40]]",
		},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %s; want %s", check.what, check.got, check.want)
		}
	}
}

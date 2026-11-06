// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/rowstest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// TestTitleFiles_EveryQueryLeavesOutARemovedVersion checks that the three queries asking for a
// title's live files agree: a live file of a removed version is not the title's.
func TestTitleFiles_EveryQueryLeavesOutARemovedVersion(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	// Title 10 has version 1, live, and version 2, removed; each has one live file.
	storetest.ExecRaw(t, database.Path(),
		rowstest.Node(rowstest.NodeSpec{ID: 1, Name: "local"}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1, NodeID: 1}),
		rowstest.Title(rowstest.ItemSpec{ID: 10, LibraryID: 1}),
		rowstest.Version(rowstest.VersionSpec{ID: 1, TitleID: 10}),
		rowstest.Version(rowstest.VersionSpec{ID: 2, TitleID: 10, RemovedAt: 5}),
		rowstest.File(rowstest.FileSpec{
			ID: 1, SizeBytes: 10, NodeID: 1, Path: "/m/kept.mkv", LibraryKind: "movie", VersionID: 1,
		}),
		rowstest.File(rowstest.FileSpec{
			ID: 2, SizeBytes: 10, NodeID: 1, Path: "/m/removed.mkv", LibraryKind: "movie", VersionID: 2,
		}))
	queries := database.Read()
	ctx := t.Context()

	listed, err := queries.ListLiveFilesOfWork(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var listedIDs []int64
	for _, file := range listed {
		listedIDs = append(listedIDs, file.ID)
	}
	if !slices.Equal(listedIDs, []int64{1}) {
		t.Errorf("ListLiveFilesOfWork = files %v, want [1]: the removed version's file is not the title's", listedIDs)
	}

	count, err := queries.CountLiveFilesOfWork(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("CountLiveFilesOfWork = %d, want 1: the removed version's file is not the title's", count)
	}

	detail, err := queries.ListWorkFiles(ctx, sqlcgen.ListWorkFilesParams{WorkID: 10, IsEveryLibrary: 1})
	if err != nil {
		t.Fatal(err)
	}
	var detailIDs []int64
	for _, file := range detail {
		detailIDs = append(detailIDs, file.FileID)
	}
	if !slices.Equal(detailIDs, []int64{1}) {
		t.Errorf("ListWorkFiles = files %v, want [1]: the removed version's file is not the title's", detailIDs)
	}
}

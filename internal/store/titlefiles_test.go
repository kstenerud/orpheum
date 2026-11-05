// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"slices"
	"testing"

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
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movie', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/m')",
		"INSERT INTO item (id, kind, filing_library_id, added_at, identification_state) "+
			"VALUES (10, 'movie', 1, 1, 'identified')",
		"INSERT INTO work (id, kind) VALUES (10, 'movie')",
		"INSERT INTO version (id, work_id, created_at, removed_at) VALUES (1, 10, 1, NULL), (2, 10, 1, 5)",
		`INSERT INTO file (id, node_id, path, reported_path_raw, size_bytes, filing, first_seen_at, last_seen_at)
		 VALUES (1, 1, '/m/kept.mkv', CAST('/m/kept.mkv' AS BLOB), 10, 'video', 1, 1),
		        (2, 1, '/m/removed.mkv', CAST('/m/removed.mkv' AS BLOB), 10, 'video', 1, 1)`,
		`INSERT INTO file_filing (file_id, library_kind, filing, version_id)
		 VALUES (1, 'movie', 'listed', 1), (2, 'movie', 'listed', 2)`)
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

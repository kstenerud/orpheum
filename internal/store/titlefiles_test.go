// SPDX-License-Identifier: GPL-3.0-or-later

package store_test

import (
	"slices"
	"testing"

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
		 VALUES (1, 'Films', 'mixed', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO item (id, kind, library_id, added_at) VALUES (10, 'title', 1, 1)",
		"INSERT INTO title (id, kind, identification_state) VALUES (10, 'movie', 'identified')",
		"INSERT INTO version (id, title_id, created_at, removed_at) VALUES (1, 10, 1, NULL), (2, 10, 1, 5)",
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, filing, version_id, first_seen_at, last_seen_at)
		 VALUES (1, 1, 1, '/m/kept.mkv', 10, 'listed', 1, 1, 1), (2, 1, 1, '/m/removed.mkv', 10, 'listed', 2, 1, 1)`)
	queries := database.Read()
	ctx := t.Context()

	listed, err := queries.ListLiveFilesOfTitle(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var listedIDs []int64
	for _, file := range listed {
		listedIDs = append(listedIDs, file.ID)
	}
	if !slices.Equal(listedIDs, []int64{1}) {
		t.Errorf("ListLiveFilesOfTitle = files %v, want [1]: the removed version's file is not the title's", listedIDs)
	}

	count, err := queries.CountLiveFilesOfTitle(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("CountLiveFilesOfTitle = %d, want 1: the removed version's file is not the title's", count)
	}

	detail, err := queries.ListTitleFiles(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var detailIDs []int64
	for _, file := range detail {
		detailIDs = append(detailIDs, file.FileID)
	}
	if !slices.Equal(detailIDs, []int64{1}) {
		t.Errorf("ListTitleFiles = files %v, want [1]: the removed version's file is not the title's", detailIDs)
	}
}

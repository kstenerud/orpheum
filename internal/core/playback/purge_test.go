// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A removed file deleted past its keeping leaves what was watched of it on its version: the
// person's totals are the same after, and a file removed lately, or one whose sidecar stays, is
// kept with its watching (D306 §8).
func TestPurgeRemovedFiles_KeepsWhatWasWatchedOnTheVersion(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := &Service{database: database}
	userID := storetest.InsertUser(t, database, "alice", "admin")
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/media"})
	const longAgo, lately = 1_000, 9_000
	statements := []string{
		"INSERT INTO item (id, kind, library_id, added_at) VALUES (10, 'title', " + fmt.Sprint(libraryID) + ", 1)",
		"INSERT INTO title (id, kind, identification_state) VALUES (10, 'movie', 'identified')",
		"INSERT INTO version (id, title_id, created_at) VALUES (20, 10, 1)",
	}
	// Files 1 and 2 were removed long ago, 3 lately; 4 long ago, but its subtitle 5 is still there.
	for fileID, removedAt := range map[int]int{1: longAgo, 2: longAgo, 3: lately, 4: longAgo} {
		statements = append(statements, fmt.Sprintf(
			`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, payload_hash, filing,
			 version_id, first_seen_at, last_seen_at, removed_at)
			 VALUES (%[1]d, %[2]d, %[3]d, '/media/Metropolis %[1]d.mkv', CAST('/media/Metropolis %[1]d.mkv' AS BLOB), 10,
			 X'0%[1]d', 'listed', 20, 1, 1, %[4]d)`, fileID, libraryID, nodeID, removedAt),
			fmt.Sprintf("INSERT INTO file_probe (file_id, probe_json) VALUES (%d, '{}')", fileID),
			fmt.Sprintf(`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, disposition_flags)
			 VALUES (%[1]d, %[1]d, 0, 'video', 'h264', 0)`, fileID),
			fmt.Sprintf(`INSERT INTO play_span (user_id, file_id, from_position_ms, to_position_ms, started_at,
			 ended_at) VALUES (%[1]d, %[2]d, 0, 600000, %[2]d000, %[2]d600)`, userID, fileID),
			fmt.Sprintf(`INSERT INTO play_event (user_id, file_id, kind, position_ms, happened_at)
			 VALUES (%[1]d, %[2]d, 'completed', 600000, %[2]d600)`, userID, fileID),
			fmt.Sprintf(`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at)
			 VALUES (%d, %d, 600000, 1, 1)`, userID, fileID),
			fmt.Sprintf(`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
			 VALUES (%d, 'file', %d, 'subtitle_offset_ms', '500', 1)`, userID, fileID),
			fmt.Sprintf(`INSERT INTO fetched_subtitle (video_file, provider, provider_file_id, language, format,
			 body_gzip, fetched_by_user_id, fetched_at) VALUES (%d, 'opensubtitles', '%d', 'en', 'srt', X'00', %d, 1)`,
				fileID, fileID, userID))
	}
	statements = append(statements, fmt.Sprintf(
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, filing, sidecar_kind,
		 attached_to_file, first_seen_at, last_seen_at)
		 VALUES (5, %d, %d, '/media/Metropolis 4.en.srt', CAST('/media/Metropolis 4.en.srt' AS BLOB), 10, 'attached',
		 'subtitle', 4, 1, 1)`, libraryID, nodeID))
	storetest.ExecRaw(t, database.Path(), statements...)
	before, err := service.WatchStatsOf(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}

	purger := library.NewFilePurger(database)
	deleted, err := purger.PurgeRemovedFiles(t.Context(), time.UnixMilli(5_000), FoldWatchingOfPurgedFiles,
		preferences.ForgetPreferencesOfPurgedFiles, subtitles.ForgetSubtitlesOfPurgedFiles)
	if err != nil || deleted != 2 {
		t.Fatalf("PurgeRemovedFiles = %d, %v; want files 1 and 2 deleted", deleted, err)
	}
	if got := storetest.QueryRaw(t, database.Path(), "SELECT id FROM file ORDER BY id"); !slices.Equal(got,
		[]string{"3", "4", "5"}) {
		t.Errorf("files after the purge = %v; want 3 (removed lately), 4 (its subtitle stays) and 5", got)
	}
	for table, column := range map[string]string{
		"play_span": "file_id", "play_event": "file_id", "play_position": "file_id", "file_probe": "file_id",
		"fetched_subtitle": "video_file", "stream": "video_file", "preference": "scope_id",
	} {
		got := storetest.QueryRaw(t, database.Path(), "SELECT "+column+" FROM "+table+" ORDER BY "+column)
		if !slices.Equal(got, []string{"3", "4"}) {
			t.Errorf("%s after the purge names files %v; want 3 and 4", table, got)
		}
	}
	kept := storetest.QueryRaw(t, database.Path(),
		"SELECT version_id || '|' || completed_count || '|' || watched_ms || '|' || last_watched_at "+
			"FROM purged_watching")
	if !slices.Equal(kept, []string{"20|2|1200|2600"}) {
		t.Errorf("kept on the version: %v; want 20|2|1200|2600: two finishes, 1.2 s watched, last at 2600", kept)
	}
	after, err := service.WatchStatsOf(t.Context(), userID)
	if err != nil || after != before {
		t.Errorf("the person's watching after the purge = %+v, %v; want %+v, as before", after, err, before)
	}
}

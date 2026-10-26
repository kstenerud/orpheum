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
	titleID := librarytest.Title(t, database, librarytest.TitleSpec{ID: 10, Library: libraryID})
	librarytest.Version(t, database, librarytest.VersionSpec{ID: 20, TitleID: titleID})
	var statements []string
	// Files 1 and 2 were removed long ago, 3 lately; 4 long ago, but its subtitle 5 is still there.
	for fileID, removedAt := range map[int]int{1: longAgo, 2: longAgo, 3: lately, 4: longAgo} {
		librarytest.File(t, database, librarytest.FileSpec{
			ID: int64(fileID), LibraryID: libraryID, NodeID: nodeID, Path: fmt.Sprintf("/media/Metropolis %d.mkv", fileID),
			SizeBytes: 10, PayloadHash: []byte{byte(fileID)}, //nolint:gosec // a test's small id, 1-4
			Filing: "listed", VersionID: 20,
			RemovedAt: int64(removedAt),
		})
		librarytest.Probe(t, database, librarytest.ProbeSpec{FileID: int64(fileID)})
		librarytest.Stream(t, database, librarytest.StreamSpec{
			BytesFile: int64(fileID), StreamIndex: 0, Kind: "video", Codec: "h264",
		})
		statements = append(statements,
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
	storetest.ExecRaw(t, database.Path(), statements...)
	librarytest.File(t, database, librarytest.FileSpec{
		ID: 5, LibraryID: libraryID, NodeID: nodeID, Path: "/media/Metropolis 4.en.srt", SizeBytes: 10,
		Filing: "attached", SidecarKind: "subtitle", AttachedToFile: 4,
	})
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
